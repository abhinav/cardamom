-- name: AttachmentExpireUpload :exec
UPDATE attachment_uploads
SET state = 'expired',
    expires_at = sqlc.arg(expires_at)
WHERE id = sqlc.arg(id)
    AND state = 'active';
