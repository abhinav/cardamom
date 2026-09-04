-- +goose Up

-- Full-text search is a derived projection of canonical issue records.
-- One document per field or Log entry preserves result provenance and keeps
-- large Log histories from accumulating relevance across several records.
CREATE TABLE issue_search_documents (
    rowid INTEGER PRIMARY KEY,
    board_id TEXT NOT NULL,
    issue_id TEXT NOT NULL,
    field TEXT NOT NULL CHECK (
        field IN ('title', 'summary', 'details', 'state', 'result', 'log')
    ),
    record_id TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL CHECK (length(trim(body)) > 0),
    FOREIGN KEY (board_id, issue_id)
        REFERENCES issues(board_id, id) ON DELETE CASCADE,
    UNIQUE (issue_id, field, record_id),
    CHECK (
        (field = 'log' AND record_id <> '')
        OR (field <> 'log' AND record_id = '')
    )
);

CREATE INDEX issue_search_documents_by_board_issue
    ON issue_search_documents (board_id, issue_id, field, record_id);

-- The content table keeps board and record ownership available to repository
-- queries while FTS5 owns only the token index for each document body.
CREATE VIRTUAL TABLE issue_search_fts USING fts5(
    body,
    content = 'issue_search_documents',
    content_rowid = 'rowid',
    tokenize = 'unicode61'
);

-- +goose StatementBegin
CREATE TRIGGER issue_search_documents_fts_insert
AFTER INSERT ON issue_search_documents
BEGIN
    INSERT INTO issue_search_fts(rowid, body) VALUES (NEW.rowid, NEW.body);
END;
-- +goose StatementEnd

