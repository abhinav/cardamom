-- name: ProjectReadBoardConflicts :many
-- Catalog reads batch compact references; a board identity narrows mutations
-- and configuration reads to the same board as their projection query.
SELECT id, board_id, kind
FROM board_conflicts
WHERE issue_uid IS NULL
    AND (board_id = sqlc.narg(board_id) OR sqlc.narg(board_id) IS NULL)
    AND kind IN (
        'board.name.set', 'board.description.set', 'board.archive.set',
        'board.issue_id_prefix.set', 'board.issue_id_strategy.set',
        'board.summary_limit.set', 'board.attachment_limit.set',
        'board.pin_limit.set'
    )
ORDER BY board_id, kind, id;
