-- name: BoardDeleteIssueDependencies :exec
DELETE FROM dependencies
WHERE board_id = sqlc.arg(board_id)
    AND issue_id = sqlc.arg(issue_id);

-- name: BoardInsertIssueDependency :exec
INSERT INTO dependencies (
    board_id,
    issue_id,
    prerequisite_id
) VALUES (
    sqlc.arg(board_id),
    sqlc.arg(issue_id),
    sqlc.arg(prerequisite_id)
);

-- name: BoardDeleteIssueParent :exec
DELETE FROM containment
WHERE board_id = sqlc.arg(board_id)
    AND child_id = sqlc.arg(child_id);

-- name: BoardInsertIssueParent :exec
INSERT INTO containment (
    board_id,
    child_id,
    parent_id
) VALUES (
    sqlc.arg(board_id),
    sqlc.arg(child_id),
    sqlc.arg(parent_id)
);

-- name: BoardInsertIssueExternalKey :exec
INSERT INTO issue_external_keys (
    board_id,
    external_key,
    issue_id
) VALUES (
    sqlc.arg(board_id),
    sqlc.arg(external_key),
    sqlc.arg(issue_id)
);
