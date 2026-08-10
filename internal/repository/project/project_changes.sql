-- name: ProjectGetProject :one
SELECT id, name, created_at
FROM projects
WHERE id = sqlc.arg(id);

-- name: ProjectUpdateProjectName :exec
UPDATE projects
SET name = sqlc.arg(name)
WHERE id = sqlc.arg(id);
