# probe-service — Context

## Cel

Wykonuje pomiary sieciowe (ping, TCP, traceroute, MTR, HTTP), zapisuje wyniki do PostgreSQL i streamuje je przez gRPC do subskrybentów (analytics-service).

## Architektura wewnętrzna

```
main.go
  ├─ NewScheduler → Start(ctx)        ticker → DB → goroutine per target
  ├─ prober.New(protocol)             factory zwraca właściwy Prober
  ├─ resultsChan (chan ProbeResult)    kanał między scheduler a main
  ├─ db.SaveResult()                  zapis do probe.probes (sqlc)
  ├─ broadcaster.Publish()            fan-out do aktywnych gRPC streamów
  └─ gRPC server :50051
       └─ ResultsServer.StreamResults() → subskrybuje broadcaster
```

## Przepływ danych

```
Scheduler (ticker)
  └─→ queries.ListActiveTargets()     świeże targety przy każdym tick
  └─→ for each target: go func()
        prober.New(t.Protocol).Probe(ctx, t.Ip+t.Port)
        resultsChan <- ProbeResult

main.go goroutine (for result := range resultsChan)
  ├─→ db.SaveResult(ctx, result)      zawsze, niezależnie od subskrybentów
  └─→ broadcaster.Publish(result)

Broadcaster
  └─→ każdy aktywny chan → ResultsServer.StreamResults() → analytics-service
```

## Pakiety

### `internal/prober`

- `prober.go` — interfejs `Prober` + struct `ProbeResult` (do wypełnienia)
- `factory.go` — `New(protocol ProbeProtocol) (Prober, error)` — switch po typie
- `icmp.go` — ping (ICMP) — **do implementacji**
- `tcp.go` — TCP connect na port — **do implementacji**
- `traceroute.go` — trasa do hosta — **do implementacji**
- `mtr.go` — ping + traceroute per hop — **do implementacji**
- `http.go` — HTTP GET, sprawdza kod odpowiedzi — **do implementacji**

### `internal/scheduler`

- `Scheduler` struct: `queries`, `resultsChan`, `interval`
- `NewScheduler(queries, resultsChan)` — domyślny interval: 1s (dev), docelowo ~30s
- `Start(ctx)` — ticker + `ListActiveTargets()` co tick + goroutine per target
- **Uwaga:** `log.Fatalf` w goroutine per target to zły wzorzec — powinno być `log.Printf` + `continue`, żeby jeden błędny target nie ubił całego procesu

### `internal/server`

- `Broadcaster` — mapa `map[string]chan *pb.StreamResultsResponse` + `sync.RWMutex`
  - `Subscribe(id)` → zwraca kanał, rejestruje subskrybenta
  - `Unsubscribe(id)` → **błąd: ustawia `nil` zamiast `delete(b.subscribers, id)`** — powoduje wyciek mapy
  - `Publish(r)` — drop jeśli kanał pełny (bufor 64)
- `ResultsServer` — implementuje `ResultsServiceServer`
  - `StreamResults` — rejestruje się w broadcasterze, filtruje po `site_id` jeśli podany, pętla select na kanał + `stream.Context().Done()`

### `internal/db`

- `connect.go` — `Connect(ctx, DBConfig) (*pgxpool.Pool, error)`
- `queries/` — kod generowany przez sqlc (nie edytować ręcznie)
  - `models.go` — `Target`, `Site`, `Probe`, `ProbeProtocol` enum, `DeviceType` enum
  - `targets.sql.go` — `ListActiveTargets(ctx)`
  - `probes.sql.go` — `SaveProbe(ctx, ...)` (lub odpowiednik)
  - `sites.sql.go`

## Schema bazy (probe.*)

```sql
probe.sites    — lokalizacje (id, name, isp_name, isp_asn, active)
probe.targets  — co probować (id, site_id, ip, port, protocol, active)
probe.probes   — wyniki (id, target_id, latency_ms, packet_loss, success, error_msg, created_at)
```

`probe_protocol` enum: `icmp | tcp | traceroute | mtr | http`

## Proto / gRPC

Plik: `proto/probe/v1/results.proto`, generowany do `pkg/pb/probe/v1/`.

```protobuf
service ResultsService {
  rpc StreamResults(StreamResultsRequest) returns (stream StreamResultsResponse);
}
```

`StreamResultsResponse` ma jedno `latency_ms` — przy MTR/traceroute potrzebna będzie lista hopów (`repeated HopResult hops`). Na razie nie zaimplementowane.

## Co zrobione

- [x] Struktura pakietów: config, db/connect, db/queries (sqlc), prober (interfejs + ProbeResult stub), scheduler (NewScheduler + Start), server (Broadcaster + ResultsServer)
- [x] Schema DB: sites, targets, probes + enum probe_protocol z 5 wartościami
- [x] sqlc queries: ListActiveTargets, modele
- [x] Broadcaster: Subscribe/Publish/Unsubscribe (z błędem w Unsubscribe — patrz niżej)
- [x] StreamResults: pętla select, filtr site_id, obsługa rozłączenia klienta
- [x] Scheduler: ticker, odświeżanie targetów co tick, factory prober.New()

## Do zrobienia (kolejność)

1. **Naprawić `Unsubscribe`** — `delete(b.subscribers, id)` zamiast `b.subscribers[id] = nil`
2. **Wypełnić `ProbeResult`** — pola: TargetID, SiteID, Success, LatencyMs, PacketLoss, ErrorMsg, ProbedAt
3. **Zaimplementować probery** — zacząć od `icmp.go` (najprostszy), potem `tcp.go`, `http.go`, `traceroute.go`, `mtr.go`
4. **`prober.New()` factory** — switch po `ProbeProtocol`, zwraca błąd dla nieznanych wartości
5. **Poprawić error handling w schedulerze** — `log.Fatalf` w goroutine zastąpić `log.Printf` + continue
6. **`main.go`** — wire wszystkiego: resultsChan, goroutine `for result := range resultsChan` (DB + broadcast), start gRPC server, start scheduler
7. **Proto rozszerzenie** — `repeated HopResult hops` dla MTR/traceroute (gdy dojdziesz do tych prober)
