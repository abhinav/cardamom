-- name: BoardListCancellationDependencyEdges :many
SELECT issue.id AS issue_id, prerequisite.id AS prerequisite_id
FROM dependencies AS relation
JOIN issues AS issue ON issue.uid = relation.issue_uid
JOIN issues AS prerequisite ON prerequisite.uid = relation.prerequisite_uid
WHERE relation.board_id = sqlc.arg(board_id)
ORDER BY issue.id, prerequisite.id;
