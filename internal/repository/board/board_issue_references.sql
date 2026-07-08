-- name: BoardResolveIssueReferences :many
SELECT id
FROM issues
WHERE board_id = sqlc.arg(board_id)
    AND id IN (sqlc.slice('issue_ids'));

-- name: BoardResolveLogReferences :many
SELECT id, issue_id
FROM issue_log_entries
WHERE board_id = sqlc.arg(board_id)
    AND id IN (sqlc.slice('log_ids'));
