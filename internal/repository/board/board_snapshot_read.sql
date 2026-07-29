-- name: BoardGetSnapshotDescription :one
SELECT description
FROM boards
WHERE id = sqlc.arg(board_id);

-- name: BoardListSnapshotResults :many
SELECT issue_id, body
FROM issue_results
WHERE board_id = sqlc.arg(board_id)
ORDER BY issue_id;

-- name: BoardListSnapshotLogEntries :many
SELECT id, issue_id, kind, author, committer, body, next_action, created_at
FROM issue_log_entries
WHERE board_id = sqlc.arg(board_id)
ORDER BY local_sequence;
