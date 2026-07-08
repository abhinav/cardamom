-- name: BoardReadChangeCursor :one
SELECT revision
FROM boards
WHERE id = sqlc.arg(board_id);
