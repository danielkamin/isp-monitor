-- name: CreateTarget :one
INSERT INTO targets (site_id, name, type, ip, port, protocol)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetTarget :one
SELECT * FROM targets
WHERE id = $1;

-- name: ListTargetsBySite :many
SELECT * FROM targets
WHERE site_id = $1
ORDER BY name;

-- name: ListActiveTargetsBySite :many
SELECT * FROM targets
WHERE site_id = $1 AND active = true
ORDER BY name;

-- name: ListActiveTargets :many
SELECT * FROM targets
WHERE active = true
ORDER BY site_id, name;

-- name: UpdateTarget :one
UPDATE targets
SET name = $2, type = $3, ip = $4, port = $5, protocol = $6
WHERE id = $1
RETURNING *;

-- name: SetTargetActive :one
UPDATE targets
SET active = $2
WHERE id = $1
RETURNING *;

-- name: DeleteTarget :exec
DELETE FROM targets
WHERE id = $1;