-- External-content FTS5 tables require a delete command that includes the
-- indexed value which previously occupied the row.
-- +goose StatementBegin
CREATE TRIGGER issue_search_documents_fts_delete
AFTER DELETE ON issue_search_documents
BEGIN
    INSERT INTO issue_search_fts(issue_search_fts, rowid, body)
    VALUES ('delete', OLD.rowid, OLD.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_search_documents_fts_update
AFTER UPDATE OF body ON issue_search_documents
BEGIN
    INSERT INTO issue_search_fts(issue_search_fts, rowid, body)
    VALUES ('delete', OLD.rowid, OLD.body);
    INSERT INTO issue_search_fts(rowid, body) VALUES (NEW.rowid, NEW.body);
END;
-- +goose StatementEnd

-- Issue scalar records share one canonical row. Replacing optional fields
-- removes absent documents before upserting the retained values.
-- +goose StatementBegin
CREATE TRIGGER issues_search_insert
AFTER INSERT ON issues
BEGIN
    INSERT INTO issue_search_documents(board_id, issue_id, field, body)
    VALUES (NEW.board_id, NEW.id, 'title', NEW.title);
    INSERT INTO issue_search_documents(board_id, issue_id, field, body)
    SELECT NEW.board_id, NEW.id, 'summary', NEW.summary
    WHERE NEW.summary IS NOT NULL;
    INSERT INTO issue_search_documents(board_id, issue_id, field, body)
    SELECT NEW.board_id, NEW.id, 'details', NEW.details
    WHERE NEW.details IS NOT NULL;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issues_search_update
AFTER UPDATE OF title, summary, details ON issues
BEGIN
    INSERT INTO issue_search_documents(board_id, issue_id, field, body)
    VALUES (NEW.board_id, NEW.id, 'title', NEW.title)
    ON CONFLICT(issue_id, field, record_id)
    DO UPDATE SET body = excluded.body;

    DELETE FROM issue_search_documents
    WHERE issue_id = NEW.id AND field = 'summary' AND NEW.summary IS NULL;
    INSERT INTO issue_search_documents(board_id, issue_id, field, body)
    SELECT NEW.board_id, NEW.id, 'summary', NEW.summary
    WHERE NEW.summary IS NOT NULL
    ON CONFLICT(issue_id, field, record_id)
    DO UPDATE SET body = excluded.body;

    DELETE FROM issue_search_documents
    WHERE issue_id = NEW.id AND field = 'details' AND NEW.details IS NULL;
    INSERT INTO issue_search_documents(board_id, issue_id, field, body)
    SELECT NEW.board_id, NEW.id, 'details', NEW.details
    WHERE NEW.details IS NOT NULL
    ON CONFLICT(issue_id, field, record_id)
    DO UPDATE SET body = excluded.body;
END;
-- +goose StatementEnd

-- Delete documents before the issue row so the document trigger can remove
-- the corresponding external-content FTS5 entries.
-- +goose StatementBegin
CREATE TRIGGER issues_search_delete
BEFORE DELETE ON issues
BEGIN
    DELETE FROM issue_search_documents WHERE issue_id = OLD.id;
END;
-- +goose StatementEnd

-- State documents combine the mutable body and next action because both form
-- the current recovery position exposed by the State record.
-- +goose StatementBegin
CREATE TRIGGER issue_states_search_insert
AFTER INSERT ON issue_states
BEGIN
    INSERT INTO issue_search_documents(board_id, issue_id, field, body)
    VALUES (
        NEW.board_id,
        NEW.issue_id,
        'state',
        NEW.body || CASE
            WHEN NEW.next_action IS NULL THEN ''
            ELSE char(10) || char(10) || NEW.next_action
        END
    );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_states_search_update
AFTER UPDATE OF body, next_action ON issue_states
BEGIN
    UPDATE issue_search_documents
    SET body = NEW.body || CASE
        WHEN NEW.next_action IS NULL THEN ''
        ELSE char(10) || char(10) || NEW.next_action
    END
    WHERE issue_id = NEW.issue_id AND field = 'state';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_states_search_delete
AFTER DELETE ON issue_states
BEGIN
    DELETE FROM issue_search_documents
    WHERE issue_id = OLD.issue_id AND field = 'state';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_results_search_insert
AFTER INSERT ON issue_results
BEGIN
    INSERT INTO issue_search_documents(board_id, issue_id, field, body)
    VALUES (NEW.board_id, NEW.issue_id, 'result', NEW.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_results_search_update
AFTER UPDATE OF body ON issue_results
BEGIN
    UPDATE issue_search_documents
    SET body = NEW.body
    WHERE issue_id = NEW.issue_id AND field = 'result';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_results_search_delete
AFTER DELETE ON issue_results
BEGIN
    DELETE FROM issue_search_documents
    WHERE issue_id = OLD.issue_id AND field = 'result';
END;
-- +goose StatementEnd

-- A Log document retains the stable Log ID used for exact follow-up reads.
-- State snapshots include their preserved next action in the searchable body.
-- +goose StatementBegin
CREATE TRIGGER issue_log_entries_search_insert
AFTER INSERT ON issue_log_entries
BEGIN
    INSERT INTO issue_search_documents(
        board_id,
        issue_id,
        field,
        record_id,
        body
    ) VALUES (
        NEW.board_id,
        NEW.issue_id,
        'log',
        NEW.id,
        NEW.body || CASE
            WHEN NEW.next_action IS NULL THEN ''
            ELSE char(10) || char(10) || NEW.next_action
        END
    );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_log_entries_search_update
AFTER UPDATE OF body, next_action ON issue_log_entries
BEGIN
    UPDATE issue_search_documents
    SET body = NEW.body || CASE
        WHEN NEW.next_action IS NULL THEN ''
        ELSE char(10) || char(10) || NEW.next_action
    END
    WHERE issue_id = NEW.issue_id
        AND field = 'log'
        AND record_id = NEW.id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_log_entries_search_delete
AFTER DELETE ON issue_log_entries
BEGIN
    DELETE FROM issue_search_documents
    WHERE issue_id = OLD.issue_id
        AND field = 'log'
        AND record_id = OLD.id;
END;
-- +goose StatementEnd

-- Backfill every current canonical record. The rebuild makes the final FTS5
-- index depend only on the completed document projection.
INSERT INTO issue_search_documents(board_id, issue_id, field, body)
SELECT board_id, id, 'title', title FROM issues
UNION ALL
SELECT board_id, id, 'summary', summary FROM issues WHERE summary IS NOT NULL
UNION ALL
SELECT board_id, id, 'details', details FROM issues WHERE details IS NOT NULL
UNION ALL
SELECT
    board_id,
    issue_id,
    'state',
    body || CASE
        WHEN next_action IS NULL THEN ''
        ELSE char(10) || char(10) || next_action
    END
FROM issue_states
UNION ALL
SELECT board_id, issue_id, 'result', body FROM issue_results;

INSERT INTO issue_search_documents(
    board_id,
    issue_id,
    field,
    record_id,
    body
)
SELECT
    board_id,
    issue_id,
    'log',
    id,
    body || CASE
        WHEN next_action IS NULL THEN ''
        ELSE char(10) || char(10) || next_action
    END
FROM issue_log_entries;

INSERT INTO issue_search_fts(issue_search_fts) VALUES ('rebuild');
