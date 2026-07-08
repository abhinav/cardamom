-- name: BoardListApplyExternalKeys :many
SELECT external_key, issue_id
FROM issue_external_keys
WHERE board_id = sqlc.arg(board_id)
ORDER BY external_key;

-- name: BoardListApplyForeignIssueBoards :many
SELECT id, board_id
FROM issues
WHERE board_id <> sqlc.arg(board_id)
    AND id IN (sqlc.slice('issue_ids'))
ORDER BY id;
