-- name: BoardListEditDependencyAncestorIDs :many
WITH RECURSIVE ancestors AS (
    SELECT dependencies.prerequisite_id AS id
    FROM dependencies
    WHERE dependencies.board_id = sqlc.arg(scope_board_id)
        AND dependencies.issue_id = sqlc.arg(start_id)
    UNION
    SELECT dependency.prerequisite_id
    FROM dependencies AS dependency
    JOIN ancestors ON dependency.issue_id = ancestors.id
    WHERE dependency.board_id = sqlc.arg(scope_board_id)
)
SELECT ancestors.id
FROM ancestors
ORDER BY id;

-- name: BoardListEditContainmentAncestorIDs :many
WITH RECURSIVE ancestors AS (
    SELECT containment.parent_id AS id
    FROM containment
    WHERE containment.board_id = sqlc.arg(scope_board_id)
        AND containment.child_id = sqlc.arg(start_id)
    UNION ALL
    SELECT containment.parent_id
    FROM containment
    JOIN ancestors ON containment.child_id = ancestors.id
    WHERE containment.board_id = sqlc.arg(scope_board_id)
)
SELECT ancestors.id
FROM ancestors;
