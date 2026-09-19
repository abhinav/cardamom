-- name: BoardListEditDependencyAncestorIDs :many
WITH RECURSIVE ancestors AS (
    SELECT dependencies.prerequisite_uid AS uid
    FROM dependencies
    WHERE dependencies.board_id = sqlc.arg(scope_board_id)
        AND dependencies.issue_uid = sqlc.arg(start_uid)
    UNION
    SELECT dependency.prerequisite_uid
    FROM dependencies AS dependency
    JOIN ancestors ON dependency.issue_uid = ancestors.uid
    WHERE dependency.board_id = sqlc.arg(scope_board_id)
)
SELECT issue.id
FROM ancestors
JOIN issues AS issue ON issue.uid = ancestors.uid
ORDER BY issue.id;

-- name: BoardListEditContainmentAncestorIDs :many
WITH RECURSIVE ancestors AS (
    SELECT containment.parent_uid AS uid
    FROM containment
    WHERE containment.board_id = sqlc.arg(scope_board_id)
        AND containment.child_uid = sqlc.arg(start_uid)
    UNION ALL
    SELECT containment.parent_uid
    FROM containment
    JOIN ancestors ON containment.child_uid = ancestors.uid
    WHERE containment.board_id = sqlc.arg(scope_board_id)
)
SELECT issue.id
FROM ancestors
JOIN issues AS issue ON issue.uid = ancestors.uid;
