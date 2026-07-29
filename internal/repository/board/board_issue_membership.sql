-- name: BoardGetIssueBoardID :one
SELECT board_id
FROM issues
WHERE id = sqlc.arg(id);
