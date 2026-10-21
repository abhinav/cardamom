package store

import (
	"bytes"
	"database/sql"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/boardhistory"
)

func TestIssueUIDMigrationPreservesSearch(t *testing.T) {
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
)
`)
	require.NoError(t, err)

	files := make(fstest.MapFS)
	for _, name := range []string{
		"20260909120000_issue_uids.sql",
		"20260916172153_board_replica_identities.sql",
		"20260916180000_board_mutation_history.sql",
	} {
		body, readErr := migrationFiles.ReadFile("migrations/" + name)
		require.NoError(t, readErr)
		files[name] = &fstest.MapFile{Data: body}
	}
	current, err := newMigrationProvider(db, files)
	require.NoError(t, err)
	results, err := current.Up(t.Context())
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, int64(20260909120000), results[0].Source.Version)
	assert.Equal(t, int64(20260916172153), results[1].Source.Version)
	assert.Equal(t, int64(20260916180000), results[2].Source.Version)
	baseline, err := newMigrationProvider(db, nil, goMigrations...)
	require.NoError(t, err)
	results, err = baseline.Up(t.Context())
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, int64(20260916180001), results[0].Source.Version)

	var uid []byte
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT uid FROM issues WHERE id = 'card-one'
`).Scan(&uid))
	assert.Len(t, uid, 16)
	assert.NotEqual(t, make([]byte, 16), uid)
	var secondUID []byte
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT uid FROM issues WHERE id = 'card-two'
`).Scan(&secondUID))
	assert.Len(t, secondUID, 16)
	assert.NotEqual(t, uid, secondUID)
	var graphRows int
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT count(*)
FROM dependencies AS dependency
JOIN containment AS relation
    ON relation.child_uid = dependency.issue_uid
    AND relation.parent_uid = dependency.prerequisite_uid
JOIN board_pins AS pin ON pin.issue_uid = dependency.issue_uid
WHERE dependency.issue_uid = ?
    AND dependency.prerequisite_uid = ?
`, uid, secondUID).Scan(&graphRows))
	assert.Equal(t, 1, graphRows)
	var stateBody, nextAction, resultBody, logBody string
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT state.body, state.next_action, result.body, log.body
FROM issue_states AS state
JOIN issue_results AS result ON result.issue_uid = state.issue_uid
JOIN issue_log_entries AS log ON log.issue_uid = state.issue_uid
WHERE state.issue_uid = ?
`, uid).Scan(&stateBody, &nextAction, &resultBody, &logBody))
	assert.Equal(t, "Originstate", stateBody)
	assert.Equal(t, "Originaction", nextAction)
	assert.Equal(t, "Originresult", resultBody)
	assert.Equal(t, "Originlog", logBody)

	var documents int
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT count(*) FROM issue_search_documents
WHERE issue_uid = ?
`, uid).Scan(&documents))
	assert.Equal(t, 6, documents)
	for _, term := range []string{
		"origintitle", "originsummary", "origindetails", "originstate",
		"originaction", "originresult", "originlog",
	} {
		var indexed int
		require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT count(*) FROM issue_search_fts WHERE body MATCH ?
`, term).Scan(&indexed))
		assert.Equal(t, 1, indexed, term)
	}
	_, err = db.ExecContext(t.Context(), `
INSERT INTO issue_search_fts(issue_search_fts, rank)
VALUES ('integrity-check', 1)
`)
	require.NoError(t, err)
	var invalidReferences int
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT count(*) FROM pragma_foreign_key_check
`).Scan(&invalidReferences))
	assert.Zero(t, invalidReferences)
	assertBaselineMutationEvents(t, db, 12)
	var retainedBlobs int
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT count(*) FROM board_mutation_blob_refs
`).Scan(&retainedBlobs))
	assert.Equal(t, 1, retainedBlobs)

	_, err = db.ExecContext(t.Context(), `
UPDATE issues SET title = 'Editedtitle' WHERE id = 'card-one'
`)
	require.NoError(t, err)
	var oldTitle, newTitle int
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT count(*) FROM issue_search_fts WHERE body MATCH 'origintitle'
`).Scan(&oldTitle))
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT count(*) FROM issue_search_fts WHERE body MATCH 'editedtitle'
`).Scan(&newTitle))
	assert.Zero(t, oldTitle)
	assert.Equal(t, 1, newTitle)
	_, err = db.ExecContext(t.Context(), `
INSERT INTO issue_log_entries (
    id, board_id, issue_uid, kind, author, committer, body, created_at
) VALUES (
    'log_22222222222222222222222222222222', 'board', ?,
    'post', 'alice', 'alice', 'Laterlog', 2
)
`, uid)
	require.NoError(t, err)
	var laterSequence int64
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT local_sequence FROM issue_log_entries
WHERE id = 'log_22222222222222222222222222222222'
`).Scan(&laterSequence))
	assert.Greater(t, laterSequence, int64(41))

	_, err = db.ExecContext(t.Context(), `DELETE FROM attachments WHERE origin_issue_uid = ?`, uid)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `DELETE FROM issues WHERE id = 'card-one'`)
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT count(*) FROM issue_search_documents
`).Scan(&documents))
	assert.Equal(t, 1, documents)
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT count(*) FROM issue_search_fts WHERE body MATCH 'editedtitle'
`).Scan(&newTitle))
	assert.Zero(t, newTitle)
}

