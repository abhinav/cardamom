-- name: BoardHistoryNextCloneCommit :one
SELECT writer_uid, sequence, marker_bytes
FROM board_mutation_commits
WHERE board_id = sqlc.arg(board_id)
    AND (writer_uid > sqlc.arg(after_writer_uid)
        OR (writer_uid = sqlc.arg(after_writer_uid)
            AND sequence > sqlc.arg(after_sequence)))
ORDER BY writer_uid, sequence LIMIT 1;

-- name: BoardHistoryClonePart :one
SELECT bytes FROM board_mutation_parts
WHERE board_id = sqlc.arg(board_id) AND writer_uid = sqlc.arg(writer_uid)
    AND sequence = sqlc.arg(sequence) AND part_index = sqlc.arg(part_index);

-- name: BoardHistoryWriterHasCommits :one
SELECT EXISTS (
    SELECT 1 FROM board_mutation_commits
    WHERE board_id = sqlc.arg(board_id) AND writer_uid = sqlc.arg(writer_uid)
);
