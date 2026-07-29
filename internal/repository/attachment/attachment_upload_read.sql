-- name: AttachmentGetUpload :one
SELECT attachment_uploads.*
FROM attachment_uploads
WHERE id = sqlc.arg(id);
