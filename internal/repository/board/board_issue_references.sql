-- name: BoardResolveIssueReferences :many
SELECT issue.id FROM issues AS issue
WHERE issue.board_id = sqlc.arg(board_id)
    AND issue.id IN (sqlc.slice('issue_ids'))
UNION ALL
SELECT marker.id FROM issue_reference_markers AS marker
WHERE marker.id IN (sqlc.slice('marker_ids'));

-- name: BoardResolveLogReferences :many
SELECT log.id, issue.id AS issue_id
FROM issue_log_entries AS log
JOIN issues AS issue ON issue.uid = log.issue_uid
WHERE log.board_id = sqlc.arg(board_id)
    AND log.id IN (sqlc.slice('log_ids'));
