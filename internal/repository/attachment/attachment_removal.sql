-- name: AttachmentTombstoneMetadata :exec
UPDATE attachments
SET lifecycle = 'removed',
    removed_actor = sqlc.arg(removed_actor),
    removed_at = sqlc.arg(removed_at),
    removed_revision = sqlc.arg(removed_revision)
WHERE board_id = sqlc.arg(board_id)
    AND id = sqlc.arg(id)
    AND lifecycle = 'active';
