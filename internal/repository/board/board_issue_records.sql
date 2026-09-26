-- name: BoardInsertIssueLogEntry :exec
INSERT INTO issue_log_entries (
    id,
    board_id,
    issue_uid,
    kind,
    author,
    committer,
    body,
    next_action,
    created_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(board_id),
    sqlc.arg(issue_uid),
    sqlc.arg(kind),
    sqlc.narg(author),
    sqlc.narg(committer),
    sqlc.arg(body),
    sqlc.narg(next_action),
    sqlc.narg(created_at)
);

-- name: BoardUpsertIssueState :exec
INSERT INTO issue_states (
    issue_uid,
    board_id,
    body,
    next_action,
    author,
    updated_at,
    snapshot_log_entry_id
) VALUES (
    sqlc.arg(issue_uid),
    sqlc.arg(board_id),
    sqlc.arg(body),
    sqlc.narg(next_action),
    sqlc.narg(author),
    sqlc.narg(updated_at),
    sqlc.narg(snapshot_log_entry_id)
)
ON CONFLICT(issue_uid) DO UPDATE SET
    body = excluded.body,
    next_action = excluded.next_action,
    author = excluded.author,
    updated_at = excluded.updated_at,
    snapshot_log_entry_id = excluded.snapshot_log_entry_id;

-- name: BoardDeleteIssueState :exec
DELETE FROM issue_states
WHERE board_id = sqlc.arg(board_id)
    AND issue_uid = sqlc.arg(issue_uid);

-- name: BoardUpsertIssueResult :exec
INSERT INTO issue_results (issue_uid, board_id, body)
VALUES (
    sqlc.arg(issue_uid),
    sqlc.arg(board_id),
    sqlc.arg(body)
)
ON CONFLICT(issue_uid) DO UPDATE SET body = excluded.body;

-- name: BoardListIssueLogEntriesAscending :many
SELECT log.id, issue.id AS issue_id, log.kind, log.author, log.committer,
    log.body, log.next_action, log.created_at
FROM issue_log_entries AS log
JOIN issues AS issue ON issue.uid = log.issue_uid
WHERE log.board_id = sqlc.arg(board_id)
    AND log.issue_uid = sqlc.arg(issue_uid)
ORDER BY log.local_sequence
LIMIT sqlc.arg(limit_count);

-- name: BoardListIssueLogEntriesDescending :many
SELECT log.id, issue.id AS issue_id, log.kind, log.author, log.committer,
    log.body, log.next_action, log.created_at
FROM issue_log_entries AS log
JOIN issues AS issue ON issue.uid = log.issue_uid
WHERE log.board_id = sqlc.arg(board_id)
    AND log.issue_uid = sqlc.arg(issue_uid)
ORDER BY log.local_sequence DESC
LIMIT sqlc.arg(limit_count);

-- name: BoardReadIssueLogEntry :one
SELECT log.id, issue.id AS issue_id, log.kind, log.author, log.committer,
    log.body, log.next_action, log.created_at
FROM issue_log_entries AS log
JOIN issues AS issue ON issue.uid = log.issue_uid
WHERE log.board_id = sqlc.arg(board_id)
    AND log.id = sqlc.arg(log_id);

-- name: BoardReadIssueResult :one
SELECT issue.id, issue.title, result.body
FROM issues AS issue
JOIN issue_results AS result ON result.issue_uid = issue.uid
WHERE issue.board_id = sqlc.arg(board_id)
    AND issue.uid = sqlc.arg(issue_uid);
