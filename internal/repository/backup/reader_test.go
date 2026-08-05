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
	repositoryboard "go.abhg.dev/cardamom/internal/repository/board"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestReader_CaptureUsesOneRetainedViewForAllAndSelectedBoards(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.sqlite3")
	readerStore := openBackupTestStore(t, path)
	writerStore := openBackupTestStore(t, path)
	seedBackupTestBoards(t, readerStore)
	reader := New(readerStore)
	reader.boards = &mutatingBackupBoardReader{
		delegate: &repositoryboard.BackupReader{},
		writer:   writerStore,
	}

	all, err := reader.Capture(
		t.Context(),
		domainbackup.AllBoards(),
		configuration.Overrides{},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(2), all.SourceRevision)
	require.Len(t, all.Projects, 2)
	require.Len(t, all.Boards, 2)
	assert.Equal(t, "board-one", all.Boards[0].Snapshot.Board.ID)
	assert.Equal(t, "original", *all.Boards[1].Snapshot.Board.Description)
	for _, captured := range all.Boards {
		assert.Equal(t, all.SourceLineageID, captured.Snapshot.SourceLineageID)
		assert.Equal(t, all.SourceRevision, captured.Snapshot.SourceRevision)
	}

	selected, err := reader.Capture(
		t.Context(),
		mustBackupSelection(t, "board-two"),
		configuration.Overrides{},
	)
	require.NoError(t, err)
	require.Len(t, selected.Projects, 1)
	require.Len(t, selected.Boards, 1)
	assert.Equal(t, "project-two", selected.Projects[0].ID.String())
	assert.Equal(t, "board-two", selected.Boards[0].Snapshot.Board.ID)

	_, err = reader.Capture(
		t.Context(),
		mustBackupSelection(t, "board-missing"),
		configuration.Overrides{},
	)
	require.Error(t, err)
	assert.ErrorContains(t, err, `board "board-missing" not found`)
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
	captured, err := reader.Capture(
		t.Context(),
		mustBackupSelection(t, "board-one"),
		configuration.Overrides{},
	)
	require.NoError(t, err)
	require.Len(t, captured.Boards, 1)
	assert.Equal(t, []boardcopy.CopyIssue{{
		ID:        "one-1",
		Title:     "Committed",
		Kind:      "task",
		Lifecycle: "open",
		Priority:  2,
		CreatedAt: backupTestTime(1000),
		UpdatedAt: backupTestTime(1001),
	}}, captured.Boards[0].Snapshot.Issues)
	require.Len(t, captured.Boards[0].Snapshot.Attachments, 1)
	assert.Equal(
		t,
		"att_aaaaaaaaaaaaaaaaaaaaaaaaaa",
		captured.Boards[0].Snapshot.Attachments[0].ID,
	)
}

type mutatingBackupBoardReader struct {
	delegate *repositoryboard.BackupReader
	writer   *store.Store
	reads    int
}

func (r *mutatingBackupBoardReader) ReadBackupSnapshot(
	ctx context.Context,
	view *store.View,
	boardID board.ID,
	storeOverrides configuration.Overrides,
	source repositoryboard.BackupSource,
) (boardcopy.CopySnapshot, error) {
	if r.reads == 1 {
		change, err := r.writer.Change(ctx)
		if err != nil {
			return boardcopy.CopySnapshot{}, err
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
			return boardcopy.CopySnapshot{}, err
		}
		if err := change.Commit(); err != nil {
			return boardcopy.CopySnapshot{}, err
		}
	}
	r.reads++
	return r.delegate.ReadBackupSnapshot(
		ctx,
		view,
		boardID,
		storeOverrides,
		source,
	)
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
INSERT INTO projects (id, name, created_at)
VALUES
    ('project-one', 'Project one', 1000),
    ('project-two', 'Project two', 1000);
INSERT INTO boards (id, project_id, name, description, created_at, revision)
VALUES
    ('board-one', 'project-one', 'Board one', NULL, 1000, 1),
    ('board-two', 'project-two', 'Board two', 'original', 1000, 2);
INSERT INTO issues (
    id, board_id, title, kind, lifecycle, priority, created_at, updated_at,
    revision
) VALUES (
    'one-1', 'board-one', 'Committed', 'task', 'open', 2, 1000, 1001, 1
);
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
