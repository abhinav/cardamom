-- name: BoardListCompletionLabels :many
SELECT DISTINCT label
FROM issue_labels
WHERE board_id = sqlc.arg(board_id)
ORDER BY label;

-- name: BoardListCompletionIssueIDs :many
SELECT id
FROM issues
WHERE board_id = sqlc.arg(board_id)
ORDER BY priority, created_at, id;

-- name: BoardListCompletionActors :many
SELECT attributed.actor
FROM (
    SELECT log_entry.author AS actor
    FROM issue_log_entries AS log_entry
    WHERE log_entry.board_id = sqlc.arg(scope_board_id)
        AND log_entry.author IS NOT NULL
    UNION
    SELECT committed_log_entry.committer AS actor
    FROM issue_log_entries AS committed_log_entry
    WHERE committed_log_entry.board_id = sqlc.arg(scope_board_id)
        AND committed_log_entry.committer IS NOT NULL
    UNION
    SELECT state.author AS actor
    FROM issue_states AS state
    WHERE state.board_id = sqlc.arg(scope_board_id)
        AND state.author IS NOT NULL
    UNION
    SELECT claim.actor
    FROM active_claims AS claim
    WHERE claim.board_id = sqlc.arg(scope_board_id)
    UNION
    SELECT attachment.created_actor
    FROM attachments AS attachment
    WHERE attachment.board_id = sqlc.arg(scope_board_id)
    UNION
    SELECT removed_attachment.removed_actor
    FROM attachments AS removed_attachment
    WHERE removed_attachment.board_id = sqlc.arg(scope_board_id)
        AND removed_attachment.removed_actor IS NOT NULL
    UNION
    SELECT upload.actor
    FROM attachment_uploads AS upload
    WHERE upload.board_id = sqlc.arg(scope_board_id)
) AS attributed
ORDER BY attributed.actor;
