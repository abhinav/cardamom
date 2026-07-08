-- name: BoardListDirectChildIDs :many
SELECT child_id
FROM containment
WHERE board_id = sqlc.arg(board_id)
    AND parent_id = sqlc.arg(parent_id)
ORDER BY child_id;

-- name: BoardGetParentID :one
SELECT parent_id
FROM containment
WHERE board_id = sqlc.arg(board_id)
    AND child_id = sqlc.arg(child_id);

-- name: BoardListPrerequisiteIDs :many
SELECT prerequisite_id
FROM dependencies
WHERE board_id = sqlc.arg(board_id)
    AND issue_id = sqlc.arg(issue_id)
ORDER BY prerequisite_id;

-- name: BoardListBlockIDs :many
SELECT issue_id
FROM dependencies
WHERE board_id = sqlc.arg(board_id)
    AND prerequisite_id = sqlc.arg(prerequisite_id)
ORDER BY issue_id;

-- name: BoardIssueExists :one
SELECT EXISTS(
    SELECT 1
    FROM issues
    WHERE board_id = sqlc.arg(board_id)
        AND id = sqlc.arg(issue_id)
);

-- name: BoardListContainmentParents :many
SELECT child_id, parent_id
FROM containment
WHERE board_id = sqlc.arg(board_id);

-- name: BoardListDescendantIDs :many
WITH RECURSIVE descendants AS (
    SELECT containment.child_id AS id
    FROM containment
    WHERE containment.board_id = sqlc.arg(scope_board_id)
        AND containment.parent_id = sqlc.arg(root_id)
    UNION ALL
    SELECT containment.child_id
    FROM containment
    JOIN descendants ON containment.parent_id = descendants.id
    WHERE containment.board_id = sqlc.arg(scope_board_id)
)
SELECT descendants.id
FROM descendants;

-- name: BoardListBlockedIssueIDs :many
SELECT DISTINCT dependency.issue_id
FROM dependencies AS dependency
JOIN issues AS prerequisite ON prerequisite.id = dependency.prerequisite_id
WHERE dependency.board_id = sqlc.arg(board_id)
    AND prerequisite.lifecycle <> 'closed';
