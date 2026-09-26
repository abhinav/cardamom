-- name: ProjectInsertBoardReplicaIdentity :exec
INSERT INTO board_replica_identities (board_id, writer_uid)
VALUES (sqlc.arg(board_id), sqlc.arg(writer_uid));

-- name: ProjectGetBoardWriterUID :one
SELECT writer_uid
FROM board_replica_identities
WHERE board_id = sqlc.arg(board_id);
