-- name: AttachmentUpdateUploadProgress :exec
UPDATE attachment_uploads
SET accepted_offset = sqlc.arg(accepted_offset),
    expires_at = sqlc.arg(expires_at)
WHERE id = sqlc.arg(id)
    AND state = 'active';
