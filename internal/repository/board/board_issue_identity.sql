-- name: BoardCountAllIssues :one
SELECT COUNT(*)
FROM issues;

-- name: BoardIssueIDExists :one
SELECT EXISTS (
    SELECT 1
    FROM issues
    WHERE id = sqlc.arg(id)
) AS issue_exists;

-- name: BoardIssueUIDExists :one
SELECT EXISTS (
    SELECT 1
    FROM issues
    WHERE uid = sqlc.arg(uid)
) AS issue_exists;

-- name: BoardGetIssueUID :one
SELECT uid
FROM issues
WHERE board_id = sqlc.arg(board_id)
    AND id = sqlc.arg(id);

-- name: BoardListIssueIDs :many
SELECT id
FROM issues
WHERE board_id = sqlc.arg(board_id)
ORDER BY id;
