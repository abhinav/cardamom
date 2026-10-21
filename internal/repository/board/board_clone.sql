-- Clone keeps only compact identity indexes in memory. Record bodies are
-- traversed separately through the bounded projection pager.

-- name: BoardCloneIssueIdentities :many
SELECT id, uid FROM issues WHERE board_id = sqlc.arg(board_id) ORDER BY id;

-- name: BoardCloneLogIdentities :many
SELECT id FROM issue_log_entries WHERE board_id = sqlc.arg(board_id) ORDER BY id;
