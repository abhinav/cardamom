-- name: BoardGetCopyReceipt :one
SELECT
    id,
    snapshot_digest,
    source_revision,
    destination_project_id,
    destination_name,
    destination_board_id,
    destination_revision
FROM board_copy_receipts
WHERE source_lineage_id = sqlc.arg(source_lineage_id)
    AND source_board_id = sqlc.arg(source_board_id)
    AND snapshot_version = sqlc.arg(snapshot_version)
ORDER BY id
LIMIT 1;

-- name: BoardListCopyIssueMappings :many
SELECT source_id, destination_id
FROM board_copy_issue_mappings
WHERE receipt_id = sqlc.arg(receipt_id)
ORDER BY source_id;

-- name: BoardListCopyLogMappings :many
SELECT source_id, destination_id
FROM board_copy_log_mappings
WHERE receipt_id = sqlc.arg(receipt_id)
ORDER BY source_id;

-- name: BoardListCopyAttachmentMappings :many
SELECT source_id, destination_id
FROM board_copy_attachment_mappings
WHERE receipt_id = sqlc.arg(receipt_id)
ORDER BY source_id;

-- name: BoardCopyLogIDExists :one
SELECT EXISTS (
    SELECT 1
    FROM issue_log_entries
    WHERE id = sqlc.arg(id)
) AS log_exists;

-- name: BoardInsertCopiedIssue :exec
INSERT INTO issues (
    id,
    board_id,
    title,
    kind,
    lifecycle,
    priority,
    created_at,
    updated_at,
    closed_at,
    waiting_reason,
    waiting_since,
    summary,
    details,
    revision
) VALUES (
    sqlc.arg(id),
    sqlc.arg(board_id),
    sqlc.arg(title),
    sqlc.arg(kind),
    sqlc.arg(lifecycle),
    sqlc.arg(priority),
    sqlc.arg(created_at),
    sqlc.arg(updated_at),
    sqlc.narg(closed_at),
    sqlc.narg(waiting_reason),
    sqlc.narg(waiting_since),
    sqlc.narg(summary),
    sqlc.narg(details),
    sqlc.arg(revision)
);

-- name: BoardInsertCopyReceipt :one
INSERT INTO board_copy_receipts (
    source_lineage_id,
    source_board_id,
    snapshot_version,
    snapshot_digest,
    source_revision,
    destination_project_id,
    destination_name,
    destination_board_id,
    destination_revision,
    created_at
) VALUES (
    sqlc.arg(source_lineage_id),
    sqlc.arg(source_board_id),
    sqlc.arg(snapshot_version),
    sqlc.arg(snapshot_digest),
    sqlc.arg(source_revision),
    sqlc.arg(destination_project_id),
    sqlc.arg(destination_name),
    sqlc.arg(destination_board_id),
    sqlc.arg(destination_revision),
    sqlc.arg(created_at)
)
RETURNING id;

-- name: BoardInsertCopyIssueMapping :exec
INSERT INTO board_copy_issue_mappings (
    receipt_id,
    source_id,
    destination_id
) VALUES (
    sqlc.arg(receipt_id),
    sqlc.arg(source_id),
    sqlc.arg(destination_id)
);

-- name: BoardInsertCopyLogMapping :exec
INSERT INTO board_copy_log_mappings (
    receipt_id,
    source_id,
    destination_id
) VALUES (
    sqlc.arg(receipt_id),
    sqlc.arg(source_id),
    sqlc.arg(destination_id)
);

-- name: BoardInsertCopyAttachmentMapping :exec
INSERT INTO board_copy_attachment_mappings (
    receipt_id,
    source_id,
    destination_id
) VALUES (
    sqlc.arg(receipt_id),
    sqlc.arg(source_id),
    sqlc.arg(destination_id)
);
