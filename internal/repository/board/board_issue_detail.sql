-- name: BoardGetIssueLogSummary :one
SELECT
    COUNT(*) AS log_count,
    CAST(COALESCE((
        SELECT latest.id
        FROM issue_log_entries AS latest
        WHERE latest.board_id = sqlc.arg(scope_board_id)
            AND latest.issue_id = sqlc.arg(selected_issue_id)
        ORDER BY latest.local_sequence DESC
        LIMIT 1
    ), '') AS TEXT) AS latest_log_id
FROM issue_log_entries AS entry
WHERE entry.board_id = sqlc.arg(scope_board_id)
    AND entry.issue_id = sqlc.arg(selected_issue_id);

-- name: BoardGetCheckpointDecision :one
SELECT outcome, reason, decided_at, revision
FROM checkpoint_decisions
WHERE board_id = sqlc.arg(board_id)
    AND issue_id = sqlc.arg(issue_id);

-- name: BoardGetIssueResultBody :one
SELECT body
FROM issue_results
WHERE board_id = sqlc.arg(board_id)
    AND issue_id = sqlc.arg(issue_id);

-- name: BoardGetIssueContextDescription :one
SELECT description
FROM boards
WHERE id = sqlc.arg(board_id);
