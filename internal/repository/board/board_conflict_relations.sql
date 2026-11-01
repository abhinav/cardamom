-- name: BoardReadRelationIssues :many
SELECT uid, kind, lifecycle FROM issues WHERE board_id = sqlc.arg(board_id);

-- name: BoardReadRelationDependencies :many
SELECT issue_uid, prerequisite_uid FROM dependencies WHERE board_id = sqlc.arg(board_id);

-- name: BoardReadRelationParents :many
SELECT child_uid, parent_uid FROM containment WHERE board_id = sqlc.arg(board_id);
