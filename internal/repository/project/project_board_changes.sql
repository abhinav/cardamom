-- name: ProjectExists :one
SELECT EXISTS (
    SELECT 1
    FROM projects
    WHERE id = sqlc.arg(id)
) AS project_exists;

-- name: ProjectCreateBoard :exec
INSERT INTO boards (
    id,
    project_id,
    name,
    description,
    created_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(project_id),
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(created_at)
);

-- name: ProjectUpdateBoardSettings :exec
UPDATE boards
SET name = sqlc.arg(name),
    description = sqlc.narg(description)
WHERE id = sqlc.arg(id);
