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
SELECT issue.id AS issue_id, label.label
FROM issue_labels AS label
JOIN issues AS issue ON issue.uid = label.issue_uid
WHERE label.board_id = sqlc.arg(board_id)
    AND (
        issue.id > sqlc.arg(after_issue_id)
        OR (
            issue.id = sqlc.arg(after_issue_id)
            AND label.label > sqlc.arg(after_label)
        )
    )
ORDER BY issue.id, label.label
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyDependencyPage :many
SELECT issue.id AS issue_id, prerequisite.id AS prerequisite_id
FROM dependencies AS relation
JOIN issues AS issue ON issue.uid = relation.issue_uid
JOIN issues AS prerequisite ON prerequisite.uid = relation.prerequisite_uid
WHERE relation.board_id = sqlc.arg(board_id)
    AND (
        issue.id > sqlc.arg(after_issue_id)
        OR (
            issue.id = sqlc.arg(after_issue_id)
            AND prerequisite.id > sqlc.arg(after_prerequisite_id)
        )
    )
ORDER BY issue.id, prerequisite.id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyContainmentPage :many
SELECT child.id AS child_id, parent.id AS parent_id
FROM containment AS relation
JOIN issues AS child ON child.uid = relation.child_uid
JOIN issues AS parent ON parent.uid = relation.parent_uid
WHERE relation.board_id = sqlc.arg(board_id)
    AND child.id > sqlc.arg(after_child_id)
ORDER BY child.id, parent.id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyExternalKeyPage :many
SELECT key.external_key, issue.id AS issue_id
FROM issue_external_keys AS key
JOIN issues AS issue ON issue.uid = key.issue_uid
WHERE key.board_id = sqlc.arg(board_id)
    AND (
        key.external_key > sqlc.arg(after_external_key)
        OR (
            key.external_key = sqlc.arg(after_external_key)
            AND issue.id > sqlc.arg(after_issue_id)
        )
    )
ORDER BY key.external_key, issue.id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyLogEntryPage :many
SELECT
    log.local_sequence,
    log.id,
    issue.id AS issue_id,
    log.kind,
    log.author,
    log.committer,
    log.body,
    log.created_at,
    log.next_action
FROM issue_log_entries AS log
JOIN issues AS issue ON issue.uid = log.issue_uid
WHERE log.board_id = sqlc.arg(board_id)
    AND log.local_sequence > sqlc.arg(after_local_sequence)
ORDER BY log.local_sequence, log.id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyStatePage :many
SELECT issue.id AS issue_id, state.body, state.author, state.updated_at,
    state.snapshot_log_entry_id, state.next_action
FROM issue_states AS state
JOIN issues AS issue ON issue.uid = state.issue_uid
WHERE state.board_id = sqlc.arg(board_id)
    AND issue.id > sqlc.arg(after_issue_id)
ORDER BY issue.id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyResultPage :many
SELECT issue.id AS issue_id, result.body
FROM issue_results AS result
JOIN issues AS issue ON issue.uid = result.issue_uid
WHERE result.board_id = sqlc.arg(board_id)
    AND issue.id > sqlc.arg(after_issue_id)
ORDER BY issue.id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyCheckpointPage :many
SELECT issue.id AS issue_id, decision.outcome, decision.reason,
    decision.decided_at
FROM checkpoint_decisions AS decision
JOIN issues AS issue ON issue.uid = decision.issue_uid
WHERE decision.board_id = sqlc.arg(board_id)
    AND issue.id > sqlc.arg(after_issue_id)
ORDER BY issue.id
LIMIT sqlc.arg(page_size);

-- name: BoardListCopyPinPage :many
SELECT pin.position, issue.id AS issue_id
FROM board_pins AS pin
JOIN issues AS issue ON issue.uid = pin.issue_uid
WHERE pin.board_id = sqlc.arg(board_id)
    AND pin.position > sqlc.arg(after_position)
ORDER BY pin.position
LIMIT sqlc.arg(page_size);
