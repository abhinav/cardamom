package store

import (
	"bytes"
	"database/sql"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNullableConflictFieldsPreservePublishedStore(t *testing.T) {
	db := populatedStoreBeforeNullableFields(t)
	before := readNullableMigrationRecords(t, db)
	schemaBefore := nullableMigrationSchemaObjects(t, db)
	provider, err := newStoreMigrationProvider(db)
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), 20261029220000)
	require.NoError(t, err)
	after := readNullableMigrationRecords(t, db)
	for table, records := range before {
		assert.ElementsMatch(t, records, after[table], table)
	}
	assert.ElementsMatch(t, append(schemaBefore, "index:board_pins_order"), nullableMigrationSchemaObjects(t, db))

	var violations int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM pragma_foreign_key_check").Scan(&violations))
	assert.Zero(t, violations)
	_, err = db.ExecContext(t.Context(), "INSERT INTO issue_search_fts(issue_search_fts, rank) VALUES ('integrity-check', 1)")
	require.NoError(t, err)
	for _, test := range []struct {
		term string
		hits int
	}{
		{term: "origintitle", hits: 1},
		{term: "originsummary", hits: 1},
		{term: "origindetails", hits: 1},
		{term: "originresult", hits: 1},
		{term: "originstate", hits: 2},
		{term: "originaction", hits: 2},
		{term: "originlog", hits: 1},
	} {
		var hits int
		require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM issue_search_fts WHERE body MATCH ?", test.term).Scan(&hits))
		assert.Equal(t, test.hits, hits, test.term)
	}
	// Deleted log sequences remain retired after the table replacement.
	_, err = db.ExecContext(t.Context(), `
INSERT INTO issue_log_entries (id, board_id, issue_uid, kind, author, committer, body, created_at)
SELECT 'log_22222222222222222222222222222222', board_id, uid, 'post', 'alice', 'alice', 'Later log', 2
FROM issues WHERE id='card-one'`)
	require.NoError(t, err)
	var sequence int64
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT local_sequence FROM issue_log_entries WHERE id='log_22222222222222222222222222222222'").Scan(&sequence))
	assert.Greater(t, sequence, int64(41))

	_, err = db.ExecContext(t.Context(), "UPDATE issues SET id='card-retired' WHERE id='card-one'")
	assert.ErrorContains(t, err, "retired reference")
	_, err = db.ExecContext(t.Context(), "UPDATE issues SET uid=randomblob(16) WHERE id='card-one'")
	assert.ErrorContains(t, err, "immutable")
	_, err = db.ExecContext(t.Context(), "INSERT INTO issue_reference_markers(id) VALUES ('card-one')")
	assert.ErrorContains(t, err, "belongs to an issue")
	_, err = db.ExecContext(t.Context(), `
INSERT INTO issue_labels(board_id, issue_uid, label)
SELECT 'other-board', uid, 'invalid' FROM issues WHERE id='card-one'`)
	assert.ErrorContains(t, err, "FOREIGN KEY")
}

func TestNullableConflictFieldsPermitAbsentCurrentValues(t *testing.T) {
	db := populatedStoreBeforeNullableFields(t)
	provider, err := newStoreMigrationProvider(db)
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), 20261029220000)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `
UPDATE issues SET title=NULL, kind=NULL, lifecycle=NULL, priority=NULL WHERE id='card-two';
UPDATE boards SET name=NULL WHERE id='board'`)
	require.NoError(t, err)
	var title, kind, lifecycle, name sql.NullString
	var priority sql.NullInt64
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT issue.title, issue.kind, issue.lifecycle, issue.priority, board.name
FROM issues AS issue JOIN boards AS board ON board.id=issue.board_id
WHERE issue.id='card-two'`).Scan(&title, &kind, &lifecycle, &priority, &name))
	assert.False(t, title.Valid)
	assert.False(t, kind.Valid)
	assert.False(t, lifecycle.Valid)
	assert.False(t, priority.Valid)
	assert.False(t, name.Valid)
	var hits int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM issue_search_fts WHERE body MATCH 'secondedited'").Scan(&hits))
	assert.Zero(t, hits)
	_, err = db.ExecContext(t.Context(), "UPDATE issues SET title='Resolvedtitle' WHERE id='card-two'")
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM issue_search_fts WHERE body MATCH 'resolvedtitle'").Scan(&hits))
	assert.Equal(t, 1, hits)
	_, err = db.ExecContext(t.Context(), "UPDATE issues SET closed_at=2 WHERE id='card-two'")
	assert.Error(t, err, "an unresolved lifecycle cannot retain a terminal timestamp")
	_, err = db.ExecContext(t.Context(), "UPDATE issues SET waiting_reason='waiting', waiting_since=2 WHERE id='card-two'")
	require.NoError(t, err, "known waiting survives unresolved kind and lifecycle")
	var waitingReason string
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT waiting_reason FROM issues WHERE id='card-two'").Scan(&waitingReason))
	assert.Equal(t, "waiting", waitingReason)
	_, err = db.ExecContext(t.Context(), "UPDATE issues SET lifecycle='closed', closed_at=2 WHERE id='card-two'")
	assert.Error(t, err, "known terminal lifecycle cannot retain waiting state")
	_, err = db.ExecContext(t.Context(), "UPDATE issues SET title='', priority=9 WHERE id='card-two'")
	assert.Error(t, err)
	_, err = db.ExecContext(t.Context(), "UPDATE boards SET name='' WHERE id='board'")
	assert.Error(t, err)
	_, err = db.ExecContext(t.Context(), `
