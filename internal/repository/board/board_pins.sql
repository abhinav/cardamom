-- name: BoardListPinIDs :many
SELECT issue.id
FROM board_pins AS pin
JOIN issues AS issue ON issue.uid = pin.issue_uid
WHERE pin.board_id = sqlc.arg(board_id)
ORDER BY pin.position, pin.issue_uid;

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

-- name: BoardInsertPin :one
INSERT INTO board_pins (board_id, issue_uid, position)
SELECT
    sqlc.arg(board_id),
    sqlc.arg(issue_uid),
    COALESCE(MAX(position), 0) + 1
FROM board_pins
WHERE board_id = sqlc.arg(board_id)
RETURNING position;

-- name: BoardDeletePin :execresult
DELETE FROM board_pins
WHERE board_id = sqlc.arg(board_id)
    AND issue_uid = sqlc.arg(issue_uid);

-- name: BoardInsertPinAtPosition :exec
INSERT INTO board_pins (board_id, issue_uid, position)
VALUES (sqlc.arg(board_id), sqlc.arg(issue_uid), sqlc.arg(position));

-- name: BoardReadPinConflict :one
SELECT id FROM board_conflicts
WHERE board_id = sqlc.arg(board_id) AND kind = 'board.pin'
ORDER BY id LIMIT 1;
