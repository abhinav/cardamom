-- name: AttachmentBoardLifecycle :one
-- Attachment writers inspect archive permission in the same immediate
-- transaction as their blob or metadata mutation. NULL archive metadata alone
-- cannot distinguish an active board from a disputed archive assignment.
SELECT boards.archived_at IS NOT NULL AS archived,
       conflict.id AS archive_conflict_id
FROM boards
LEFT JOIN board_conflicts AS conflict
    ON conflict.board_id = boards.id
    AND conflict.issue_uid IS NULL
    AND conflict.kind = 'board.archive.set'
WHERE boards.id = sqlc.arg(id)
ORDER BY conflict.id
LIMIT 1;