INSERT INTO issues(uid, id, board_id, created_at, updated_at)
VALUES (randomblob(16), 'card-unresolved', 'board', 2, 2)`)
	require.NoError(t, err, "a new unresolved issue has no title search document")
	var documents int
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT count(*) FROM issue_search_documents
WHERE issue_uid=(SELECT uid FROM issues WHERE id='card-unresolved')`).Scan(&documents))
	assert.Zero(t, documents)
}

func TestNullableConflictFieldsFailureRetainsPreviousSchema(t *testing.T) {
	db := populatedStoreBeforeNullableFields(t)
	before := readNullableMigrationRecords(t, db)
	body, err := migrationFiles.ReadFile("migrations/20261029220000_nullable_conflict_fields.sql")
	require.NoError(t, err)
	body = append(bytes.Clone(body), []byte("\nINSERT INTO missing_rollback_test_table VALUES (1);\n")...)
	provider, err := newMigrationProvider(db, fstest.MapFS{
		"20261029220000_nullable_conflict_fields.sql": {Data: body},
	})
	require.NoError(t, err)
	_, err = provider.Up(t.Context())
	assert.ErrorContains(t, err, "missing_rollback_test_table")
	after := readNullableMigrationRecords(t, db)
	for table, records := range before {
		assert.ElementsMatch(t, records, after[table], table)
	}
	version, err := provider.GetDBVersion(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(20261029120000), version)
	_, err = db.ExecContext(t.Context(), "UPDATE issues SET title=NULL WHERE id='card-two'")
	assert.Error(t, err)
	assert.False(t, tableExists(t, db, "issues_next"))
}

