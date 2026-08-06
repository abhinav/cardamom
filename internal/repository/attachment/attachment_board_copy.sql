-- name: AttachmentCountCopyActiveUploads :one
SELECT count(*) AS active_upload_count
FROM attachment_uploads
WHERE board_id = sqlc.arg(board_id)
    AND state = 'active';

-- name: AttachmentListCopyMetadataPage :many
SELECT
    id,
    origin_issue_id,
    blob_digest,
    blob_size_bytes,
    filename,
    media_type,
    lifecycle,
    created_actor,
    created_at,
    removed_actor,
    removed_at
FROM attachments
WHERE board_id = sqlc.arg(board_id)
    AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(page_size);

-- name: AttachmentInsertCopiedMetadata :exec
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
    created_revision,
    removed_actor,
    removed_at,
    removed_revision
) VALUES (
    sqlc.arg(board_id),
    sqlc.arg(id),
    sqlc.narg(origin_issue_id),
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
