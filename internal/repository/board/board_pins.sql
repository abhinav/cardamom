-- name: BoardListPinIDs :many
SELECT issue_id
FROM board_pins
WHERE board_id = sqlc.arg(board_id)
ORDER BY position;

-- name: BoardListPinReferences :many
SELECT
    issue.id,
    issue.title,
    issue.kind,
    CAST(CASE
        WHEN issue.lifecycle <> 'open' THEN issue.lifecycle
        WHEN claim.issue_id IS NOT NULL THEN 'in_progress'
        WHEN issue.waiting_reason IS NOT NULL THEN 'waiting'
        WHEN EXISTS (
            SELECT 1
            FROM dependencies AS dependency
            JOIN issues AS prerequisite
                ON prerequisite.id = dependency.prerequisite_id
            WHERE dependency.issue_id = issue.id
                AND prerequisite.lifecycle <> 'closed'
        ) THEN 'blocked'
        ELSE 'ready'
    END AS TEXT) AS status,
    issue.priority
FROM board_pins AS pin
JOIN issues AS issue
    ON issue.board_id = pin.board_id
    AND issue.id = pin.issue_id
LEFT JOIN active_claims AS claim ON claim.issue_id = issue.id
WHERE pin.board_id = sqlc.arg(board_id)
ORDER BY pin.position;

-- name: BoardGetPinIssueReference :one
SELECT
    issue.id,
    issue.title,
    issue.kind,
    CAST(CASE
        WHEN issue.lifecycle <> 'open' THEN issue.lifecycle
        WHEN claim.issue_id IS NOT NULL THEN 'in_progress'
        WHEN issue.waiting_reason IS NOT NULL THEN 'waiting'
        WHEN EXISTS (
            SELECT 1
            FROM dependencies AS dependency
            JOIN issues AS prerequisite
                ON prerequisite.id = dependency.prerequisite_id
            WHERE dependency.issue_id = issue.id
                AND prerequisite.lifecycle <> 'closed'
        ) THEN 'blocked'
        ELSE 'ready'
    END AS TEXT) AS status,
    issue.priority
FROM issues AS issue
LEFT JOIN active_claims AS claim ON claim.issue_id = issue.id
WHERE issue.board_id = sqlc.arg(board_id)
    AND issue.id = sqlc.arg(issue_id);

-- name: BoardPinExists :one
SELECT EXISTS (
    SELECT 1
    FROM board_pins
    WHERE board_id = sqlc.arg(board_id)
        AND issue_id = sqlc.arg(issue_id)
);

-- name: BoardCountPins :one
SELECT count(*)
FROM board_pins
WHERE board_id = sqlc.arg(board_id);

-- name: BoardInsertPin :exec
INSERT INTO board_pins (board_id, issue_id, position)
SELECT
    sqlc.arg(board_id),
    sqlc.arg(issue_id),
    COALESCE(MAX(position), 0) + 1
FROM board_pins
WHERE board_id = sqlc.arg(board_id);

-- name: BoardDeletePin :execresult
DELETE FROM board_pins
WHERE board_id = sqlc.arg(board_id)
    AND issue_id = sqlc.arg(issue_id);
