-- name: BoardGetRevision :one
-- Issue writers read lifecycle and revision together through their immediate
-- transaction, preventing archive and mutation acceptance from interleaving.
SELECT revision, archived_at
FROM boards
WHERE id = sqlc.arg(id);

-- name: BoardPublishIssueRevision :execresult
UPDATE issues
SET revision = sqlc.arg(revision)
WHERE board_id = sqlc.arg(board_id)
    AND id = sqlc.arg(issue_id);

-- name: BoardPublishRevision :execresult
UPDATE boards
SET revision = sqlc.arg(revision)
WHERE id = sqlc.arg(id)
    AND revision = sqlc.arg(previous_revision);
