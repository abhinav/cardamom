-- name: ProjectCopyBoardIDExists :one
SELECT EXISTS (
    SELECT 1
    FROM boards
    WHERE id = sqlc.arg(id)
) AS board_exists;

-- name: ProjectCopyBoardNameExists :one
SELECT EXISTS (
    SELECT 1
    FROM boards
    WHERE project_id = sqlc.arg(project_id)
        AND name = sqlc.arg(name)
) AS board_exists;

-- name: ProjectInsertCopiedBoard :exec
INSERT INTO boards (
    id,
    project_id,
    name,
    description,
    created_at,
    issue_id_prefix,
    issue_id_strategy,
    issue_summary_max_bytes,
    attachment_max_bytes,
    revision
) VALUES (
    sqlc.arg(id),
    sqlc.arg(project_id),
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(created_at),
    sqlc.arg(issue_id_prefix),
    sqlc.arg(issue_id_strategy),
    sqlc.arg(issue_summary_max_bytes),
    sqlc.arg(attachment_max_bytes),
    sqlc.arg(revision)
);
