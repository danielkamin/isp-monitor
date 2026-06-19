# ISP Monitor — Project Context

## Cel projektu

System monitorujący połączenia internetowe — pinguje lub sprawdza w inny sposób adresy/miejsca sieciowe, zapisuje wyniki, a następnie generuje raporty (failure rate per ISP, per lokalizacja). Projekt jest przede wszystkim środowiskiem nauki, a nie systemem produkcyjnym.

**Technologie do ćwiczenia:** Go, sqlc, protokoły internetowe (ICMP/ping, HTTP, DNS), Docker, mikroserwisy, gRPC, PostgreSQL.

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
├── services/
│   ├── probe-service/               # module: github.com/danielkamin/probe-service
│   ├── analytics-service/           # module: github.com/danielkamin/analytics-service
│   └── api-service/                 # module: github.com/danielkamin/api-service
├── deployments/
│   ├── docker/                      # Dockerfile per serwis
│   └── postgres/                    # init scripts (schematy, role, granty)
├── docker-compose.yml
└── Taskfile.yml
```

### Serwisy

| Serwis | Port | DB user | Rola |
|---|---|---|---|
| probe-service | gRPC :50051 | probe_user | wykonuje pomiary, zapisuje wyniki |
| analytics-service | gRPC :50052 | analytics_user | agreguje dane, liczy statystyki |
| api-service | HTTP :8080 | api_user (read-only) | serwuje dane do klientów |

`storage-service` — zdecydowano że jest zbędny bez zdalnych agentów RPi. Może być zastąpiony `alert-service` lub `scheduler-service` w przyszłości.

---

## Baza danych

### Strategia: schema-per-service w jednej instancji PostgreSQL 17

Każdy serwis ma własny schemat i usera z uprawnieniami tylko do swojego schematu. `api_user` ma `SELECT` na schematach `probe` i `analytics`.

```
schematy: probe | analytics | api
```

Init scripts uruchamiane automatycznie przy pierwszym `docker compose up`:
- `deployments/postgres/01_schemas.sql` — CREATE SCHEMA
- `deployments/postgres/02_roles.sql` — CREATE USER + GRANT + ALTER DEFAULT PRIVILEGES

**Kluczowe:** `ALTER DEFAULT PRIVILEGES FOR ROLE probe_user IN SCHEMA probe GRANT SELECT ON TABLES TO api_user` — bez tego `api_user` nie widzi tabel tworzonych przez `probe_user`.

### Tabele serwisów

Migracje tabel (bez goose — nie potrzeba na tym etapie) trzymane w `services/*/sql/schema/`. Uruchamiane ręcznie przez:
```
task db:migrate:probe
```

Pliki używają `CREATE TABLE IF NOT EXISTS` — bezpieczne do wielokrotnego uruchamiania.

### probe-service schema (obecny stan)

```sql
probe.sites    — lokalizacje/miejsca do monitorowania (IP, ISP, ASN)
probe.probes   — wyniki pomiarów (latency_ms, packet_loss, success) z FK do sites
```

DSN z `search_path=probe` — zapytania bez prefiksu schematu.

---

## sqlc

Konfiguracja w `services/probe-service/sqlc.yaml`:
- engine: postgresql
- sql_package: pgx/v5
- queries: `sql/queries/` → generuje do `internal/db/queries/`
- schema: `sql/schema/` (te same pliki co migracje)

Generowanie: `task sqlc:probe` (używa lokalnego `sqlc generate`, nie Docker — sqlc zainstalowany z `CGO_ENABLED=0`).

---

## Wzorce i decyzje

### Sekrety / konfiguracja

`pkg/secrets.Provider` interface z `Get(key string) (string, error)`. Factory (`secrets.New()`) wybiera provider na podstawie `APP_ENV`:
- `development` → `EnvProvider` (ładuje `.env` plik, nie failuje gdy brak pliku)
- `production` → `AWSProvider` (stub, do zaimplementowania)

Każdy serwis ma swój `internal/config/config.go` z typed `Config` struct ładowanym przez `config.Load(provider)`. Paczka `pkg/config` to tylko helpery (`LoadString`, `LoadStringOrDefault`).

### Połączenie DB

`internal/db/connect.go` — funkcja `Connect(ctx, config.DBConfig) (*pgxpool.Pool, error)` z pgxpool. Przyjmuje `DBConfig` (nie cały `Config`) — db package nie powinien znać konfiguracji gRPC.

### Per-serwis `.env`

Każdy serwis ma `services/*/  .env` (z `.env.example`). W `docker-compose.yml` `env_file:` wskazuje na plik serwisu. `DB_HOST=postgres` — Docker DNS resolwuje nazwę serwisu compose.

### Docker build

Build context to root repozytorium (`.`) — konieczne ze względu na go.work i `pkg/` współdzielone między serwisami. Dockerfile kopiuje selektywnie:
```dockerfile
COPY go.work go.work.sum ./
COPY go.mod go.sum ./
COPY services/probe-service/go.mod services/probe-service/go.sum ./services/probe-service/
RUN go mod download
COPY pkg/ ./pkg/
COPY services/probe-service/ ./services/probe-service/
```

Obraz bazowy: `gcr.io/distroless/static-debian12:nonroot` (minimalistyczny, bez shella).

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

## Co jeszcze nie jest zrobione

- [ ] Implementacja logiki probingowej w probe-service (ICMP ping, HTTP check, DNS check)
- [ ] gRPC server w probe-service
- [ ] analytics-service — schemat i logika agregacji
- [ ] api-service — HTTP server (prawdopodobnie Chi lub stdlib)
- [ ] AWSProvider w pkg/secrets
- [ ] Raporty failure rate per ISP (główny cel analityczny)
- [ ] Prawdopodobnie: alert-service lub scheduler-service jako czwarty serwis