func assertBaselineMutationEvents(t *testing.T, db *sql.DB, wantCount int) {
	t.Helper()
	var markerBytes []byte
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT marker_bytes
FROM board_mutation_commits
WHERE board_id = 'board' AND sequence = 1
`).Scan(&markerBytes))
	marker, err := boardhistory.DecodeMarker(markerBytes)
	require.NoError(t, err)
	assert.Empty(t, marker.Header().Frontier)

	var events []boardhistory.Event
	for retained, err := range boardhistory.MutationEvents(
		marker,
		func(index int64) ([]byte, error) {
			var part []byte
			err := db.QueryRowContext(t.Context(), `
SELECT bytes
FROM board_mutation_parts
WHERE board_id = 'board' AND writer_uid = ?
    AND sequence = 1 AND part_index = ?
`, marker.Header().Writer.Bytes(), index).Scan(&part)
			return part, err
		},
	) {
		require.NoError(t, err)
		events = append(events, retained.Event)
	}
	require.Len(t, events, wantCount)
	assert.IsType(t, (*boardhistory.BoardOrigin)(nil), events[0].Payload)
	var boardOrigin *boardhistory.BoardOrigin
	var boardOriginAt time.Time
	var issueOrigins int
	var editedIssueOrigin *boardhistory.IssueOrigin
	var editedIssueOriginAt time.Time
	var stateAt, resultAt time.Time
	for _, event := range events {
		switch value := event.Payload.(type) {
		case *boardhistory.BoardOrigin:
			boardOrigin = value
			boardOriginAt = event.OccurredAt
		case *boardhistory.IssueOrigin:
			issueOrigins++
			if value.PublicID == "card-two" {
				editedIssueOrigin = value
				editedIssueOriginAt = event.OccurredAt
			}
		case *boardhistory.IssueState:
			stateAt = event.OccurredAt
		case *boardhistory.IssueResult:
			resultAt = event.OccurredAt
		}
	}
	require.NotNil(t, boardOrigin)
	assert.Equal(t, "Edited board", boardOrigin.Name)
	assert.Equal(t, time.Unix(1, 0).UTC(), boardOrigin.CreatedAt)
	assert.True(t, boardOriginAt.After(boardOrigin.CreatedAt))
	assert.Equal(t, 2, issueOrigins)
	require.NotNil(t, editedIssueOrigin)
	assert.Equal(t, "Secondedited", editedIssueOrigin.Title)
	assert.Equal(t, time.Unix(2, 0).UTC(), editedIssueOrigin.UpdatedAt)
	assert.True(t, editedIssueOriginAt.After(editedIssueOrigin.UpdatedAt))
	assert.True(t, stateAt.After(time.Unix(1, 0).UTC()))
	assert.True(t, resultAt.After(time.Unix(1, 0).UTC()))
}

func TestIssueUIDMigrationFailurePreservesOldSchema(t *testing.T) {
	db := openMigrationTestDatabase(t)
	applySchemaBeforeIssueUIDs(t, db)
	_, err := db.ExecContext(t.Context(), `
INSERT INTO projects (id, name, created_at) VALUES ('project', 'Project', 1);
INSERT INTO boards (id, project_id, name, created_at)
VALUES ('board', 'project', 'Board', 1);
INSERT INTO issues (
    id, board_id, title, kind, lifecycle, priority, created_at, updated_at
) VALUES ('card-one', 'board', 'Savedtitle', 'task', 'open', 2, 1, 1)`)
	require.NoError(t, err)
	migration, err := migrationFiles.ReadFile(
		"migrations/20260909120000_issue_uids.sql",
	)
	require.NoError(t, err)
	migration = append(bytes.Clone(migration), []byte(`
INSERT INTO missing_rollback_test_table VALUES (1);
`)...)
	failure, err := newMigrationProvider(db, fstest.MapFS{
		"20260909120000_issue_uids.sql": {Data: migration},
	})
	require.NoError(t, err)
	_, err = failure.Up(t.Context())
	assert.ErrorContains(t, err, "missing_rollback_test_table")
	assert.False(t, tableExists(t, db, "issues_next"))
	var title string
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT title FROM issues WHERE id = 'card-one'
`).Scan(&title))
	assert.Equal(t, "Savedtitle", title)
	var indexed int
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT count(*) FROM issue_search_fts WHERE body MATCH 'savedtitle'
`).Scan(&indexed))
	assert.Equal(t, 1, indexed)
	version, err := failure.GetDBVersion(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(20260904120000), version)
}

func TestBoardHistoryBaselineFailureRollsBackMutation(t *testing.T) {
	db := openMigrationTestDatabase(t)
	applySchemaBeforeIssueUIDs(t, db)
	_, err := db.ExecContext(t.Context(), `
INSERT INTO projects (id, name, created_at) VALUES ('project', 'Project', 1);
INSERT INTO boards (id, project_id, name, created_at)
VALUES ('board', 'project', 'Board', 'not-a-time')
`)
	require.NoError(t, err)

	current, err := newStoreMigrationProvider(db)
	require.NoError(t, err)
	_, err = current.Up(t.Context())
	assert.ErrorContains(t, err, "unsupported stored time")

	var parts int
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT count(*) FROM board_mutation_parts
`).Scan(&parts))
	assert.Zero(t, parts)

	version, err := current.GetDBVersion(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(20260916180000), version)
}

func applySchemaBeforeIssueUIDs(t *testing.T, db *sql.DB) {
	t.Helper()
	files := make(fstest.MapFS)
	for _, name := range []string{
		"20260726181403_baseline.sql",
		"20260729090000_board_copy.sql",
		"20260811090000_board_archival.sql",
		"20260812120000_board_pins.sql",
		"20260904120000_issue_search.sql",
	} {
		body, err := migrationFiles.ReadFile("migrations/" + name)
		require.NoError(t, err)
		files[name] = &fstest.MapFile{Data: body}
	}
	provider, err := newMigrationProvider(db, files)
	require.NoError(t, err)
	_, err = provider.Up(t.Context())
	require.NoError(t, err)
}
