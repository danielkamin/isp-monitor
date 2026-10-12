# probe-service — Context

## Cel

Wykonuje pomiary sieciowe (ping, TCP, traceroute, MTR, HTTP), zapisuje wyniki do PostgreSQL i streamuje je przez gRPC do subskrybentów (analytics-service).

## Architektura wewnętrzna

```
main.go
  ├─ NewScheduler → Start(ctx)        ticker → DB → goroutine per target
  ├─ prober.New(protocol)             factory zwraca właściwy Prober
  ├─ resultsChan (chan ProbeResult)    kanał tworzony w main.go
  ├─ db.SaveResult()                  zapis do probe.probes (sqlc)
  ├─ broadcaster.Publish()            fan-out do aktywnych gRPC streamów
  └─ gRPC server :50051
       └─ ResultsServer.StreamResults() → subskrybuje broadcaster
```

## Przepływ danych

```
Scheduler (ticker co ~30s)
  └─→ queries.ListActiveTargets()     świeże targety przy każdym tick
  └─→ for each target: go func()
        prober.New(t.Protocol).Probe(ctx, t.Ip, t.Port)
        resultsChan <- ProbeResult

main.go goroutine (for result := range resultsChan)
  ├─→ db.SaveProbe(ctx, result)       zawsze, niezależnie od subskrybentów
  └─→ broadcaster.Publish(result)

Broadcaster
  └─→ każdy aktywny chan → ResultsServer.StreamResults() → analytics-service
```

## Pakiety

### `internal/prober`

- `prober.go` — interfejs `Prober` + struct `ProbeResult`
- `factory.go` — `New(protocol ProbeProtocol) (Prober, error)` — switch po typie, błąd dla nieznanych
- `ping.go` — ICMP echo przez `icmp.ListenPacket("udp4")` (bez roota, wymaga `net.ipv4.ping_group_range`); respektuje `ctx` — **zaimplementowane**
- `tcp.go` — TCP connect (`net.Dialer.DialContext`, `net.JoinHostPort` — działa też dla IPv6) — **zaimplementowane**
- `mtr.go` — **wspólny silnik ICMP** `icmpTrace(ctx, ip, rounds)` + `MtrProber` (5 rund) — **zaimplementowane**
  - raw socket `ip4:icmp`, losowe ID per `Probe`, `seq = round*maxHops + ttl`
  - `classify()` dopasowuje odpowiedź po ID/seq (Echo Reply = cel; Time Exceeded / Dest Unreachable → ID/seq z zagnieżdżonego oryginalnego datagramu); obce pakiety ICMP są ignorowane
  - runda 1 odkrywa długość trasy (koniec: odpowiedź celu / Dest Unreachable / 5 kolejnych timeoutów), kolejne rundy mierzą tylko tę trasę
  - `runMtr(ctx, probe hopProbeFunc, rounds)` jest czystą logiką (agregacja per hop) — testowalną na fake'owym probe'rze
  - tylko IPv4; próby sekwencyjne, timeout hopa 1 s
