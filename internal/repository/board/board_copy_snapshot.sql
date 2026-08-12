-- name: BoardGetCopySource :one
SELECT
    board.id AS board_id,
    board.name AS board_name,
    board.description AS board_description,
    board.created_at AS board_created_at,
    project.issue_id_prefix AS project_issue_id_prefix,
    project.issue_id_strategy AS project_issue_id_strategy,
    project.issue_summary_max_bytes AS project_issue_summary_max_bytes,
    project.attachment_max_bytes AS project_attachment_max_bytes,
    project.board_pins_max_count AS project_board_pins_max_count,
    board.issue_id_prefix AS board_issue_id_prefix,
    board.issue_id_strategy AS board_issue_id_strategy,
    board.issue_summary_max_bytes AS board_issue_summary_max_bytes,
    board.attachment_max_bytes AS board_attachment_max_bytes,
    board.board_pins_max_count AS board_board_pins_max_count
FROM boards AS board
JOIN projects AS project ON project.id = board.project_id
WHERE board.id = sqlc.arg(board_id);

-- name: BoardCountCopyActiveClaims :one
SELECT count(*) AS active_claim_count
FROM active_claims
WHERE board_id = sqlc.arg(board_id);

-- Incremental copy and backup traversal. Each query returns one bounded keyset
-- page from a caller-owned retained view.

-- name: BoardListCopyIssuePage :many
SELECT
    id,
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
    details
FROM issues
WHERE board_id = sqlc.arg(board_id)
    AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyLabelPage :many
SELECT issue_id, label
FROM issue_labels
WHERE board_id = sqlc.arg(board_id)
    AND (
        issue_id > sqlc.arg(after_issue_id)
        OR (
            issue_id = sqlc.arg(after_issue_id)
            AND label > sqlc.arg(after_label)
        )
    )
ORDER BY issue_id, label
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyDependencyPage :many
SELECT issue_id, prerequisite_id
FROM dependencies
WHERE board_id = sqlc.arg(board_id)
    AND (
        issue_id > sqlc.arg(after_issue_id)
        OR (
            issue_id = sqlc.arg(after_issue_id)
            AND prerequisite_id > sqlc.arg(after_prerequisite_id)
        )
    )
ORDER BY issue_id, prerequisite_id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyContainmentPage :many
SELECT child_id, parent_id
FROM containment
WHERE board_id = sqlc.arg(board_id)
    AND child_id > sqlc.arg(after_child_id)
ORDER BY child_id, parent_id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyExternalKeyPage :many
SELECT external_key, issue_id
FROM issue_external_keys
WHERE board_id = sqlc.arg(board_id)
    AND (
        external_key > sqlc.arg(after_external_key)
        OR (
            external_key = sqlc.arg(after_external_key)
            AND issue_id > sqlc.arg(after_issue_id)
        )
    )
ORDER BY external_key, issue_id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyLogEntryPage :many
SELECT
    local_sequence,
    id,
    issue_id,
    kind,
    author,
    committer,
    body,
    created_at,
    next_action
FROM issue_log_entries
WHERE board_id = sqlc.arg(board_id)
    AND local_sequence > sqlc.arg(after_local_sequence)
ORDER BY local_sequence, id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyStatePage :many
SELECT issue_id, body, author, updated_at, snapshot_log_entry_id, next_action
FROM issue_states
WHERE board_id = sqlc.arg(board_id)
    AND issue_id > sqlc.arg(after_issue_id)
ORDER BY issue_id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyResultPage :many
SELECT issue_id, body
FROM issue_results
WHERE board_id = sqlc.arg(board_id)
    AND issue_id > sqlc.arg(after_issue_id)
ORDER BY issue_id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyCheckpointPage :many
SELECT issue_id, outcome, reason, decided_at
FROM checkpoint_decisions
WHERE board_id = sqlc.arg(board_id)
    AND issue_id > sqlc.arg(after_issue_id)
ORDER BY issue_id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyPinPage :many
SELECT position, issue_id
FROM board_pins
WHERE board_id = sqlc.arg(board_id)
    AND position > sqlc.arg(after_position)
ORDER BY position
LIMIT sqlc.arg(page_size);
