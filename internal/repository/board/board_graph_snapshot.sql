-- name: BoardListSnapshotDependencies :many
SELECT issue.id AS issue_id, prerequisite.id AS prerequisite_id
FROM dependencies AS relation
JOIN issues AS issue ON issue.uid = relation.issue_uid
JOIN issues AS prerequisite ON prerequisite.uid = relation.prerequisite_uid
WHERE relation.board_id = sqlc.arg(board_id)
ORDER BY issue.id, prerequisite.id;

-- name: BoardListSnapshotContainment :many
SELECT child.id AS child_id, parent.id AS parent_id
FROM containment AS relation
JOIN issues AS child ON child.uid = relation.child_uid
JOIN issues AS parent ON parent.uid = relation.parent_uid
WHERE relation.board_id = sqlc.arg(board_id)
ORDER BY child.id;
