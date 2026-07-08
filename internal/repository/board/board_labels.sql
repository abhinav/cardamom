-- name: BoardDeleteIssueLabels :exec
DELETE FROM issue_labels
WHERE board_id = sqlc.arg(board_id)
    AND issue_id = sqlc.arg(issue_id);

-- name: BoardInsertIssueLabel :exec
INSERT INTO issue_labels (
    board_id,
    issue_id,
    label
) VALUES (
    sqlc.arg(board_id),
    sqlc.arg(issue_id),
    sqlc.arg(label)
);

-- name: BoardListLabelsForIssue :many
SELECT label
FROM issue_labels
WHERE board_id = sqlc.arg(board_id)
    AND issue_id = sqlc.arg(issue_id)
ORDER BY label;

-- name: BoardListAllIssueLabels :many
SELECT issue_id, label
FROM issue_labels
WHERE board_id = sqlc.arg(board_id)
ORDER BY issue_id, label;
