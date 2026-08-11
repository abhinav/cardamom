-- name: AttachmentBoardArchived :one
-- Attachment writers call this through the same immediate transaction as their
-- blob or metadata mutation.
SELECT archived_at IS NOT NULL
FROM boards
WHERE id = sqlc.arg(id);
