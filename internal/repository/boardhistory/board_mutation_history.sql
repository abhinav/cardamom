-- name: BoardHistoryListBoards :many
SELECT board.id
FROM boards AS board
ORDER BY board.id;

-- name: BoardHistoryGetBoardBaseline :one
SELECT
    board.name,
    board.description,
    CAST(board.created_at AS TEXT) AS created_at_text,
    CAST(COALESCE(board.archived_at, '') AS TEXT) AS archived_at_text,
    board.archived_by,
    board.archive_reason,
    board.issue_id_prefix,
    board.issue_id_strategy,
    board.issue_summary_max_bytes,
    board.attachment_max_bytes,
    board.board_pins_max_count,
    replica.writer_uid
FROM boards AS board
JOIN board_replica_identities AS replica ON replica.board_id = board.id
WHERE board.id = sqlc.arg(board_id);

-- name: BoardHistoryNextIssue :one
SELECT
    issue.uid,
    issue.id,
    issue.title,
    issue.kind,
    issue.lifecycle,
    issue.priority,
    CAST(issue.created_at AS TEXT) AS created_at_text,
    CAST(issue.updated_at AS TEXT) AS updated_at_text,
    CAST(COALESCE(issue.closed_at, '') AS TEXT) AS closed_at_text,
    issue.waiting_reason,
    CAST(COALESCE(issue.waiting_since, '') AS TEXT) AS waiting_since_text,
    issue.summary,
    issue.details,
    result.body AS result_body,
    state.body AS state_body,
    state.author AS state_author,
    CAST(COALESCE(state.updated_at, '') AS TEXT) AS state_updated_at_text,
    state.snapshot_log_entry_id,
    state.next_action AS state_next_action,
    checkpoint.outcome AS checkpoint_outcome,
    checkpoint.reason AS checkpoint_reason,
    CAST(COALESCE(checkpoint.decided_at, '') AS TEXT) AS checkpoint_decided_at_text
FROM issues AS issue
LEFT JOIN issue_results AS result ON result.issue_uid = issue.uid
LEFT JOIN issue_states AS state ON state.issue_uid = issue.uid
LEFT JOIN checkpoint_decisions AS checkpoint
    ON checkpoint.issue_uid = issue.uid
WHERE issue.board_id = sqlc.arg(board_id)
    AND issue.uid > sqlc.arg(after_uid)
ORDER BY issue.uid
LIMIT 1;

-- name: BoardHistoryNextLog :one
SELECT
    local_sequence,
    id,
    kind,
    author,
    committer,
    body,
    CAST(COALESCE(created_at, '') AS TEXT) AS created_at_text,
    next_action
FROM issue_log_entries
WHERE board_id = sqlc.arg(board_id)
    AND issue_uid = sqlc.arg(issue_uid)
    AND local_sequence > sqlc.arg(after_sequence)
ORDER BY local_sequence
LIMIT 1;

-- name: BoardHistoryNextLabel :one
SELECT issue_uid, label
FROM issue_labels
WHERE board_id = sqlc.arg(board_id)
    AND (
        issue_uid > sqlc.arg(after_uid)
        OR (
            issue_uid = sqlc.arg(after_uid)
            AND label > sqlc.arg(after_label)
        )
    )
ORDER BY issue_uid, label
LIMIT 1;

-- name: BoardHistoryNextDependency :one
SELECT issue_uid, prerequisite_uid
FROM dependencies
WHERE board_id = sqlc.arg(board_id)
    AND (
        issue_uid > sqlc.arg(after_uid)
        OR (
            issue_uid = sqlc.arg(after_uid)
            AND prerequisite_uid > sqlc.arg(after_prerequisite_uid)
        )
    )
ORDER BY issue_uid, prerequisite_uid
LIMIT 1;

-- name: BoardHistoryNextContainment :one
SELECT child_uid, parent_uid
FROM containment
WHERE board_id = sqlc.arg(board_id)
    AND child_uid > sqlc.arg(after_uid)
ORDER BY child_uid
LIMIT 1;

-- name: BoardHistoryNextExternalKey :one
SELECT external_key, issue_uid
FROM issue_external_keys
WHERE board_id = sqlc.arg(board_id)
    AND external_key > sqlc.arg(after_key)
ORDER BY external_key
LIMIT 1;

-- name: BoardHistoryNextPin :one
SELECT issue_uid, position
FROM board_pins
WHERE board_id = sqlc.arg(board_id)
    AND issue_uid > sqlc.arg(after_uid)
ORDER BY issue_uid
LIMIT 1;

-- name: BoardHistoryNextAttachment :one
SELECT
    id,
    origin_issue_uid,
    blob_digest,
    blob_size_bytes,
    filename,
    media_type,
    lifecycle,
    created_actor,
    CAST(created_at AS TEXT) AS created_at_text,
    removed_actor,
    CAST(COALESCE(removed_at, '') AS TEXT) AS removed_at_text
FROM attachments
WHERE board_id = sqlc.arg(board_id)
    AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT 1;

-- name: BoardHistoryInsertPart :exec
INSERT INTO board_mutation_parts (
    board_id, writer_uid, sequence, part_index, bytes
)
VALUES (
    sqlc.arg(board_id),
    sqlc.arg(writer_uid),
    sqlc.arg(sequence),
    sqlc.arg(part_index),
    sqlc.arg(bytes)
);

-- name: BoardHistoryInsertCommit :exec
INSERT INTO board_mutation_commits (
    board_id,
    writer_uid,
    sequence,
    previous_hash,
    marker_hash,
    marker_bytes,
    part_count,
    event_count,
    payload_hash
)
VALUES (
    sqlc.arg(board_id),
    sqlc.arg(writer_uid),
    sqlc.arg(sequence),
    sqlc.arg(previous_hash),
    sqlc.arg(marker_hash),
    sqlc.arg(marker_bytes),
    sqlc.arg(part_count),
    sqlc.arg(event_count),
    sqlc.arg(payload_hash)
);

-- name: BoardHistoryInsertTip :exec
INSERT INTO board_mutation_tips (
    board_id, writer_uid, sequence, marker_hash
)
VALUES (
    sqlc.arg(board_id),
    sqlc.arg(writer_uid),
    sqlc.arg(sequence),
    sqlc.arg(marker_hash)
);

-- name: BoardHistoryInsertBlobReference :exec
INSERT INTO board_mutation_blob_refs (
    board_id, writer_uid, sequence, digest, size_bytes
)
VALUES (
    sqlc.arg(board_id),
    sqlc.arg(writer_uid),
    sqlc.arg(sequence),
    sqlc.arg(digest),
    sqlc.arg(size_bytes)
)
ON CONFLICT(board_id, writer_uid, sequence, digest) DO NOTHING;

-- name: BoardHistoryGetWriterUID :one
SELECT writer_uid
FROM board_replica_identities
WHERE board_id = sqlc.arg(board_id);

-- name: BoardHistoryListTips :many
SELECT writer_uid, sequence, marker_hash
FROM board_mutation_tips
WHERE board_id = sqlc.arg(board_id)
ORDER BY writer_uid;

-- name: BoardHistoryAdvanceTip :execrows
UPDATE board_mutation_tips
SET sequence = sqlc.arg(sequence), marker_hash = sqlc.arg(marker_hash)
WHERE board_id = sqlc.arg(board_id)
    AND writer_uid = sqlc.arg(writer_uid)
    AND sequence = sqlc.arg(previous_sequence)
    AND marker_hash = sqlc.arg(previous_hash);
