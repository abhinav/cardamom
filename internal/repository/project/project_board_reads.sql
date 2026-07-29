-- name: ProjectListAllBoards :many
SELECT id, project_id, name, description, created_at
FROM boards
ORDER BY project_id, name, id;

-- name: ProjectGetBoard :one
SELECT id, project_id, name, description, created_at
FROM boards
WHERE id = sqlc.arg(id);

-- name: ProjectListBoardsForSoleSelection :many
SELECT id, project_id, name, description, created_at
FROM boards
ORDER BY id
LIMIT 2;
