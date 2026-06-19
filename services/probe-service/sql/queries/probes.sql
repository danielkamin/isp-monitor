-- name: CreateProbe :one
INSERT INTO probes (target_id, latency_ms, packet_loss, success, error_msg)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;
