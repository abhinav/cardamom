-- name: AttachmentCloneNextBlob :one
SELECT digest, size_bytes FROM (
    SELECT a.blob_digest AS digest, a.blob_size_bytes AS size_bytes
    FROM attachments AS a WHERE a.board_id = sqlc.arg(board_id)
    UNION
    SELECT h.digest, h.size_bytes
    FROM board_mutation_blob_refs AS h WHERE h.board_id = sqlc.arg(board_id)
) WHERE digest > sqlc.arg(after_digest)
ORDER BY digest LIMIT 1;
