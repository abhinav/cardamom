-- name: BoardListDirectChildIDs :many
SELECT child.id
FROM containment AS relation
JOIN issues AS child ON child.uid = relation.child_uid
WHERE relation.board_id = sqlc.arg(board_id)
    AND relation.parent_uid = sqlc.arg(parent_uid)
ORDER BY child.id;

-- name: BoardGetParentID :one
SELECT parent.id
FROM containment AS relation
JOIN issues AS parent ON parent.uid = relation.parent_uid
WHERE relation.board_id = sqlc.arg(board_id)
    AND relation.child_uid = sqlc.arg(child_uid);

-- name: BoardListPrerequisiteIDs :many
SELECT prerequisite.id
FROM dependencies AS relation
JOIN issues AS prerequisite ON prerequisite.uid = relation.prerequisite_uid
WHERE relation.board_id = sqlc.arg(board_id)
    AND relation.issue_uid = sqlc.arg(issue_uid)
ORDER BY prerequisite.id;

-- name: BoardListBlockIDs :many
SELECT issue.id
FROM dependencies AS relation
JOIN issues AS issue ON issue.uid = relation.issue_uid
WHERE relation.board_id = sqlc.arg(board_id)
    AND relation.prerequisite_uid = sqlc.arg(prerequisite_uid)
ORDER BY issue.id;

-- name: BoardIssueExists :one
SELECT EXISTS(
    SELECT 1
    FROM issues
    WHERE board_id = sqlc.arg(board_id)
        AND id = sqlc.arg(issue_id)
);

-- name: BoardListContainmentParents :many
SELECT child.id AS child_id, parent.id AS parent_id
FROM containment AS relation
JOIN issues AS child ON child.uid = relation.child_uid
JOIN issues AS parent ON parent.uid = relation.parent_uid
WHERE relation.board_id = sqlc.arg(board_id);

-- name: BoardListDescendantIDs :many
WITH RECURSIVE descendants AS (
    SELECT containment.child_uid AS uid
    FROM containment
    WHERE containment.board_id = sqlc.arg(scope_board_id)
        AND containment.parent_uid = sqlc.arg(root_uid)
    UNION ALL
    SELECT containment.child_uid
    FROM containment
    JOIN descendants ON containment.parent_uid = descendants.uid
    WHERE containment.board_id = sqlc.arg(scope_board_id)
)
SELECT issue.id
FROM descendants
JOIN issues AS issue ON issue.uid = descendants.uid;

-- name: BoardListBlockedIssueIDs :many
SELECT DISTINCT issue.id
FROM dependencies AS dependency
JOIN issues AS issue ON issue.uid = dependency.issue_uid
JOIN issues AS prerequisite ON prerequisite.uid = dependency.prerequisite_uid
WHERE dependency.board_id = sqlc.arg(board_id)
    AND prerequisite.lifecycle <> 'closed';
