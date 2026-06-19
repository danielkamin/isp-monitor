FROM golang:1.26-alpine AS builder

WORKDIR /workspace

COPY go.work go.work.sum ./
COPY go.mod go.sum ./
COPY services/probe-service/go.mod     services/probe-service/go.sum     ./services/probe-service/
COPY services/analytics-service/go.mod services/analytics-service/go.sum ./services/analytics-service/
COPY services/api-service/go.mod       services/api-service/go.sum       ./services/api-service/

RUN go mod download

COPY pkg/ ./pkg/
COPY services/probe-service/ ./services/probe-service/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-w -s" \
    -o /bin/probe-service \
    ./services/probe-service/cmd/probe-service

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /bin/probe-service /probe-service

EXPOSE 50051

USER nonroot:nonroot
ENTRYPOINT ["/probe-service"]
