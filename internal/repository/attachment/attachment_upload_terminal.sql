-- name: AttachmentRetainBlob :exec
INSERT INTO attachment_blobs (
    digest,
    size_bytes
) VALUES (
    sqlc.arg(digest),
    sqlc.arg(size_bytes)
)
ON CONFLICT(digest) DO NOTHING;

-- name: AttachmentInsertMetadata :exec
INSERT INTO attachments (
    board_id,
    id,
    origin_issue_id,
    blob_digest,
    blob_size_bytes,
    filename,
    media_type,
    lifecycle,
    created_actor,
    created_at,
    created_revision
) VALUES (
    sqlc.arg(board_id),
    sqlc.arg(id),
    sqlc.narg(origin_issue_id),
    sqlc.arg(blob_digest),
    sqlc.arg(blob_size_bytes),
    sqlc.arg(filename),
    sqlc.arg(media_type),
    'active',
    sqlc.arg(created_actor),
    sqlc.arg(created_at),
    sqlc.arg(created_revision)
);

-- name: AttachmentCommitUploadReceipt :exec
UPDATE attachment_uploads
SET state = 'committed',
    expires_at = sqlc.arg(expires_at),
    attachment_id = sqlc.arg(attachment_id)
WHERE id = sqlc.arg(id)
    AND state = 'active';

-- name: AttachmentAbortUploadReceipt :exec
UPDATE attachment_uploads
SET state = 'aborted',
    expires_at = sqlc.arg(expires_at)
WHERE id = sqlc.arg(id)
    AND state = 'active';
