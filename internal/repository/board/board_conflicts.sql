-- name: BoardListConflicts :many
SELECT conflict.id, conflict.kind, conflict.issue_uid, conflict.related_uid,
       conflict.member_key, conflict.revision, conflict.category,
       count(alternative.alternative_id) AS alternative_count
FROM board_conflicts AS conflict
LEFT JOIN board_conflict_alternatives AS alternative ON alternative.conflict_id = conflict.id
WHERE conflict.board_id = sqlc.arg(board_id)
  AND conflict.id > sqlc.arg(after_id)
  AND (sqlc.narg(issue_uid) IS NULL OR EXISTS (
      SELECT 1 FROM board_conflict_issues AS affected
      WHERE affected.conflict_id = conflict.id AND affected.issue_uid = sqlc.narg(issue_uid)
  ))
GROUP BY conflict.id
ORDER BY conflict.id
LIMIT sqlc.arg(page_limit);

-- name: BoardGetConflict :one
SELECT * FROM board_conflicts WHERE board_id = sqlc.arg(board_id) AND id = sqlc.arg(id);

-- name: BoardReadConflictAlternatives :many
SELECT alternative_id, kind, issue_uid, related_uid, member_key, waiting_present FROM board_conflict_alternatives
WHERE conflict_id = sqlc.arg(conflict_id) ORDER BY alternative_id;

-- name: BoardReadConflictOrigins :many
SELECT alternative_id, writer_uid, sequence, ordinal FROM board_conflict_origins
WHERE conflict_id = sqlc.arg(conflict_id)
ORDER BY alternative_id, writer_uid, sequence, ordinal;

-- name: BoardReadIssueConflicts :many
SELECT affected.issue_uid, affected.field, conflict.id, conflict.member_key
FROM board_conflict_issues AS affected
JOIN board_conflicts AS conflict ON conflict.id = affected.conflict_id
WHERE conflict.board_id = sqlc.arg(board_id)
  AND (sqlc.narg(issue_uid) IS NULL OR affected.issue_uid = sqlc.narg(issue_uid))
ORDER BY affected.issue_uid, conflict.id, affected.field;

-- name: BoardInsertConflict :exec
INSERT INTO board_conflicts (id, board_id, kind, category, issue_uid, related_uid, member_key, revision)
VALUES (sqlc.arg(id), sqlc.arg(board_id), sqlc.arg(kind), sqlc.arg(category), sqlc.narg(issue_uid),
        sqlc.narg(related_uid), sqlc.arg(member_key), sqlc.arg(revision));

-- name: BoardInsertConflictAlternative :exec
INSERT INTO board_conflict_alternatives (conflict_id, alternative_id, kind, issue_uid, related_uid, member_key, waiting_present)
VALUES (sqlc.arg(conflict_id), sqlc.arg(alternative_id), sqlc.arg(kind), sqlc.narg(issue_uid), sqlc.narg(related_uid), sqlc.arg(member_key), sqlc.arg(waiting_present));

-- name: BoardInsertConflictOrigin :exec
INSERT INTO board_conflict_origins (conflict_id, alternative_id, writer_uid, sequence, ordinal)
VALUES (sqlc.arg(conflict_id), sqlc.arg(alternative_id), sqlc.arg(writer_uid), sqlc.arg(sequence), sqlc.arg(ordinal));

-- name: BoardInsertConflictIssue :exec
INSERT INTO board_conflict_issues (conflict_id, issue_uid, field)
VALUES (sqlc.arg(conflict_id), sqlc.arg(issue_uid), sqlc.arg(field));

-- name: BoardDeleteConflict :exec
DELETE FROM board_conflicts WHERE board_id = sqlc.arg(board_id) AND id = sqlc.arg(id);

-- name: BoardReadConflictIssueReferences :many
SELECT DISTINCT issue.uid, issue.id
FROM board_conflict_issues AS affected
JOIN issues AS issue ON issue.uid = affected.issue_uid
WHERE affected.conflict_id = sqlc.arg(conflict_id)
ORDER BY issue.id;

-- name: BoardGetIssueIDByUID :one
SELECT id FROM issues WHERE board_id = sqlc.arg(board_id) AND uid = sqlc.arg(issue_uid);

-- name: BoardReadConflictAlternativeOrigin :one
SELECT alternative.kind, alternative.issue_uid, alternative.related_uid, alternative.member_key,
       origin.writer_uid, origin.sequence, origin.ordinal
FROM board_conflict_alternatives AS alternative
JOIN board_conflicts AS conflict ON conflict.id = alternative.conflict_id
JOIN board_conflict_origins AS origin ON origin.conflict_id = alternative.conflict_id
    AND origin.alternative_id = alternative.alternative_id
WHERE conflict.board_id = sqlc.arg(board_id) AND conflict.id = sqlc.arg(conflict_id)
    AND alternative.alternative_id = sqlc.arg(alternative_id)
ORDER BY origin.writer_uid, origin.sequence, origin.ordinal LIMIT 1;

-- name: BoardListExternalKeyConflicts :many
SELECT member_key, id FROM board_conflicts
WHERE board_id = sqlc.arg(board_id) AND kind = 'board.external_key';

-- name: BoardReadArchiveConflict :one
SELECT id FROM board_conflicts WHERE board_id = sqlc.arg(board_id) AND kind = 'board.archive.set';

-- name: BoardReadIdentityConflict :one
SELECT id FROM board_conflicts WHERE board_id = sqlc.arg(board_id)
AND kind IN ('board.issue_id_prefix.set', 'board.issue_id_strategy.set') ORDER BY id LIMIT 1;

-- name: BoardReadConflictAffectedIssues :many
SELECT issue_uid, field FROM board_conflict_issues WHERE conflict_id = sqlc.arg(conflict_id)
ORDER BY issue_uid, field;

-- name: BoardReadStrategyConflict :one
SELECT id FROM board_conflicts WHERE board_id = sqlc.arg(board_id) AND kind = 'board.issue_id_strategy.set';
