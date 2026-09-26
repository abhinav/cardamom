-- name: BoardDeleteActiveClaim :exec
DELETE FROM active_claims
WHERE issue_uid = sqlc.arg(issue_uid);

-- name: BoardInsertActiveClaim :exec
INSERT INTO active_claims (
    issue_uid,
    board_id,
    actor,
    started_at,
    started_revision
) VALUES (
    sqlc.arg(issue_uid),
    sqlc.arg(board_id),
    sqlc.arg(actor),
    sqlc.arg(started_at),
    sqlc.arg(started_revision)
);
