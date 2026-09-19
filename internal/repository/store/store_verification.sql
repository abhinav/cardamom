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

-- name: StoreCountBoardsWithoutReplicaIdentity :one
SELECT count(*)
FROM boards AS board
LEFT JOIN board_replica_identities AS replica
    ON replica.board_id = board.id
WHERE replica.board_id IS NULL;

-- name: StoreCountBoardsWithoutLocalMutationHistory :one
SELECT count(*)
FROM boards AS board
JOIN board_replica_identities AS replica ON replica.board_id = board.id
LEFT JOIN board_mutation_tips AS tip
    ON tip.board_id = board.id
    AND tip.writer_uid = replica.writer_uid
WHERE tip.board_id IS NULL;

-- name: StoreCountInvalidMutationTopology :one
SELECT
    (
        SELECT count(*)
        FROM board_mutation_commits AS commit_row
        WHERE (
            commit_row.sequence = 1
            AND commit_row.previous_hash <> zeroblob(32)
        ) OR (
            commit_row.sequence > 1
            AND NOT EXISTS (
                SELECT 1
                FROM board_mutation_commits AS predecessor
                WHERE predecessor.board_id = commit_row.board_id
                    AND predecessor.writer_uid = commit_row.writer_uid
                    AND predecessor.sequence = commit_row.sequence - 1
                    AND predecessor.marker_hash = commit_row.previous_hash
            )
        )
    ) + (
        SELECT count(*)
        FROM board_mutation_commits AS commit_row
        WHERE commit_row.part_count <> (
            SELECT count(*)
            FROM board_mutation_parts AS part
            WHERE part.board_id = commit_row.board_id
                AND part.writer_uid = commit_row.writer_uid
                AND part.sequence = commit_row.sequence
        ) OR commit_row.part_count - 1 <> (
            SELECT max(part.part_index)
            FROM board_mutation_parts AS part
            WHERE part.board_id = commit_row.board_id
                AND part.writer_uid = commit_row.writer_uid
                AND part.sequence = commit_row.sequence
        )
    ) + (
        SELECT count(*)
        FROM board_mutation_tips AS tip
        WHERE tip.sequence <> (
            SELECT max(commit_row.sequence)
            FROM board_mutation_commits AS commit_row
            WHERE commit_row.board_id = tip.board_id
                AND commit_row.writer_uid = tip.writer_uid
        )
    ) + (
        SELECT count(*)
        FROM board_mutation_commits AS commit_row
        WHERE NOT EXISTS (
            SELECT 1
            FROM board_mutation_tips AS tip
            WHERE tip.board_id = commit_row.board_id
                AND tip.writer_uid = commit_row.writer_uid
        )
    );

-- name: StoreNextBoardMutationCommit :one
SELECT
    board_id,
    writer_uid,
    sequence,
    previous_hash,
    marker_hash,
    marker_bytes,
    part_count,
    event_count,
    payload_hash
FROM board_mutation_commits
WHERE board_id > sqlc.arg(after_board_id)
    OR (
        board_id = sqlc.arg(after_board_id)
        AND writer_uid > sqlc.arg(after_writer_uid)
    )
    OR (
        board_id = sqlc.arg(after_board_id)
        AND writer_uid = sqlc.arg(after_writer_uid)
        AND sequence > sqlc.arg(after_sequence)
    )
ORDER BY board_id, writer_uid, sequence
LIMIT 1;

-- name: StoreGetBoardMutationPart :one
SELECT bytes
FROM board_mutation_parts
WHERE board_id = sqlc.arg(board_id)
    AND writer_uid = sqlc.arg(writer_uid)
    AND sequence = sqlc.arg(sequence)
    AND part_index = sqlc.arg(part_index);

-- name: StoreListBoardMutationBlobReferences :many
SELECT digest, size_bytes
FROM board_mutation_blob_refs
WHERE board_id = sqlc.arg(board_id)
    AND writer_uid = sqlc.arg(writer_uid)
    AND sequence = sqlc.arg(sequence)
ORDER BY digest;

-- name: StoreCountBoardMutationFrontierCommit :one
SELECT count(*)
FROM board_mutation_commits
WHERE board_id = sqlc.arg(board_id)
    AND writer_uid = sqlc.arg(writer_uid)
    AND sequence = sqlc.arg(sequence)
    AND marker_hash = sqlc.arg(marker_hash);

-- name: StoreCountInvalidIssueUIDs :one
SELECT count(*)
FROM issues
WHERE typeof(uid) <> 'blob'
    OR length(uid) <> 16
    OR uid = zeroblob(16);

-- name: StoreCountInvalidClaims :one
SELECT count(*)
FROM active_claims AS claim
JOIN issues AS issue ON issue.uid = claim.issue_uid
WHERE issue.lifecycle <> 'open'
    OR issue.kind NOT IN ('workstream', 'task', 'routine');

-- name: StoreCountInvalidIssueSearchDocuments :one
WITH expected AS (
    SELECT
        board_id AS board_id,
        uid AS issue_uid,
        CAST('title' AS TEXT) AS field,
        CAST('' AS TEXT) AS record_id,
        title AS body
    FROM issues
    UNION ALL
    SELECT board_id, uid, 'summary', '', summary
    FROM issues
    WHERE summary IS NOT NULL
    UNION ALL
    SELECT board_id, uid, 'details', '', details
    FROM issues
    WHERE details IS NOT NULL
    UNION ALL
    SELECT
        board_id,
        issue_uid,
        'state',
        '',
        body || CASE
            WHEN next_action IS NULL THEN ''
            ELSE char(10) || char(10) || next_action
        END
    FROM issue_states
    UNION ALL
    SELECT board_id, issue_uid, 'result', '', body
    FROM issue_results
    UNION ALL
    SELECT
        board_id,
        issue_uid,
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
                AND document.issue_uid = expected.issue_uid
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
                AND expected.issue_uid = document.issue_uid
                AND expected.field = document.field
                AND expected.record_id = document.record_id
                AND expected.body = document.body
        )
    );

-- name: StoreCheckIssueSearchIndex :exec
INSERT INTO issue_search_fts(issue_search_fts, rank)
VALUES ('integrity-check', 1);
