FROM golang:1.26-alpine AS builder

WORKDIR /workspace

COPY go.work go.work.sum ./
COPY go.mod go.sum ./
COPY services/probe-service/go.mod     services/probe-service/go.sum     ./services/probe-service/
COPY services/analytics-service/go.mod services/analytics-service/go.sum ./services/analytics-service/
COPY services/api-service/go.mod       services/api-service/go.sum       ./services/api-service/

RUN go mod download

COPY pkg/ ./pkg/
COPY services/analytics-service/ ./services/analytics-service/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-w -s" \
    -o /bin/analytics-service \
    ./services/analytics-service/cmd/analytics-service

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /bin/analytics-service /analytics-service

EXPOSE 50052

USER nonroot:nonroot
ENTRYPOINT ["/analytics-service"]
