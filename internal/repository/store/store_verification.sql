-- name: StoreCountStateRows :one
SELECT count(*)
FROM store_state;

-- name: StoreGetVerificationState :one
SELECT current_revision, next_issue_number
FROM store_state
WHERE singleton = 1;

-- name: StoreCountInvalidProjectionRevisions :one
SELECT count(*)
FROM boards AS board
LEFT JOIN issues AS issue ON issue.board_id = board.id
WHERE board.revision > sqlc.arg(current_revision)
    OR issue.revision > board.revision;

-- name: StoreCountInvalidClaims :one
SELECT count(*)
FROM active_claims AS claim
JOIN issues AS issue ON issue.id = claim.issue_id
WHERE issue.lifecycle <> 'open'
    OR issue.kind NOT IN ('workstream', 'task', 'routine');
