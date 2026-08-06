-- name: ProjectListBackupBoards :many
SELECT
    project.id AS project_id,
    project.name AS project_name,
    project.created_at AS project_created_at,
    board.id AS board_id
FROM boards AS board
JOIN projects AS project ON project.id = board.project_id
ORDER BY board.id;

-- name: ProjectListSelectedBackupBoards :many
SELECT
    project.id AS project_id,
    project.name AS project_name,
    project.created_at AS project_created_at,
    board.id AS board_id
FROM boards AS board
JOIN projects AS project ON project.id = board.project_id
WHERE board.id IN (sqlc.slice('board_ids'))
ORDER BY board.id;
