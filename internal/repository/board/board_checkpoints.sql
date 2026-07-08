-- name: BoardInsertCheckpointDecision :exec
INSERT INTO checkpoint_decisions (
    issue_id,
    board_id,
    outcome,
    reason,
    decided_at,
    revision
) VALUES (
    sqlc.arg(issue_id),
    sqlc.arg(board_id),
    sqlc.arg(outcome),
    sqlc.arg(reason),
    sqlc.arg(decided_at),
    sqlc.arg(revision)
);
