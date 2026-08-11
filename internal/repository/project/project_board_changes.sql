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

-- name: ProjectArchiveBoard :exec
-- Archive metadata is one logical value protected by the table invariant.
UPDATE boards
SET archived_at = sqlc.arg(archived_at),
    archived_by = sqlc.arg(archived_by),
    archive_reason = sqlc.narg(archive_reason)
WHERE id = sqlc.arg(id);

-- name: ProjectUnarchiveBoard :exec
-- Clearing all archive columns restores the active representation.
UPDATE boards
SET archived_at = NULL, archived_by = NULL, archive_reason = NULL
WHERE id = sqlc.arg(id);