- `traceroute.go` — `TracerouteProber` = jedna runda `icmpTrace` (nie ma już własnej pętli) + typ `HopResult` — **zaimplementowane**
- `http.go` — **stub** (świadomie odłożone, patrz „Do zrobienia")

`HopResult`: `Ttl, Rtt (ms; mtr = średnia), Ip ("*" przy timeoucie), Sent, Recv, Loss (0.0–1.0), Best, Worst (ms)`.
`ProbeResult` dla MTR/traceroute: `Success` = cel odpowiedział; `LatencyMs`/`PacketLoss` z ostatniego hopa; `ErrorMsg = "destination not reached"` gdy nie.

### `internal/scheduler`

- `Scheduler` struct: `queries`, `resultsChan`, `interval`, `logger`
- `NewScheduler(queries, resultsChan, intervalSeconds int, logger)` — interval z env
- `Start(ctx)` — ticker + `ListActiveTargets()` co tick + goroutine per target
- po `Probe()` scheduler dokłada `res.TargetID` i `res.SiteID` z `target` przed wysłaniem do kanału
- `target.Port.Valid` sprawdzane przed wywołaniem probera

### `internal/server`

- `Broadcaster` — `map[string]chan *pb.StreamResultsResponse` + `sync.RWMutex`
  - `Subscribe(id)` → zwraca kanał (bufor 64), rejestruje subskrybenta
  - `Unsubscribe(id)` → `delete` z mapy
  - `Publish(r)` — drop + log jeśli kanał pełny
- `ResultsServer` — implementuje `ResultsServiceServer`
  - `StreamResults` — rejestruje się w broadcasterze, opcjonalny filtr po `site_id`, pętla `select` na kanał + `stream.Context().Done()`

### `internal/db`

- `connect.go` — `Connect(ctx, DBConfig) (*pgxpool.Pool, error)`
- `queries/` — kod generowany przez sqlc (nie edytować ręcznie)
  - `models.go` — `Target`, `Site`, `Probe`, `ProbeProtocol` enum, `DeviceType` enum
  - `targets.sql.go` — `ListActiveTargets(ctx)`
  - `probes.sql.go` — zapis wyników
  - `sites.sql.go`

## Schema bazy (probe.*)

```sql
probe.isp_providers  — ISP (id, name, asn, country)
probe.sites          — lokalizacje (id, name, isp_provider_id FK, active)
probe.targets        — co probować (id, site_id, ip, port, protocol, active)
probe.probes         — wyniki (id, target_id, latency_ms, packet_loss, success, error_msg, created_at)
```

`probe_protocol` enum: `icmp | tcp | traceroute | mtr | http`

`analytics_user` ma SELECT na całą schemę `probe` — może joinować sites/targets/isp_providers bez duplikowania danych w analytics.

## Proto / gRPC

Plik: `proto/probe/v1/results.proto`, generowany do `pkg/pb/probe/v1/`.

```protobuf
service ResultsService {
  rpc StreamResults(StreamResultsRequest) returns (stream StreamResultsResponse);
}
message StreamResultsRequest { optional string site_id = 1; }
message StreamResultsResponse {
  string target_id   = 1;
  string site_id     = 2;
  bool   success     = 3;
  int32  latency_ms  = 4;
  double packet_loss = 5;
  string error_msg   = 6;
  int64  probed_at   = 7;
}
```

**Do rozszerzenia:** `repeated HopResult hops` (pola jak w `prober.HopResult`) dla MTR/traceroute.

## Logowanie

`log/slog` z `JSONHandler(os.Stdout)`. Logger tworzony w `main.go`, przekazywany przez DI:
```go
baseLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
scheduler.NewScheduler(queries, resultsChan, baseLogger.With("component", "scheduler"))
```
Promtail zbiera stdout → Loki automatycznie.

## Uruchamianie / testy (Taskfile)

- `task build:probe` — `go build ./...`
- `task test:probe` — testy na hoście (Windows): testy ICMP **pomijają się** (`x/net` nie ma ICMP `udp4` ani `SetControlMessage` na Windows; kod jest pod Linuksa)
- `task test:probe:linux` — testy w kontenerze `golang:1.26` z `NET_RAW` i `ping_group_range`; tu działa komplet. `task test:probe:linux -- -short` pomija testy czekające na pełne timeouty (~10 s)

## Co zrobione

- [x] Struktura pakietów: config, db/connect, db/queries (sqlc), prober, scheduler, server
- [x] Schema DB: isp_providers, sites, targets, probes + probe_protocol enum
- [x] sqlc queries: ListActiveTargets, modele
- [x] Broadcaster: logger przez DI, Unsubscribe z `delete`
- [x] StreamResults: pętla select, filtr site_id, obsługa rozłączenia klienta
- [x] Scheduler: logger przez DI, error handling, `intervalSeconds` z env, TargetID/SiteID doklejane po Probe()
- [x] `prober.New()` factory + `ProbeResult` + sygnatura `Probe(ctx, ip netip.Addr, port int)`
- [x] `ping.go` — respektuje `ctx` (anulowanie → błąd `context.Canceled`, deadline rodzica skraca 5 s → wynik `Success=false`), `ErrorMsg` także przy nieoczekiwanym typie ICMP
- [x] `tcp.go` — poprawka IPv6 (`net.JoinHostPort` zamiast `Sprintf("%s:%d")`)
- [x] `mtr.go` — pełna implementacja (silnik wspólny z traceroute, statystyki per hop: loss/best/avg/worst, wykrywanie końca trasy, dopasowanie odpowiedzi po ID/seq)
- [x] `traceroute.go` — przepisany na jedną rundę silnika MTR; naprawione: własny Echo Request na loopbacku liczony jako hop, mieszanie odpowiedzi przy współbieżnych probach ICMP, `IPv4PacketConn()` zamiast paniczącego `ipv4.NewPacketConn`
- [x] `HopResult` rozszerzony o `Sent/Recv/Loss/Best/Worst`
- [x] Testy proberów (`ping_test.go`, `tcp_test.go`, `traceroute_test.go`, `mtr_test.go`) — wszystkie przechodzą w Linuksie
  - TCP: otwarty/zamknięty port, nieprawidłowe porty, anulowany ctx, deadline rodzica, IPv6, 50 równoległych
  - ping/traceroute: loopback, powtarzane i równoległe próby, anulowanie ctx, deadline, host bez odpowiedzi (TEST-NET 192.0.2.1)
  - MTR: czysta trasa, loss na jednym hopie, milczący router przy osiągniętym celu, martwy cel, Dest Unreachable, statystyki RTT, unikalność seq, zmiana trasy między rundami, propagacja błędu, anulowanie ctx
  - `classify`/`matchesOrig` na prawdziwych bajtach ICMP (Marshal → Parse)
  - testy to głównie integracja na loopbacku + czysta logika; **refaktor pod `Dialer`/`http.Client` nie był potrzebny**
- [x] Taskfile: `test:probe:linux`

## Do zrobienia (kolejność)

### 1. Zweryfikować uprawnienia ICMP w obrazie (najważniejsze ryzyko)

Obraz (`deployments/docker/probe-service.Dockerfile`) działa jako `nonroot` w distroless. `cap_add: NET_RAW` w compose zwykle nie daje aktywnego capability procesowi bez roota → raw socket `ip4:icmp` (traceroute, MTR) może zwracać `operation not permitted`. Testy w Linuksie przechodzą, bo biegną jako root. **Nie sprawdzone w uruchomionym kontenerze.**
- `task dev:up` + `task dev:logs -- probe-service` z targetem `mtr`/`traceroute`
- jeśli potwierdzi: `user: root` dla probe-service w compose albo `setcap cap_net_raw+ep` na binarce
- sprawdzić też, że `ping_group_range` w kontenerze obejmuje gid 65532 (ping `udp4`)

### 2. Scheduler a czas trwania probów

- MTR do martwego celu trwa ok. 25 s (`mtrMaxMisses × mtrTimeout × mtrRounds`), ping do 5 s; scheduler tickuje co ~30 s i odpala goroutine per target bez limitu → ryzyko nakładania się probów tego samego targetu
- dodać per-probe `context.WithTimeout` w schedulerze (budżet zależny od protokołu) i/lub pomijać target, którego poprzedni probe jeszcze trwa
- rozważyć równoległe TTL-e w MTR (komentarz `ponytail:` w `runMtr`), jeśli sekwencyjność będzie przeszkadzać

### 3. Zapis i przesył danych per hop

- `probe.probes` nie ma gdzie zapisać `HopResult` — dodać tabelę `probe.probe_hops` (probe_id, ttl, ip, sent, recv, loss, best/avg/worst) albo kolumnę `jsonb`, potem sqlc + zapis w `db.SaveProbe`
- proto: `repeated HopResult hops` w `StreamResultsResponse` + mapowanie w `ResultsServer`
- ustalić semantykę: `PacketLoss` dla ping/TCP przy porażce wynosi dziś 0 (komentarz w `prober.go` mówi „0 albo 1") — ujednolicić (porażka = 1.0)

### 4. `http.go`

Świadomie odłożone na rzecz MTR. Jeśli wróci (routery, rejestratory/kamery Hikvision, Dahua, BCMS):
- `net/http` GET, **każdy kod HTTP = urządzenie żyje** (też `401` — Hikvision/Dahua wymagają digest auth), mierzyć tylko RTT
- `InsecureSkipVerify` (samopodpisane certyfikaty), `CheckRedirect → http.ErrUseLastResponse`, timeout z `ctx`
- dla kamer/NVR sprawdzić najpierw, czy `tcp.go` z portem producenta (RTSP 554, Hikvision 8000, Dahua 37777) nie wystarcza

### 5. Testy pozostałych elementów

- `prober.New()`: znany protokół → poprawny typ, nieznany → błąd (brak testu)
- broadcaster: publish do wielu subskrybentów, drop gdy bufor pełny, unsubscribe czyści mapę (brak testu; czysta logika in-memory)
- scheduler: wywołuje prober dla każdego targetu, wyniki trafiają do kanału — wymaga wydzielenia interfejsu `TargetRepository` z `ListActiveTargets` (dziś `*db.Queries`)
- błąd DNS: probery dostają `netip.Addr`, więc DNS nie dotyczy proberów (ewentualnie przy rozwiązywaniu hosta targetu)

### 6. Porządki i dalsze

- `go mod tidy` w `services/probe-service` — `go.mod` nie wymaga `golang.org/x/net` (działa dzięki `go.work` z roota; build obrazu też, bo Dockerfile kopiuje `go.work`)
- IPv6 w ICMP (ping `udp4`, MTR/traceroute `ip4:icmp` → `ip6:ipv6-icmp`) — dziś tylko IPv4
- CI: uruchamiać `task test:probe:linux`
- Prometheus `/metrics` endpoint
