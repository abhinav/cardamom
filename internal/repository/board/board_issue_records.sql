-- name: BoardInsertIssueLogEntry :exec
INSERT INTO issue_log_entries (
    id,
    board_id,
    issue_id,
    kind,
    author,
    committer,
    body,
    next_action,
    created_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(board_id),
    sqlc.arg(issue_id),
    sqlc.arg(kind),
    sqlc.narg(author),
    sqlc.narg(committer),
    sqlc.arg(body),
    sqlc.narg(next_action),
    sqlc.narg(created_at)
);

-- name: BoardUpsertIssueState :exec
INSERT INTO issue_states (
    issue_id,
    board_id,
    body,
    next_action,
    author,
    updated_at,
    snapshot_log_entry_id
) VALUES (
    sqlc.arg(issue_id),
    sqlc.arg(board_id),
    sqlc.arg(body),
    sqlc.narg(next_action),
    sqlc.narg(author),
    sqlc.narg(updated_at),
    sqlc.narg(snapshot_log_entry_id)
)
ON CONFLICT(issue_id) DO UPDATE SET
    body = excluded.body,
    next_action = excluded.next_action,
    author = excluded.author,
    updated_at = excluded.updated_at,
    snapshot_log_entry_id = excluded.snapshot_log_entry_id;

-- name: BoardDeleteIssueState :exec
DELETE FROM issue_states
WHERE board_id = sqlc.arg(board_id)
    AND issue_id = sqlc.arg(issue_id);

-- name: BoardUpsertIssueResult :exec
INSERT INTO issue_results (issue_id, board_id, body)
VALUES (
    sqlc.arg(issue_id),
    sqlc.arg(board_id),
    sqlc.arg(body)
)
ON CONFLICT(issue_id) DO UPDATE SET body = excluded.body;

-- name: BoardListIssueLogEntriesAscending :many
SELECT id, issue_id, kind, author, committer, body, next_action, created_at
FROM issue_log_entries
WHERE board_id = sqlc.arg(board_id)
    AND issue_id = sqlc.arg(issue_id)
ORDER BY local_sequence
LIMIT sqlc.arg(limit_count);

-- name: BoardListIssueLogEntriesDescending :many
SELECT id, issue_id, kind, author, committer, body, next_action, created_at
FROM issue_log_entries
WHERE board_id = sqlc.arg(board_id)
    AND issue_id = sqlc.arg(issue_id)
ORDER BY local_sequence DESC
LIMIT sqlc.arg(limit_count);

-- name: BoardReadIssueResult :one
SELECT issue.id, issue.title, result.body
FROM issues AS issue
JOIN issue_results AS result ON result.issue_id = issue.id
WHERE issue.board_id = sqlc.arg(board_id)
    AND issue.id = sqlc.arg(issue_id);
