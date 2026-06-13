# ── ETAP 1: Builder ──────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-w -s -X main.version=$(git describe --tags --always)" \
    -o /bin/probe-service \
    ./cmd/probe-service

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /bin/probe-service /probe-service

COPY --from=builder /app/db/migrations /migrations

EXPOSE 50051

USER nonroot:nonroot
ENTRYPOINT ["/storage-service"]