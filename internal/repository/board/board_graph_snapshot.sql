-- name: BoardListSnapshotDependencies :many
SELECT issue_id, prerequisite_id
FROM dependencies
WHERE board_id = sqlc.arg(board_id)
ORDER BY issue_id, prerequisite_id;

-- name: BoardListSnapshotContainment :many
SELECT child_id, parent_id
FROM containment
WHERE board_id = sqlc.arg(board_id)
ORDER BY child_id;