// populatedStoreBeforeNullableFields carries a historical issue graph through
// its released migrations. It includes graph edges, claims, records, upload
// receipts, a deleted log sequence, retained mutation bytes, and retired IDs.
func populatedStoreBeforeNullableFields(t *testing.T) *sql.DB {
	t.Helper()
	db := openMigrationTestDatabase(t)
	applySchemaBeforeIssueUIDs(t, db)
	_, err := db.ExecContext(t.Context(), `
INSERT INTO projects (id, name, created_at) VALUES ('project', 'Project', 1);
INSERT INTO boards (id, project_id, name, created_at)
VALUES ('board', 'project', 'Board', 1);
INSERT INTO issues (
    id, board_id, title, kind, lifecycle, priority,
    created_at, updated_at, summary, details
) VALUES (
    'card-one', 'board', 'Origintitle', 'task', 'open', 2,
    1, 1, 'Originsummary', 'Origindetails'
);
INSERT INTO issues (
    id, board_id, title, kind, lifecycle, priority, created_at, updated_at
) VALUES ('card-two', 'board', 'Secondtitle', 'workstream', 'open', 2, 1, 1);
UPDATE boards SET name = 'Edited board' WHERE id = 'board';
UPDATE issues SET title = 'Secondedited', updated_at = 2 WHERE id = 'card-two';
INSERT INTO issue_external_keys (board_id, external_key, issue_id)
VALUES ('board', ' ', 'card-two');
INSERT INTO active_claims (issue_id, board_id, actor, started_at, started_revision)
VALUES ('card-one', 'board', 'alice', 1, 1);
INSERT INTO dependencies (board_id, issue_id, prerequisite_id)
VALUES ('board', 'card-one', 'card-two');
INSERT INTO containment (board_id, child_id, parent_id)
VALUES ('board', 'card-one', 'card-two');
INSERT INTO board_pins (board_id, issue_id, position)
VALUES ('board', 'card-one', 1);
INSERT INTO issue_states (issue_id, board_id, body, next_action)
VALUES ('card-one', 'board', 'Originstate', 'Originaction');
INSERT INTO issue_results (issue_id, board_id, body)
VALUES ('card-one', 'board', 'Originresult');
INSERT INTO issue_log_entries (
    id, board_id, issue_id, kind, author, committer, body, created_at
) VALUES (
    'log_11111111111111111111111111111111', 'board', 'card-one',
    'post', 'alice', 'alice', 'Originlog', 1
);
INSERT INTO issue_log_entries (
    local_sequence, id, board_id, issue_id, kind,
    author, committer, body, created_at
) VALUES (
    41, 'log_ffffffffffffffffffffffffffffffff', 'board', 'card-one',
    'post', 'alice', 'alice', 'Discardedlog', 1
);
DELETE FROM issue_log_entries
WHERE id = 'log_ffffffffffffffffffffffffffffffff';
INSERT INTO attachment_blobs (digest, size_bytes)
VALUES ('sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 4);
INSERT INTO attachments (
    board_id, id, origin_issue_id, blob_digest, blob_size_bytes,
    filename, media_type, lifecycle, created_actor, created_at, created_revision
) VALUES (
    'board', 'att_aaaaaaaaaaaaaaaaaaaaaaaaaa', 'card-one',
    'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 4,
    'first.txt', 'text/plain', 'active', 'alice', 1, 1
), (
    'board', 'att_bbbbbbbbbbbbbbbbbbbbbbbbba', NULL,
    'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 4,
    'second.txt', 'text/plain', 'active', 'alice', 1, 1
);

INSERT INTO issue_labels (board_id, issue_id, label) VALUES ('board', 'card-one', 'retained');
INSERT INTO issues (id, board_id, title, kind, lifecycle, priority, created_at, updated_at, closed_at)
VALUES ('card-checkpoint', 'board', 'Checkpoint', 'checkpoint', 'closed', 2, 1, 1, 1);
INSERT INTO checkpoint_decisions (board_id, issue_id, outcome, reason, decided_at, revision)
VALUES ('board', 'card-checkpoint', 'approved', 'Accepted', 1, 1);
INSERT INTO attachment_uploads (id, board_id, origin_issue_id, filename, actor, state, accepted_offset, expires_at)
VALUES ('upload-retained', 'board', 'card-one', 'upload.txt', 'alice', 'active', 0, 10);
INSERT INTO attachment_uploads (
    id, board_id, origin_issue_id, filename, actor, state,
    accepted_offset, expires_at, expected_size_bytes, attachment_id
) VALUES (
    'upload-committed', 'board', 'card-one', 'first.txt', 'alice', 'committed',
    4, 10, 4, 'att_aaaaaaaaaaaaaaaaaaaaaaaaaa'
);
INSERT INTO issue_log_entries (
    local_sequence, id, board_id, issue_id, kind,
    author, committer, body, next_action, created_at
) VALUES (
    2, 'log_33333333333333333333333333333333', 'board', 'card-one', 'state_snapshot',
    'alice', 'alice', 'Originstate', 'Originaction', 2
);
UPDATE issue_states SET author='alice', updated_at=2,
    snapshot_log_entry_id='log_33333333333333333333333333333333'
WHERE issue_id='card-one';
UPDATE store_state SET current_revision=2;
UPDATE boards SET revision=2;
UPDATE issues SET revision=2;
`)
	require.NoError(t, err)
	provider, err := newStoreMigrationProvider(db)
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), 20261029120000)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `
INSERT INTO issue_reference_markers(id) VALUES ('card-retired');
INSERT INTO issue_reference_candidates(marker_id, issue_uid, board_id, replacement_id)
SELECT 'card-retired', uid, board_id, id FROM issues WHERE id IN ('card-one', 'card-two')`)
	require.NoError(t, err)
	return db
}

// readNullableMigrationRecords captures complete canonical rows by column name.
// Column order and SQLite row IDs are not part of the preservation contract.
func readNullableMigrationRecords(t *testing.T, db *sql.DB) map[string][]map[string]any {
	t.Helper()
	snapshot := make(map[string][]map[string]any)
	for _, table := range []string{
		"boards", "issues", "active_claims", "issue_labels", "dependencies", "containment",
		"issue_external_keys", "issue_results", "issue_log_entries", "issue_states", "checkpoint_decisions",
		"attachment_blobs", "attachments", "attachment_uploads", "board_pins",
		"board_replica_identities", "board_mutation_commits", "board_mutation_parts", "board_mutation_tips",
		"board_mutation_blob_refs", "issue_reference_markers", "issue_reference_candidates",
	} {
		rows, err := db.QueryContext(t.Context(), "SELECT * FROM "+table+" ORDER BY rowid")
		require.NoError(t, err)
		columns, err := rows.Columns()
		require.NoError(t, err)
		var records []map[string]any
		for rows.Next() {
			values := make([]any, len(columns))
			targets := make([]any, len(columns))
			for index := range values {
				targets[index] = &values[index]
			}
			require.NoError(t, rows.Scan(targets...))
			record := make(map[string]any, len(columns))
			for index, column := range columns {
				value := values[index]
				if blob, ok := value.([]byte); ok {
					value = bytes.Clone(blob)
				}
				record[column] = value
			}
			records = append(records, record)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		require.NotEmpty(t, records, table)
		snapshot[table] = records
	}
	return snapshot
}

// nullableMigrationSchemaObjects identifies the indexes and triggers whose
// behavior must survive replacement of their owning tables.
func nullableMigrationSchemaObjects(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT type || ':' || name FROM sqlite_schema WHERE type IN ('index', 'trigger') AND name NOT LIKE 'sqlite_%' ORDER BY type, name")
	require.NoError(t, err)
	defer func() { assert.NoError(t, rows.Close()) }()
	var objects []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		objects = append(objects, name)
	}
	require.NoError(t, rows.Err())
	return objects
}
