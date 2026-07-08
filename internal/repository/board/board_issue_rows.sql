-- name: BoardInsertIssue :exec
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
    details
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
    sqlc.narg(details)
);

-- name: BoardUpdateIssue :exec
UPDATE issues
SET title = sqlc.arg(title),
    kind = sqlc.arg(kind),
    lifecycle = sqlc.arg(lifecycle),
    priority = sqlc.arg(priority),
    updated_at = sqlc.arg(updated_at),
    closed_at = sqlc.narg(closed_at),
    waiting_reason = sqlc.narg(waiting_reason),
    waiting_since = sqlc.narg(waiting_since),
    summary = sqlc.narg(summary),
    details = sqlc.narg(details)
WHERE board_id = sqlc.arg(board_id)
    AND id = sqlc.arg(id);

-- name: BoardGetIssueState :one
SELECT
    issue.id,
    issue.title,
    issue.kind,
    issue.lifecycle,
    issue.priority,
    issue.created_at,
    issue.updated_at,
    issue.closed_at,
    issue.waiting_reason,
    issue.waiting_since,
    issue.summary,
    issue.details,
    state.body AS state_body,
    state.next_action AS state_next_action,
    state.author AS state_author,
    state.updated_at AS state_updated_at,
    state.snapshot_log_entry_id AS state_snapshot_log_entry_id,
    issue.revision,
    claim.actor AS claim_actor,
    claim.started_at AS claim_started_at,
    result.body AS result_body
FROM issues AS issue
LEFT JOIN active_claims AS claim ON claim.issue_id = issue.id
LEFT JOIN issue_results AS result ON result.issue_id = issue.id
LEFT JOIN issue_states AS state ON state.issue_id = issue.id
WHERE issue.board_id = sqlc.arg(board_id)
    AND issue.id = sqlc.arg(id);

-- name: BoardListIssueStates :many
SELECT
    issue.id,
    issue.title,
    issue.kind,
    issue.lifecycle,
    issue.priority,
    issue.created_at,
    issue.updated_at,
    issue.closed_at,
    issue.waiting_reason,
    issue.waiting_since,
    issue.summary,
    issue.details,
    state.body AS state_body,
    state.next_action AS state_next_action,
    state.author AS state_author,
    state.updated_at AS state_updated_at,
    state.snapshot_log_entry_id AS state_snapshot_log_entry_id,
    issue.revision,
    claim.actor AS claim_actor,
    claim.started_at AS claim_started_at,
    result.body AS result_body
FROM issues AS issue
LEFT JOIN active_claims AS claim ON claim.issue_id = issue.id
LEFT JOIN issue_results AS result ON result.issue_id = issue.id
LEFT JOIN issue_states AS state ON state.issue_id = issue.id
WHERE issue.board_id = sqlc.arg(board_id);
