-- name: BoardReadBackupReferenceMarker :one
SELECT marker_id
FROM issue_reference_candidates
WHERE board_id = sqlc.arg(board_id)
ORDER BY marker_id
LIMIT 1;

-- name: BoardReadAnyBackupReferenceMarker :one
SELECT id FROM issue_reference_markers ORDER BY id LIMIT 1;
