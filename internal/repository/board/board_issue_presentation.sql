-- Claim selection and reference rendering need execution metadata, not the
-- Summary, Details, State, Result, or conflict alternative bodies.
-- name: BoardListIssuePresentation :many
SELECT issue.uid, issue.id, issue.title, issue.kind, issue.lifecycle,
       issue.priority, issue.created_at, issue.updated_at, issue.closed_at,
       issue.waiting_since, issue.revision,
       claim.actor AS claim_actor, claim.started_at AS claim_started_at
FROM issues AS issue
LEFT JOIN active_claims AS claim ON claim.issue_uid = issue.uid
WHERE issue.board_id = sqlc.arg(board_id);

-- These board-level disputes affect inherited context for every claim candidate.
-- name: BoardListContextConflicts :many
SELECT id, kind FROM board_conflicts
WHERE board_id = sqlc.arg(board_id)
    AND kind IN ('board.description.set', 'board.pin')
ORDER BY id;
