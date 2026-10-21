-- name: ProjectGetCloneBoard :one
SELECT id, name, description, created_at, archived_at, archived_by,
    archive_reason, issue_id_prefix, issue_id_strategy,
    issue_summary_max_bytes, attachment_max_bytes, board_pins_max_count
FROM boards WHERE id = sqlc.arg(board_id);

-- name: ProjectInsertClonedBoard :exec
INSERT INTO boards (
    id, project_id, name, description, created_at,
    archived_at, archived_by, archive_reason,
    issue_id_prefix, issue_id_strategy, issue_summary_max_bytes,
    attachment_max_bytes, board_pins_max_count, revision
) VALUES (
    sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(name),
    sqlc.narg(description), sqlc.arg(created_at), sqlc.narg(archived_at),
    sqlc.narg(archived_by), sqlc.narg(archive_reason),
    sqlc.narg(issue_id_prefix), sqlc.narg(issue_id_strategy),
    sqlc.narg(issue_summary_max_bytes), sqlc.narg(attachment_max_bytes),
    sqlc.narg(board_pins_max_count), sqlc.arg(revision)
);
