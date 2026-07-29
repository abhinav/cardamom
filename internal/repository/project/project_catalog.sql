-- name: ProjectListProjects :many
SELECT id, name, created_at
FROM projects
ORDER BY name, id;
