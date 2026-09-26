-- name: BoardCountInventoryIssues :one
SELECT COUNT(*)
FROM issues
WHERE board_id = sqlc.arg(board_id);

-- name: BoardCountInventoryIssuesByStatus :one
SELECT COUNT(*)
FROM issues AS issue
LEFT JOIN active_claims AS claim ON claim.issue_uid = issue.uid
WHERE issue.board_id = sqlc.arg(board_id)
    AND CASE
        WHEN issue.lifecycle <> 'open' THEN issue.lifecycle
        WHEN claim.issue_uid IS NOT NULL THEN 'in_progress'
        WHEN issue.waiting_reason IS NOT NULL THEN 'waiting'
        WHEN EXISTS (
            SELECT 1
            FROM dependencies AS dependency
            JOIN issues AS prerequisite
                ON prerequisite.uid = dependency.prerequisite_uid
            WHERE dependency.issue_uid = issue.uid
                AND prerequisite.lifecycle <> 'closed'
        ) THEN 'blocked'
        ELSE 'ready'
    END = sqlc.arg(status);
