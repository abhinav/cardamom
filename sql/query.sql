-- name: GetIssue :one
SELECT id, title, type, status, priority, agent, assignee, created, updated, closed
FROM issues
WHERE id = ?;
