-- name: BoardGetIssueIDByExternalKey :one
SELECT issue.id, conflict.id AS conflict_id
FROM boards AS board
LEFT JOIN issue_external_keys AS key ON key.board_id = board.id AND key.external_key = sqlc.arg(external_key)
LEFT JOIN issues AS issue ON issue.uid = key.issue_uid
LEFT JOIN board_conflicts AS conflict ON conflict.board_id = board.id
    AND conflict.kind = 'board.external_key' AND conflict.member_key = sqlc.arg(external_key)
WHERE board.id = sqlc.arg(board_id);

-- name: BoardListIssueExternalKeys :many
SELECT external_key
FROM issue_external_keys
WHERE board_id = sqlc.arg(board_id)
    AND issue_uid = sqlc.arg(issue_uid)
ORDER BY external_key;

-- name: BoardGetIssueLogSummary :one
SELECT
    COUNT(*) AS log_count,
    CAST(COALESCE((
        SELECT latest.id
        FROM issue_log_entries AS latest
        WHERE latest.board_id = sqlc.arg(scope_board_id)
            AND latest.issue_uid = sqlc.arg(selected_issue_uid)
        ORDER BY latest.local_sequence DESC
        LIMIT 1
    ), '') AS TEXT) AS latest_log_id
FROM issue_log_entries AS entry
WHERE entry.board_id = sqlc.arg(scope_board_id)
    AND entry.issue_uid = sqlc.arg(selected_issue_uid);

-- name: BoardGetCheckpointDecision :one
SELECT outcome, reason, decided_at, revision
FROM checkpoint_decisions
WHERE board_id = sqlc.arg(board_id)
    AND issue_uid = sqlc.arg(issue_uid);

-- name: BoardGetIssueResultBody :one
SELECT body
FROM issue_results
WHERE board_id = sqlc.arg(board_id)
    AND issue_uid = sqlc.arg(issue_uid);

-- name: BoardGetIssueContextDescription :one
SELECT description
FROM boards
WHERE id = sqlc.arg(board_id);
