-- name: AttachmentCountCopyActiveUploads :one
SELECT count(*) AS active_upload_count
FROM attachment_uploads
WHERE board_id = sqlc.arg(board_id)
    AND state = 'active';

-- name: AttachmentListCopyMetadataPage :many
SELECT
    attachment.id,
    issue.id AS origin_issue_id,
    attachment.blob_digest,
    attachment.blob_size_bytes,
    attachment.filename,
    attachment.media_type,
    attachment.lifecycle,
    attachment.created_actor,
    attachment.created_at,
    attachment.removed_actor,
    attachment.removed_at
FROM attachments AS attachment
LEFT JOIN issues AS issue ON issue.uid = attachment.origin_issue_uid
WHERE attachment.board_id = sqlc.arg(board_id)
    AND attachment.id > sqlc.arg(after_id)
ORDER BY attachment.id
LIMIT sqlc.arg(page_size);

-- name: AttachmentInsertCopiedMetadata :exec
INSERT INTO attachments (
    board_id,
    id,
    origin_issue_uid,
    blob_digest,
    blob_size_bytes,
    filename,
    media_type,
    lifecycle,
    created_actor,
    created_at,
    created_revision,
    removed_actor,
    removed_at,
    removed_revision
) VALUES (
    sqlc.arg(board_id),
    sqlc.arg(id),
    sqlc.narg(origin_issue_uid),
    sqlc.arg(blob_digest),
    sqlc.arg(blob_size_bytes),
    sqlc.arg(filename),
    sqlc.arg(media_type),
    sqlc.arg(lifecycle),
    sqlc.arg(created_actor),
    sqlc.arg(created_at),
    sqlc.arg(created_revision),
    sqlc.narg(removed_actor),
    sqlc.narg(removed_at),
    sqlc.narg(removed_revision)
);
