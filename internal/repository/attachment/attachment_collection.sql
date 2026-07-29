-- name: AttachmentListCollectibleUploads :many
SELECT id
FROM attachment_uploads
WHERE state <> 'active'
    OR (
        sqlc.arg(include_expired_active)
        AND state = 'active'
        AND expires_at <= sqlc.arg(now)
    )
ORDER BY id;

-- name: AttachmentDeleteExpiredUploadReceipts :exec
DELETE FROM attachment_uploads
WHERE state <> 'active'
    AND expires_at <= sqlc.arg(now);

-- name: AttachmentExpireActiveUploads :exec
UPDATE attachment_uploads
SET state = 'expired',
    expires_at = sqlc.arg(receipt_expires_at)
WHERE state = 'active'
    AND expires_at <= sqlc.arg(now);

-- name: AttachmentListRetainedBlobs :many
SELECT board_id, id, blob_digest, blob_size_bytes
FROM attachments
ORDER BY board_id, id;
