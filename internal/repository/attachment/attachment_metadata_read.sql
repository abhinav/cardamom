-- name: AttachmentGetMetadata :one
SELECT attachment.*, issue.id AS origin_issue_id
FROM attachments AS attachment
LEFT JOIN issues AS issue ON issue.uid = attachment.origin_issue_uid
WHERE attachment.board_id = sqlc.arg(board_id)
    AND attachment.id = sqlc.arg(id);

-- name: AttachmentListMetadata :many
SELECT attachment.*, issue.id AS origin_issue_id
FROM attachments AS attachment
LEFT JOIN issues AS issue ON issue.uid = attachment.origin_issue_uid
WHERE attachment.board_id = sqlc.arg(board_id)
    AND attachment.id > sqlc.arg(after_id)
    AND (sqlc.arg(include_removed) OR attachment.lifecycle = 'active')
    AND (
        NOT sqlc.arg(has_origin_issue)
        OR attachment.origin_issue_uid = sqlc.narg(origin_issue_uid)
    )
ORDER BY attachment.id
LIMIT sqlc.arg(result_limit);

-- name: AttachmentResolveMetadata :many
SELECT attachment.*, issue.id AS origin_issue_id
FROM attachments AS attachment
LEFT JOIN issues AS issue ON issue.uid = attachment.origin_issue_uid
WHERE attachment.board_id = sqlc.arg(board_id)
    AND attachment.id IN (sqlc.slice('attachment_ids'));
