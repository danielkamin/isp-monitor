-- name: CreateSite :one
INSERT INTO sites (name, isp_name, isp_asn)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSite :one
SELECT * FROM sites
WHERE id = $1;

-- name: ListSites :many
SELECT * FROM sites
ORDER BY name;

-- name: ListActiveSites :many
SELECT * FROM sites
WHERE active = true
ORDER BY name;

-- name: UpdateSite :one
UPDATE sites
SET name = $2, isp_name = $3, isp_asn = $4, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetSiteActive :one
UPDATE sites
SET active = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteSite :exec
DELETE FROM sites
WHERE id = $1;
