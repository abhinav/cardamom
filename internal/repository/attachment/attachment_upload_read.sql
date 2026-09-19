-- name: AttachmentGetUpload :one
SELECT upload.*, issue.id AS origin_issue_id
FROM attachment_uploads AS upload
LEFT JOIN issues AS issue ON issue.uid = upload.origin_issue_uid
WHERE upload.id = sqlc.arg(id);
