-- name: AttachmentTargetBoardExists :one
SELECT EXISTS (
    SELECT 1
    FROM boards
    WHERE id = sqlc.arg(board_id)
) AS board_exists;

-- name: AttachmentTargetIssueExists :one
SELECT EXISTS (
    SELECT 1
    FROM issues
    WHERE board_id = sqlc.arg(board_id)
        AND id = sqlc.arg(issue_id)
) AS issue_exists;

-- name: AttachmentInsertUpload :exec
INSERT INTO attachment_uploads (
    id,
    board_id,
    origin_issue_id,
    filename,
    expected_size_bytes,
    expected_digest,
    actor,
    state,
    accepted_offset,
    expires_at,
    admitted_max_bytes
) VALUES (
    sqlc.arg(id),
    sqlc.arg(board_id),
    sqlc.narg(origin_issue_id),
    sqlc.arg(filename),
    sqlc.narg(expected_size_bytes),
    sqlc.narg(expected_digest),
    sqlc.arg(actor),
    sqlc.arg(state),
    sqlc.arg(accepted_offset),
    sqlc.arg(expires_at),
    sqlc.arg(admitted_max_bytes)
);
