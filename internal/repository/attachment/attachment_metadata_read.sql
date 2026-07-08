-- name: AttachmentGetMetadata :one
SELECT attachments.*
FROM attachments
WHERE board_id = sqlc.arg(board_id)
    AND id = sqlc.arg(id);

-- name: AttachmentListMetadata :many
SELECT attachments.*
FROM attachments
WHERE board_id = sqlc.arg(board_id)
    AND id > sqlc.arg(after_id)
    AND (sqlc.arg(include_removed) OR lifecycle = 'active')
    AND (
        NOT sqlc.arg(has_origin_issue)
        OR origin_issue_id = sqlc.narg(origin_issue_id)
    )
ORDER BY id
LIMIT sqlc.arg(result_limit);

-- name: AttachmentResolveMetadata :many
SELECT attachments.*
FROM attachments
WHERE board_id = sqlc.arg(board_id)
    AND id IN (sqlc.slice('attachment_ids'));
