-- name: BoardReferenceMarkerExists :one
SELECT EXISTS (
    SELECT 1 FROM issue_reference_markers WHERE id = sqlc.arg(id)
) AS marker_exists;

-- name: BoardReadReferenceCandidates :many
SELECT candidate.issue_uid, candidate.board_id, candidate.replacement_id,
    issue.id AS current_id, issue.title
FROM issue_reference_candidates AS candidate
LEFT JOIN issues AS issue ON issue.uid = candidate.issue_uid
WHERE candidate.marker_id = sqlc.arg(id)
ORDER BY candidate.issue_uid;

-- name: BoardInsertReferenceMarker :exec
INSERT INTO issue_reference_markers (id) VALUES (sqlc.arg(id))
ON CONFLICT (id) DO NOTHING;

-- name: BoardInsertReferenceCandidate :exec
INSERT INTO issue_reference_candidates (
    marker_id, issue_uid, board_id, replacement_id
) VALUES (
    sqlc.arg(marker_id), sqlc.arg(issue_uid), sqlc.arg(board_id),
    sqlc.arg(replacement_id)
)
ON CONFLICT (marker_id, issue_uid) DO NOTHING;

-- name: BoardCloneReferenceCandidates :many
SELECT candidate.marker_id, candidate.issue_uid, candidate.board_id,
    candidate.replacement_id
FROM issue_reference_candidates AS candidate
WHERE candidate.marker_id IN (
    SELECT owned.marker_id FROM issue_reference_candidates AS owned
    WHERE owned.board_id = sqlc.arg(board_id)
)
ORDER BY candidate.marker_id, candidate.issue_uid
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);
