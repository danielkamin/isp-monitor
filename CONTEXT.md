# ISP Monitor — Project Context

## Cel projektu

System monitorujący połączenia internetowe — pinguje lub sprawdza w inny sposób adresy/miejsca sieciowe, zapisuje wyniki, a następnie generuje raporty (failure rate per ISP, per lokalizacja). Projekt jest przede wszystkim środowiskiem nauki, a nie systemem produkcyjnym.

**Technologie do ćwiczenia:** Go, sqlc, protokoły internetowe (ICMP/ping, TCP, HTTP, traceroute, MTR), Docker, mikroserwisy, gRPC, PostgreSQL, Prometheus, Loki, Grafana.

---

## Architektura

### Monorepo — Go Workspace

```
isp-monitor/
├── go.work                          # łączy root module z serwisami
├── go.work.sum
├── go.mod                           # module: github.com/danielkamin/isp-monitor
├── pkg/
│   ├── config/                      # helpers: LoadString, LoadStringOrDefault
│   └── secrets/                     # Provider interface + EnvProvider + AWSProvider (stub)
├── proto/
│   └── probe/v1/results.proto       # gRPC kontrakt probe-service
├── services/
│   ├── probe-service/               # module: github.com/danielkamin/probe-service
│   ├── analytics-service/           # module: github.com/danielkamin/analytics-service
│   └── api-service/                 # module: github.com/danielkamin/api-service
├── deployments/
│   ├── docker/                      # Dockerfile per serwis
│   ├── postgres/                    # init scripts (schematy, role, granty)
│   └── monitoring/                  # konfiguracje Prometheus, Loki, Grafana, Promtail
├── docker-compose.yml
└── Taskfile.yml
```

### Serwisy

| Serwis | Port | DB user | Rola |
|---|---|---|---|
| probe-service | gRPC :50051 | probe_user | wykonuje pomiary, zapisuje wyniki, streamuje do analytics |
| analytics-service | gRPC :50052 | analytics_user | agreguje dane, liczy statystyki, tworzy incydenty |
| api-service | HTTP :8080 | api_user (read-only) | serwuje dane do klientów |

---

## Baza danych

### Strategia: schema-per-service w jednej instancji PostgreSQL 17

Każdy serwis ma własny schemat i usera z uprawnieniami tylko do swojego schematu.

```
schematy: probe | analytics | api
```

Init scripts uruchamiane automatycznie przy pierwszym `docker compose up`:
- `deployments/postgres/01_schemas.sql` — CREATE SCHEMA
- `deployments/postgres/02_roles.sql` — CREATE USER + GRANT + ALTER DEFAULT PRIVILEGES

**Uprawnienia cross-schema:**
- `api_user` — SELECT na `probe` i `analytics`
- `analytics_user` — SELECT na `probe` (żeby joinować sites/targets/isp_providers bez duplikowania danych)
- `ALTER DEFAULT PRIVILEGES FOR ROLE probe_user IN SCHEMA probe GRANT SELECT ON TABLES TO analytics_user` — konieczne żeby analytics_user widział nowe tabele probe tworzone przez probe_user

### probe schema — obecny stan

```sql
probe.isp_providers  — dostawcy internetu (id, name, asn, country)
probe.sites          — lokalizacje (id, name, isp_provider_id FK, active)
probe.targets        — co probować (id, site_id, ip, port, protocol, active)
probe.probes         — wyniki pomiarów (id, target_id, latency_ms, packet_loss, success, error_msg, created_at)
```

`probe_protocol` enum: `icmp | tcp | traceroute | mtr | http`

Migracje tabel w `services/*/sql/schema/`, uruchamiane ręcznie: `task db:migrate:probe`

DSN z `search_path=probe` — zapytania bez prefiksu schematu.

---

## Monitoring stack

Serwisy w `docker-compose.yml`, konfiguracje w `deployments/monitoring/`:

| Serwis | Port | Rola |
|---|---|---|
| Prometheus | :9090 | zbiera metryki (scrape) |
| Loki | :3100 | agreguje logi |
| Grafana | :3000 | dashboardy (admin/admin) |
| Promtail | — | czyta docker stdout → wysyła do Loki |

Promtail automatycznie dodaje label `container=<nazwa>` do logów — bez zmian w kodzie Go.

**Logowanie w serwisach:** `log/slog` z `JSONHandler` (stdout → Promtail → Loki). Logger tworzony w `main.go`, przekazywany przez DI do konstruktorów. Per-komponent kontekst przez `logger.With("component", "scheduler")`.

---

## sqlc

Konfiguracja w `services/probe-service/sqlc.yaml`:
- engine: postgresql, sql_package: pgx/v5
- queries: `sql/queries/` → generuje do `internal/db/queries/`
- schema: `sql/schema/`

Generowanie: `task sqlc:probe`

---

## Wzorce i decyzje

### Sekrety / konfiguracja

`pkg/secrets.Provider` interface z `Get(key string) (string, error)`. Factory (`secrets.New()`) wybiera provider na podstawie `APP_ENV`:
- `development` → `EnvProvider` (ładuje `.env` plik)
- `production` → `AWSProvider` (stub, do zaimplementowania)

### Połączenie DB

`internal/db/connect.go` — `Connect(ctx, config.DBConfig) (*pgxpool.Pool, error)`. Przyjmuje `DBConfig` (nie cały `Config`).

### Docker build

Build context to root repozytorium — konieczne ze względu na `go.work` i `pkg/`. Obraz bazowy: `gcr.io/distroless/static-debian12:nonroot`.

---

## Taskfile — kluczowe komendy

```
task dev:up              # docker compose up -d
task dev:down            # docker compose down (zachowuje dane)
task dev:reset           # docker compose down -v && up (czysta baza)
task dev:logs -- <serwis>
task db:shell            # psql jako admin
task db:migrate:probe    # uruchamia sql/schema/*.sql dla probe-service
task sqlc:probe          # generuje kod Go z SQL
task build:probe         # go build
```

---

## Co zrobione

- [x] Monorepo: go.work, pkg/secrets, pkg/config, Dockerfiles, docker-compose
- [x] PostgreSQL: schematy, role, granty (w tym cross-schema dla analytics_user i api_user)
- [x] Monitoring stack: Prometheus, Loki, Grafana, Promtail w docker-compose + pliki config
- [x] probe-service: scheduler, broadcaster, gRPC ResultsServer, sqlc queries, prober interface
- [x] probe schema: isp_providers, sites, targets, probes + probe_protocol enum

## Co jeszcze nie jest zrobione

- [ ] probe-service: implementacja proberów (icmp, tcp, http, traceroute, mtr), wire main.go, ProbeResult struct
- [ ] analytics-service: schema DB, logika agregacji, obsługa incydentów, klient gRPC probe-service
- [ ] api-service: HTTP server (Chi lub stdlib), endpointy raportów
- [ ] AWSProvider w pkg/secrets
- [ ] Prometheus metryki w serwisach Go (`/metrics` endpoint)
- [ ] Raporty failure rate per ISP (główny cel analityczny)
