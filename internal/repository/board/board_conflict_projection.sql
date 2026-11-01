-- name: BoardSetIssueTitleProjection :exec
UPDATE issues SET title = sqlc.narg(value), updated_at = COALESCE(sqlc.narg(updated_at), updated_at)
WHERE board_id = sqlc.arg(board_id) AND uid = sqlc.arg(issue_uid);

-- name: BoardSetIssueKindProjection :exec
UPDATE issues SET kind = sqlc.narg(value), updated_at = COALESCE(sqlc.narg(updated_at), updated_at)
WHERE board_id = sqlc.arg(board_id) AND uid = sqlc.arg(issue_uid);

-- name: BoardSetIssuePriorityProjection :exec
UPDATE issues SET priority = sqlc.narg(value), updated_at = COALESCE(sqlc.narg(updated_at), updated_at)
WHERE board_id = sqlc.arg(board_id) AND uid = sqlc.arg(issue_uid);

-- name: BoardSetIssueSummaryProjection :exec
UPDATE issues SET summary = sqlc.narg(value), updated_at = COALESCE(sqlc.narg(updated_at), updated_at)
WHERE board_id = sqlc.arg(board_id) AND uid = sqlc.arg(issue_uid);

-- name: BoardSetIssueDetailsProjection :exec
UPDATE issues SET details = sqlc.narg(value), updated_at = COALESCE(sqlc.narg(updated_at), updated_at)
WHERE board_id = sqlc.arg(board_id) AND uid = sqlc.arg(issue_uid);

-- name: BoardSetIssueLifecycleProjection :exec
UPDATE issues SET lifecycle = sqlc.narg(lifecycle), closed_at = sqlc.narg(closed_at), updated_at = COALESCE(sqlc.narg(updated_at), updated_at)
WHERE board_id = sqlc.arg(board_id) AND uid = sqlc.arg(issue_uid);

-- name: BoardSetIssueWaitingProjection :exec
UPDATE issues SET waiting_reason = sqlc.narg(reason), waiting_since = sqlc.narg(since), updated_at = COALESCE(sqlc.narg(updated_at), updated_at)
WHERE board_id = sqlc.arg(board_id) AND uid = sqlc.arg(issue_uid);

-- name: BoardDeleteIssueResultProjection :exec
DELETE FROM issue_results WHERE board_id = sqlc.arg(board_id) AND issue_uid = sqlc.arg(issue_uid);

-- name: BoardDeleteCheckpointProjection :exec
DELETE FROM checkpoint_decisions WHERE board_id = sqlc.arg(board_id) AND issue_uid = sqlc.arg(issue_uid);

-- name: BoardDeleteDependencyProjection :exec
DELETE FROM dependencies WHERE board_id = sqlc.arg(board_id)
    AND issue_uid = sqlc.arg(issue_uid) AND prerequisite_uid = sqlc.arg(prerequisite_uid);

-- name: BoardDeleteExternalKeyProjection :exec
DELETE FROM issue_external_keys WHERE board_id = sqlc.arg(board_id) AND external_key = sqlc.arg(external_key);

-- name: BoardDeleteLabelProjection :exec
DELETE FROM issue_labels WHERE board_id = sqlc.arg(board_id)
    AND issue_uid = sqlc.arg(issue_uid) AND label = sqlc.arg(label);

-- name: BoardSetNameProjection :exec
UPDATE boards SET name = sqlc.narg(value) WHERE id = sqlc.arg(board_id);

-- name: BoardSetDescriptionProjection :exec
UPDATE boards SET description = sqlc.narg(value) WHERE id = sqlc.arg(board_id);

-- name: BoardSetIDPrefixProjection :exec
UPDATE boards SET issue_id_prefix = sqlc.narg(value) WHERE id = sqlc.arg(board_id);

-- name: BoardSetIDStrategyProjection :exec
UPDATE boards SET issue_id_strategy = sqlc.narg(value) WHERE id = sqlc.arg(board_id);

-- name: BoardSetSummaryLimitProjection :exec
UPDATE boards SET issue_summary_max_bytes = sqlc.narg(value) WHERE id = sqlc.arg(board_id);

-- name: BoardSetAttachmentLimitProjection :exec
UPDATE boards SET attachment_max_bytes = sqlc.narg(value) WHERE id = sqlc.arg(board_id);

-- name: BoardSetPinLimitProjection :exec
UPDATE boards SET board_pins_max_count = sqlc.narg(value) WHERE id = sqlc.arg(board_id);

-- name: BoardSetArchiveProjection :exec
UPDATE boards SET archived_at = sqlc.narg(archived_at), archived_by = sqlc.narg(archived_by), archive_reason = sqlc.narg(archive_reason)
WHERE id = sqlc.arg(board_id);
