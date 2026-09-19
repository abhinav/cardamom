-- name: BoardGetSnapshotDescription :one
SELECT description
FROM boards
WHERE id = sqlc.arg(board_id);

-- name: BoardListSnapshotResults :many
SELECT issue.id AS issue_id, result.body
FROM issue_results AS result
JOIN issues AS issue ON issue.uid = result.issue_uid
WHERE result.board_id = sqlc.arg(board_id)
ORDER BY issue.id;

-- name: BoardListSnapshotLogEntries :many
SELECT log.id, issue.id AS issue_id, log.kind, log.author, log.committer,
    log.body, log.next_action, log.created_at
FROM issue_log_entries AS log
JOIN issues AS issue ON issue.uid = log.issue_uid
WHERE log.board_id = sqlc.arg(board_id)
ORDER BY log.local_sequence;
