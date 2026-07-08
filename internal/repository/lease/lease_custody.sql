-- name: LeaseAcquire :one
INSERT INTO leases (
    name,
    owner,
    acquired_at,
    expires_at
) VALUES (
    sqlc.arg(name),
    sqlc.arg(owner),
    sqlc.arg(acquired_at),
    sqlc.arg(expires_at)
)
ON CONFLICT(name) DO UPDATE SET
    owner = excluded.owner,
    acquired_at = excluded.acquired_at,
    expires_at = excluded.expires_at
WHERE leases.expires_at <= excluded.acquired_at
RETURNING name, owner, acquired_at, expires_at;

-- name: LeaseRenew :one
UPDATE leases
SET expires_at = sqlc.arg(expires_at)
WHERE name = sqlc.arg(name)
    AND owner = sqlc.arg(owner)
    AND expires_at > sqlc.arg(now)
RETURNING name, owner, acquired_at, expires_at;

-- name: LeaseRelease :one
DELETE FROM leases
WHERE name = sqlc.arg(name)
    AND owner = sqlc.arg(owner)
    AND expires_at > sqlc.arg(now)
RETURNING name, owner, acquired_at, expires_at;

-- name: LeaseRevoke :one
DELETE FROM leases
WHERE name = sqlc.arg(name)
    AND owner = sqlc.arg(owner)
    AND expires_at > sqlc.arg(now)
RETURNING name, owner, acquired_at, expires_at;

-- name: LeaseGetActive :one
SELECT name, owner, acquired_at, expires_at
FROM leases
WHERE name = sqlc.arg(name)
    AND expires_at > sqlc.arg(now);

-- name: LeaseListActive :many
SELECT name, owner, acquired_at, expires_at
FROM leases
WHERE expires_at > sqlc.arg(now)
ORDER BY name ASC;
