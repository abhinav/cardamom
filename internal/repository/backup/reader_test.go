package backup

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainbackup "go.abhg.dev/cardamom/internal/backup"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/project"
	repositoryboard "go.abhg.dev/cardamom/internal/repository/board"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestReader_CaptureUsesOneViewForAllAndSelectedBoards(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.sqlite3")
	readerStore := openBackupTestStore(t, path)
	writerStore := openBackupTestStore(t, path)
	seedBackupTestBoards(t, readerStore)
	reader := New(readerStore)
	reader.boards = &mutatingBackupBoardReader{
		delegate: &repositoryboard.BackupReader{},
		writer:   writerStore,
	}

	allDestination := &backupTestCaptureDestination{}
	all, err := reader.Capture(
		t.Context(),
		domainbackup.AllBoards(),
		configuration.Overrides{},
		allDestination,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(2), all.SourceRevision)
	assert.Equal(t, 2, all.Projects)
	assert.Equal(t, 2, all.Boards)
	require.Len(t, allDestination.projects, 2)
	require.Len(t, allDestination.boards, 2)
	assert.Equal(t, board.ID("board-one"), allDestination.boards[0].boardID)
	header := allDestination.boards[1].records[0].(boardcopy.RecordHeader)
	assert.Equal(t, "original", *header.Board.Description)
	for _, captured := range allDestination.boards {
		header := captured.records[0].(boardcopy.RecordHeader)
		assert.Equal(t, all.SourceLineageID, header.SourceLineageID)
		assert.Equal(t, all.SourceRevision, header.SourceRevision)
	}

	selectedDestination := &backupTestCaptureDestination{}
	selected, err := reader.Capture(
		t.Context(),
		mustBackupSelection(t, "board-two"),
		configuration.Overrides{},
		selectedDestination,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, selected.Projects)
	assert.Equal(t, 1, selected.Boards)
	require.Len(t, selectedDestination.projects, 1)
	require.Len(t, selectedDestination.boards, 1)
	assert.Equal(t, "project-two", selectedDestination.projects[0].ID.String())
	assert.Equal(t, board.ID("board-two"), selectedDestination.boards[0].boardID)
	selectedHeader := selectedDestination.boards[0].records[0].(boardcopy.RecordHeader)
	assert.Equal(t, board.PinLimit(5), selectedHeader.Configuration.Board.Pins.MaxCount)
	var selectedPins []boardcopy.CopyPin
	for _, record := range selectedDestination.boards[0].records {
		if pin, ok := record.(boardcopy.CopyPin); ok {
			selectedPins = append(selectedPins, pin)
		}
	}
	assert.Equal(t, []boardcopy.CopyPin{{Order: 0, IssueID: "two-1"}}, selectedPins)

	_, captureErr := reader.Capture(
		t.Context(),
		mustBackupSelection(t, "board-missing"),
		configuration.Overrides{},
		&backupTestCaptureDestination{},
	)
	require.Error(t, captureErr)
	assert.ErrorContains(t, captureErr, `board "board-missing" not found`)
}

