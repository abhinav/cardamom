-- name: BoardReadBackupConflict :one
SELECT id FROM board_conflicts
WHERE board_id = sqlc.arg(board_id)
ORDER BY id LIMIT 1;

-- name: BoardReadAnyBackupConflict :one
SELECT id FROM board_conflicts ORDER BY id LIMIT 1;
