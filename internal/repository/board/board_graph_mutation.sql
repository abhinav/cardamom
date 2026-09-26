-- name: BoardDeleteIssueDependencies :exec
DELETE FROM dependencies
WHERE board_id = sqlc.arg(board_id)
    AND issue_uid = sqlc.arg(issue_uid);

-- name: BoardInsertIssueDependency :exec
INSERT INTO dependencies (
    board_id,
    issue_uid,
    prerequisite_uid
) VALUES (
    sqlc.arg(board_id),
    sqlc.arg(issue_uid),
    sqlc.arg(prerequisite_uid)
);

-- name: BoardDeleteIssueParent :exec
DELETE FROM containment
WHERE board_id = sqlc.arg(board_id)
    AND child_uid = sqlc.arg(child_uid);

-- name: BoardInsertIssueParent :exec
INSERT INTO containment (
    board_id,
    child_uid,
    parent_uid
) VALUES (
    sqlc.arg(board_id),
    sqlc.arg(child_uid),
    sqlc.arg(parent_uid)
);

-- name: BoardInsertIssueExternalKey :exec
INSERT INTO issue_external_keys (
    board_id,
    external_key,
    issue_uid
) VALUES (
    sqlc.arg(board_id),
    sqlc.arg(external_key),
    sqlc.arg(issue_uid)
);