func TestReader_CaptureIgnoresEphemeralActivity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.sqlite3")
	persistence := openBackupTestStore(t, path)
	seedBackupTestBoards(t, persistence)
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
INSERT INTO active_claims (
    issue_id, board_id, actor, started_at, started_revision
) VALUES ('one-1', 'board-one', 'engineer', 1002, 2);
INSERT INTO attachment_uploads (
    id, board_id, filename, actor, state, accepted_offset, expires_at
) VALUES (
    'upload-active', 'board-one', 'pending.txt', 'engineer', 'active', 0, 2000
)`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())

	reader := New(persistence)
	destination := &backupTestCaptureDestination{}
	captured, err := reader.Capture(
		t.Context(),
		mustBackupSelection(t, "board-one"),
		configuration.Overrides{},
		destination,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, captured.Boards)
	require.Len(t, destination.boards, 1)
	var issues []boardcopy.CopyIssue
	var attachments []boardcopy.CopyAttachment
	for _, record := range destination.boards[0].records {
		switch value := record.(type) {
		case boardcopy.CopyIssue:
			issues = append(issues, value)
		case boardcopy.CopyAttachment:
			attachments = append(attachments, value)
		}
	}
	assert.Equal(t, []boardcopy.CopyIssue{{
		ID:        "one-1",
		Title:     "Committed",
		Kind:      "task",
		Lifecycle: "open",
		Priority:  2,
		CreatedAt: backupTestTime(1000),
		UpdatedAt: backupTestTime(1001),
	}}, issues)
	require.Len(t, attachments, 1)
	assert.Equal(
		t,
		"att_aaaaaaaaaaaaaaaaaaaaaaaaaa",
		attachments[0].ID,
	)
}

type mutatingBackupBoardReader struct {
	delegate *repositoryboard.BackupReader
	writer   *store.Store
	reads    int
}

type backupTestCaptureDestination struct {
	projects []project.Snapshot
	boards   []backupTestCapturedBoard
}

func (d *backupTestCaptureDestination) AddProject(snapshot project.Snapshot) error {
	d.projects = append(d.projects, snapshot)
	return nil
}

func (d *backupTestCaptureDestination) AddBoard(
	projectID project.ID,
	boardID board.ID,
	records boardcopy.RecordSequence,
) error {
	var captured []boardcopy.Record
	for record, err := range records {
		if err != nil {
			return err
		}
		captured = append(captured, record)
	}
	d.boards = append(d.boards, backupTestCapturedBoard{
		projectID: projectID,
		boardID:   boardID,
		records:   captured,
	})
	return nil
}

type backupTestCapturedBoard struct {
	projectID project.ID
	boardID   board.ID
	records   []boardcopy.Record
}

func (r *mutatingBackupBoardReader) ReadBackupRecords(
	ctx context.Context,
	view *store.View,
	boardID board.ID,
	storeOverrides configuration.Overrides,
	source repositoryboard.BackupSource,
) boardcopy.RecordSequence {
	sequence := r.delegate.ReadBackupRecords(
		ctx,
		view,
		boardID,
		storeOverrides,
		source,
	)
	return func(yield func(boardcopy.Record, error) bool) {
		if r.reads == 1 {
			change, err := r.writer.Change(ctx)
			if err != nil {
				yield(nil, err)
				return
			}
			defer func() { _ = change.Done() }()
			_, err = change.ExecContext(ctx, `
UPDATE boards
SET description = 'changed', revision = 3
WHERE id = 'board-two';
UPDATE store_state
SET current_revision = 3
WHERE singleton = 1`)
			if err != nil {
				yield(nil, err)
				return
			}
			if err := change.Commit(); err != nil {
				yield(nil, err)
				return
			}
		}
		r.reads++
		for record, err := range sequence {
			if !yield(record, err) {
				return
			}
		}
	}
}

func openBackupTestStore(t *testing.T, path string) *store.Store {
	t.Helper()
	persistence, err := store.Open(t.Context(), store.Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	return persistence
}

func seedBackupTestBoards(t *testing.T, persistence *store.Store) {
	t.Helper()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, change.Done()) })
	_, err = change.ExecContext(t.Context(), `
INSERT INTO projects (id, name, created_at, board_pins_max_count)
VALUES
    ('project-one', 'Project one', 1000, NULL),
    ('project-two', 'Project two', 1000, 5);
INSERT INTO boards (id, project_id, name, description, created_at, revision)
VALUES
    ('board-one', 'project-one', 'Board one', NULL, 1000, 1),
    ('board-two', 'project-two', 'Board two', 'original', 1000, 2);
INSERT INTO issues (
    id, board_id, title, kind, lifecycle, priority, created_at, updated_at,
    revision
) VALUES (
    'one-1', 'board-one', 'Committed', 'task', 'open', 2, 1000, 1001, 1
), (
    'two-1', 'board-two', 'Selected', 'task', 'open', 2, 1000, 1001, 2
);
INSERT INTO board_pins (board_id, issue_id, position)
VALUES
    ('board-one', 'one-1', 1),
    ('board-two', 'two-1', 1);
INSERT INTO attachment_blobs (digest, size_bytes)
VALUES (
    'sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7',
    4
);
INSERT INTO attachments (
    board_id, id, origin_issue_id, blob_digest, blob_size_bytes, filename,
    media_type, lifecycle, created_actor, created_at, created_revision
) VALUES (
    'board-one', 'att_aaaaaaaaaaaaaaaaaaaaaaaaaa', 'one-1',
    'sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7',
    4, 'committed.txt', 'text/plain', 'active', 'engineer', 1001, 1
);
UPDATE store_state
SET current_revision = 2
WHERE singleton = 1`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
}

func mustBackupSelection(t *testing.T, ids ...board.ID) domainbackup.Selection {
	t.Helper()
	selection, err := domainbackup.SelectBoards(ids...)
	require.NoError(t, err)
	return selection
}

func backupTestTime(seconds int64) time.Time {
	return time.Unix(seconds, 0).UTC()
}
