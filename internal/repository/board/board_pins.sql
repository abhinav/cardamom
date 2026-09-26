-- name: BoardListPinIDs :many
SELECT issue.id
FROM board_pins AS pin
JOIN issues AS issue ON issue.uid = pin.issue_uid
WHERE pin.board_id = sqlc.arg(board_id)
ORDER BY pin.position;

-- name: BoardListPinReferences :many
SELECT
    issue.id,
    issue.title,
    issue.kind,
    CAST(CASE
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
    END AS TEXT) AS status,
    issue.priority
FROM board_pins AS pin
JOIN issues AS issue
    ON issue.board_id = pin.board_id
    AND issue.uid = pin.issue_uid
LEFT JOIN active_claims AS claim ON claim.issue_uid = issue.uid
WHERE pin.board_id = sqlc.arg(board_id)
ORDER BY pin.position;

-- name: BoardGetPinIssueReference :one
SELECT
    issue.id,
    issue.title,
    issue.kind,
    CAST(CASE
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
    END AS TEXT) AS status,
    issue.priority
FROM issues AS issue
LEFT JOIN active_claims AS claim ON claim.issue_uid = issue.uid
WHERE issue.board_id = sqlc.arg(board_id)
    AND issue.id = sqlc.arg(issue_id);

-- name: BoardPinExists :one
SELECT EXISTS (
    SELECT 1
    FROM board_pins
    WHERE board_id = sqlc.arg(board_id)
        AND issue_uid = sqlc.arg(issue_uid)
);

-- name: BoardCountPins :one
SELECT count(*)
FROM board_pins
WHERE board_id = sqlc.arg(board_id);

-- name: BoardInsertPin :exec
INSERT INTO board_pins (board_id, issue_uid, position)
SELECT
    sqlc.arg(board_id),
    sqlc.arg(issue_uid),
    COALESCE(MAX(position), 0) + 1
FROM board_pins
WHERE board_id = sqlc.arg(board_id);

-- name: BoardDeletePin :execresult
DELETE FROM board_pins
WHERE board_id = sqlc.arg(board_id)
    AND issue_uid = sqlc.arg(issue_uid);
