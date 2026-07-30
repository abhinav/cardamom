-- name: ProjectCreateProject :exec
INSERT INTO projects (
    id,
    name,
    created_at,
    issue_id_prefix
) VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(created_at),
    sqlc.narg(issue_id_prefix)
);
