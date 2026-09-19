-- name: BoardDeleteIssueLabels :exec
DELETE FROM issue_labels
WHERE board_id = sqlc.arg(board_id)
    AND issue_uid = sqlc.arg(issue_uid);

-- name: BoardInsertIssueLabel :exec
INSERT INTO issue_labels (
    board_id,
    issue_uid,
    label
) VALUES (
    sqlc.arg(board_id),
    sqlc.arg(issue_uid),
    sqlc.arg(label)
);

-- name: BoardListLabelsForIssue :many
SELECT label
FROM issue_labels
WHERE board_id = sqlc.arg(board_id)
    AND issue_uid = sqlc.arg(issue_uid)
ORDER BY label;

-- name: BoardListAllIssueLabels :many
SELECT issue.id AS issue_id, label.label
FROM issue_labels AS label
JOIN issues AS issue ON issue.uid = label.issue_uid
WHERE label.board_id = sqlc.arg(board_id)
ORDER BY issue.id, label.label;
