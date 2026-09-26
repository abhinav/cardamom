-- name: ProjectListAllBoards :many
SELECT id, project_id, name, description, created_at,
       archived_at, archived_by, archive_reason
FROM boards
ORDER BY project_id, name, id;

-- name: ProjectListProjectBoards :many
-- Project aggregate discovery excludes archived boards; explicit identity reads
-- use ProjectGetBoard below and include either lifecycle state.
SELECT id, project_id, name, description, created_at,
       archived_at, archived_by, archive_reason
FROM boards
WHERE project_id = sqlc.arg(project_id) AND archived_at IS NULL
ORDER BY name, id;

-- name: ProjectGetBoard :one
SELECT id, project_id, name, description, created_at,
       archived_at, archived_by, archive_reason
FROM boards
WHERE id = sqlc.arg(id);

-- name: ProjectListBoardsForSoleSelection :many
-- Archived boards do not make ambient board selection ambiguous.
SELECT id, project_id, name, description, created_at,
       archived_at, archived_by, archive_reason
FROM boards
WHERE archived_at IS NULL
ORDER BY id
LIMIT 2;

-- name: ProjectBoardHasActiveClaim :one
SELECT EXISTS (SELECT 1 FROM active_claims WHERE board_id = sqlc.arg(board_id));

-- name: ProjectCountBoardIssues :one
SELECT count(*) FROM issues WHERE board_id = sqlc.arg(board_id);

-- name: ProjectCountBoardIssuesByStatus :one
-- Effective status precedence matches board inventory and archive reporting.
SELECT count(*)
FROM issues AS issue
LEFT JOIN active_claims AS claim ON claim.issue_uid = issue.uid
WHERE issue.board_id = sqlc.arg(board_id)
    AND CASE
        WHEN issue.lifecycle <> 'open' THEN issue.lifecycle
        WHEN claim.issue_uid IS NOT NULL THEN 'in_progress'
        WHEN issue.waiting_reason IS NOT NULL THEN 'waiting'
        WHEN EXISTS (
            SELECT 1
            FROM dependencies AS dependency
            JOIN issues AS prerequisite ON prerequisite.uid = dependency.prerequisite_uid
            WHERE dependency.issue_uid = issue.uid
                AND prerequisite.lifecycle <> 'closed'
        ) THEN 'blocked'
        ELSE 'ready'
    END = sqlc.arg(status);
