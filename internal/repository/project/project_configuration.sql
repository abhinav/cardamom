-- name: ProjectGetConfigurationLayers :one
SELECT
    p.id AS project_id,
    p.issue_id_prefix AS project_issue_id_prefix,
    p.issue_id_strategy AS project_issue_id_strategy,
    p.issue_summary_max_bytes AS project_issue_summary_max_bytes,
    p.attachment_max_bytes AS project_attachment_max_bytes,
    b.issue_id_prefix AS board_issue_id_prefix,
    b.issue_id_strategy AS board_issue_id_strategy,
    b.issue_summary_max_bytes AS board_issue_summary_max_bytes,
    b.attachment_max_bytes AS board_attachment_max_bytes
FROM boards AS b
JOIN projects AS p ON p.id = b.project_id
WHERE b.id = sqlc.arg(board_id);

-- name: ProjectGetProjectConfiguration :one
SELECT
    issue_id_prefix,
    issue_id_strategy,
    issue_summary_max_bytes,
    attachment_max_bytes
FROM projects
WHERE id = sqlc.arg(id);

-- name: ProjectUpdateProjectConfiguration :exec
UPDATE projects
SET issue_id_prefix = sqlc.narg(issue_id_prefix),
    issue_id_strategy = sqlc.narg(issue_id_strategy),
    issue_summary_max_bytes = sqlc.narg(issue_summary_max_bytes),
    attachment_max_bytes = sqlc.narg(attachment_max_bytes)
WHERE id = sqlc.arg(id);

-- name: ProjectGetBoardConfiguration :one
-- The board configuration writer reads lifecycle in the same transaction as
-- the values it may replace.
SELECT
    issue_id_prefix,
    issue_id_strategy,
    issue_summary_max_bytes,
    attachment_max_bytes,
    archived_at
FROM boards
WHERE id = sqlc.arg(id);

-- name: ProjectUpdateBoardConfiguration :exec
UPDATE boards
SET issue_id_prefix = sqlc.narg(issue_id_prefix),
    issue_id_strategy = sqlc.narg(issue_id_strategy),
    issue_summary_max_bytes = sqlc.narg(issue_summary_max_bytes),
    attachment_max_bytes = sqlc.narg(attachment_max_bytes)
WHERE id = sqlc.arg(id);
