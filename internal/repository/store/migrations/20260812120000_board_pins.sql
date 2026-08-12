-- +goose Up

-- Pin limits inherit through project and board configuration. A lower limit
-- controls later admission without constraining an existing pin collection.
ALTER TABLE projects ADD COLUMN board_pins_max_count INTEGER
    CHECK (board_pins_max_count IS NULL OR board_pins_max_count >= 0);
ALTER TABLE boards ADD COLUMN board_pins_max_count INTEGER
    CHECK (board_pins_max_count IS NULL OR board_pins_max_count >= 0);

-- Board pins retain insertion order and cannot reference an issue from another
-- board. Removing an issue removes its pin without renumbering retained pins.
CREATE TABLE board_pins (
    board_id TEXT NOT NULL,
    issue_id TEXT NOT NULL,
    position INTEGER NOT NULL CHECK (position > 0),
    PRIMARY KEY (board_id, issue_id),
    UNIQUE (board_id, position),
    FOREIGN KEY (board_id, issue_id)
        REFERENCES issues(board_id, id) ON DELETE CASCADE
);

-- +goose Down

DROP TABLE board_pins;
ALTER TABLE boards DROP COLUMN board_pins_max_count;
ALTER TABLE projects DROP COLUMN board_pins_max_count;
