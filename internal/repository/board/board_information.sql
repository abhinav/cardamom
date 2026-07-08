-- name: BoardCountInventoryIssues :one
SELECT COUNT(*)
FROM issues
WHERE board_id = sqlc.arg(board_id);

-- name: BoardCountInventoryIssuesByStatus :one
SELECT COUNT(*)
FROM issues AS issue
LEFT JOIN active_claims AS claim ON claim.issue_id = issue.id
WHERE issue.board_id = sqlc.arg(board_id)
    AND CASE
        WHEN issue.lifecycle <> 'open' THEN issue.lifecycle
        WHEN claim.issue_id IS NOT NULL THEN 'in_progress'
        WHEN issue.waiting_reason IS NOT NULL THEN 'waiting'
        WHEN EXISTS (
            SELECT 1
            FROM dependencies AS dependency
            JOIN issues AS prerequisite ON prerequisite.id = dependency.prerequisite_id
            WHERE dependency.issue_id = issue.id
                AND prerequisite.lifecycle <> 'closed'
        ) THEN 'blocked'
        ELSE 'ready'
    END = sqlc.arg(status);
