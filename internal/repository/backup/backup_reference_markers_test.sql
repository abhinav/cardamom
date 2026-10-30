-- One marker includes a board-two candidate and an unavailable board.
-- Backup must preserve this evidence or fail.
INSERT INTO issue_reference_markers (id) VALUES ('retired-1');
INSERT INTO issue_reference_candidates (
    marker_id, issue_uid, board_id, replacement_id
)
SELECT 'retired-1', uid, board_id, id
FROM issues WHERE id = 'two-1';
INSERT INTO issue_reference_candidates (
    marker_id, issue_uid, board_id, replacement_id
) VALUES (
    'retired-1', X'01010101010101010101010101010101',
    'board-remote', 'remote-1'
);
