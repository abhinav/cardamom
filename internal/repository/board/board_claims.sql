-- name: BoardDeleteActiveClaim :exec
DELETE FROM active_claims
WHERE issue_id = sqlc.arg(issue_id);

-- name: BoardInsertActiveClaim :exec
INSERT INTO active_claims (
    issue_id,
    board_id,
    actor,
    started_at,
    started_revision
) VALUES (
    sqlc.arg(issue_id),
    sqlc.arg(board_id),
    sqlc.arg(actor),
    sqlc.arg(started_at),
    sqlc.arg(started_revision)
);
