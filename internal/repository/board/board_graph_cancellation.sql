-- name: BoardListCancellationDependencyEdges :many
SELECT issue_id, prerequisite_id
FROM dependencies
WHERE board_id = sqlc.arg(board_id)
ORDER BY issue_id, prerequisite_id;
