-- name: ProjectGetRestoreProject :one
SELECT name, created_at
FROM projects
WHERE id = sqlc.arg(id);

-- name: ProjectInsertRestoredProject :exec
INSERT INTO projects (id, name, created_at)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(created_at));
