-- name: BoardResolveIssueReferences :many
SELECT id
FROM issues
WHERE board_id = sqlc.arg(board_id)
    AND id IN (sqlc.slice('issue_ids'));

-- name: BoardResolveLogReferences :many
SELECT log.id, issue.id AS issue_id
FROM issue_log_entries AS log
JOIN issues AS issue ON issue.uid = log.issue_uid
WHERE log.board_id = sqlc.arg(board_id)
    AND log.id IN (sqlc.slice('log_ids'));
