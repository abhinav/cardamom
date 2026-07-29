-- name: ProjectCreateInitialProject :exec
INSERT INTO projects (id, name, created_at, issue_id_prefix)
VALUES (
    sqlc.arg(id),
    sqlc.arg(name),
    sqlc.arg(created_at),
    sqlc.narg(issue_id_prefix)
);

-- name: ProjectCreateInitialBoard :exec
INSERT INTO boards (id, project_id, name, created_at)
VALUES (
    sqlc.arg(id),
    sqlc.arg(project_id),
    sqlc.arg(name),
    sqlc.arg(created_at)
);
