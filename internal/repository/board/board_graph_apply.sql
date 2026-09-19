-- name: BoardListApplyExternalKeys :many
SELECT key.external_key, issue.id AS issue_id
FROM issue_external_keys AS key
JOIN issues AS issue ON issue.uid = key.issue_uid
WHERE key.board_id = sqlc.arg(board_id)
ORDER BY key.external_key;

-- name: BoardListApplyForeignIssueBoards :many
SELECT id, board_id
FROM issues
WHERE board_id <> sqlc.arg(board_id)
    AND id IN (sqlc.slice('issue_ids'))
ORDER BY id;
