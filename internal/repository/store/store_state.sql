-- name: StoreGetCanonicalRevision :one
SELECT current_revision
FROM store_state
WHERE singleton = 1;

-- name: StorePublishCanonicalRevision :execresult
UPDATE store_state
SET current_revision = sqlc.arg(revision)
WHERE singleton = 1
    AND current_revision = sqlc.arg(current_revision);

-- name: StoreGetNextIssueNumber :one
SELECT next_issue_number
FROM store_state
WHERE singleton = 1;

-- name: StoreSetNextIssueNumber :exec
UPDATE store_state
SET next_issue_number = sqlc.arg(next_issue_number)
WHERE singleton = 1;

-- name: StoreAdvanceNextIssueNumber :exec
UPDATE store_state
SET next_issue_number = sqlc.arg(next_issue_number)
WHERE singleton = 1
    AND next_issue_number < sqlc.arg(next_issue_number);
