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

-- name: StoreCountInvalidIssueSearchDocuments :one
WITH expected AS (
    SELECT
        board_id AS board_id,
        id AS issue_id,
        CAST('title' AS TEXT) AS field,
        CAST('' AS TEXT) AS record_id,
        title AS body
    FROM issues
    UNION ALL
    SELECT board_id, id, 'summary', '', summary
    FROM issues
    WHERE summary IS NOT NULL
    UNION ALL
    SELECT board_id, id, 'details', '', details
    FROM issues
    WHERE details IS NOT NULL
    UNION ALL
    SELECT
        board_id,
        issue_id,
        'state',
        '',
        body || CASE
            WHEN next_action IS NULL THEN ''
            ELSE char(10) || char(10) || next_action
        END
    FROM issue_states
    UNION ALL
    SELECT board_id, issue_id, 'result', '', body
    FROM issue_results
    UNION ALL
    SELECT
        board_id,
        issue_id,
        'log',
        id,
        body || CASE
            WHEN next_action IS NULL THEN ''
            ELSE char(10) || char(10) || next_action
        END
    FROM issue_log_entries
)
SELECT
    (
        SELECT count(*)
        FROM expected
        WHERE NOT EXISTS (
            SELECT 1
            FROM issue_search_documents AS document
            WHERE document.board_id = expected.board_id
                AND document.issue_id = expected.issue_id
                AND document.field = expected.field
                AND document.record_id = expected.record_id
                AND document.body = expected.body
        )
    ) + (
        SELECT count(*)
        FROM issue_search_documents AS document
        WHERE NOT EXISTS (
            SELECT 1
            FROM expected
            WHERE expected.board_id = document.board_id
                AND expected.issue_id = document.issue_id
                AND expected.field = document.field
                AND expected.record_id = document.record_id
                AND expected.body = document.body
        )
    );

-- name: StoreCheckIssueSearchIndex :exec
INSERT INTO issue_search_fts(issue_search_fts, rank)
VALUES ('integrity-check', 1);
