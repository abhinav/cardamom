package backup

import (
	"context"
	_ "embed"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	domainbackup "go.abhg.dev/cardamom/internal/backup"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestCaptureRejectsReferenceMarkersBeforeOutput(t *testing.T) {
	for _, test := range []struct {
		name      string
		selection domainbackup.Selection
	}{
		{name: "AllBoards", selection: domainbackup.AllBoards()},
		{name: "SelectedBoards", selection: mustBackupSelection(t, "board-one", "board-two")},
		{name: "MarkerParticipant", selection: mustBackupSelection(t, "board-two")},
	} {
		t.Run(test.name, func(t *testing.T) {
			persistence := openBackupTestStore(t, filepath.Join(t.TempDir(), "board.sqlite3"))
			seedBackupTestBoards(t, persistence)
			seedBackupReferenceMarker(t, persistence)
			destination := &backupTestCaptureDestination{}

			_, err := New(persistence).Capture(t.Context(), test.selection, configuration.Overrides{}, destination)

			assert.ErrorContains(t, err, "retired-1")
			if !test.selection.IsAll() {
				assert.ErrorContains(t, err, "board-two")
			}
			assert.Empty(t, destination.projects)
			assert.Empty(t, destination.boards)
		})
	}
}

func TestCaptureAllowsBoardOutsideReferenceMarker(t *testing.T) {
	persistence := openBackupTestStore(t, filepath.Join(t.TempDir(), "board.sqlite3"))
	seedBackupTestBoards(t, persistence)
	seedBackupReferenceMarker(t, persistence)
	destination := &backupTestCaptureDestination{}

	result, err := New(persistence).Capture(t.Context(), mustBackupSelection(t, "board-one"), configuration.Overrides{}, destination)

	require.NoError(t, err)
	assert.Equal(t, 1, result.Boards)
	assert.Len(t, destination.projects, 1)
	assert.Len(t, destination.boards, 1)
}

func TestBackupReferenceMarkerRejectionPreservesDestination(t *testing.T) {
	directory := t.TempDir()
	persistence := openBackupTestStore(t, filepath.Join(directory, "board.sqlite3"))
	seedBackupTestBoards(t, persistence)
	seedBackupReferenceMarker(t, persistence)
	destination := filepath.Join(directory, "saved.cardamom")
	require.NoError(t, os.WriteFile(destination, []byte("previous archive"), 0o600))
	operation := domainbackup.NewOperation(domainbackup.OperationConfig{
		Source: New(persistence), Blobs: &markerBackupBlobs{},
		Configuration: &markerBackupConfiguration{}, Publisher: &domainbackup.FilePublisher{},
	})

	_, err := operation.Write(t.Context(), domainbackup.WriteRequest{
		Destination: destination, Selection: mustBackupSelection(t, "board-two"),
	})

	assert.ErrorContains(t, err, "retired-1")
	contents, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, "previous archive", string(contents))
}

func TestCaptureAllRejectsReferenceMarkerWithoutLocalCandidates(t *testing.T) {
	persistence := openBackupTestStore(t, filepath.Join(t.TempDir(), "board.sqlite3"))
	seedBackupTestBoards(t, persistence)
	seedBackupReferenceMarker(t, persistence)
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `UPDATE issue_reference_candidates
 SET issue_uid=X'02020202020202020202020202020202', board_id='board-away', replacement_id='away-1'
 WHERE board_id='board-two'`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	destination := &backupTestCaptureDestination{}
	_, err = New(persistence).Capture(t.Context(), domainbackup.AllBoards(), configuration.Overrides{}, destination)
	require.ErrorContains(t, err, "retired-1")
	assert.Empty(t, destination.projects)
	assert.Empty(t, destination.boards)
	_, err = New(persistence).Capture(t.Context(), mustBackupSelection(t, "board-two"), configuration.Overrides{}, destination)
	require.NoError(t, err)
	assert.Len(t, destination.boards, 1)
}

//go:embed backup_reference_markers_test.sql
var backupReferenceMarkerFixture string

func seedBackupReferenceMarker(t *testing.T, persistence *store.Store) {
	t.Helper()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), backupReferenceMarkerFixture)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
}

// Rejected capture must finish before any attachment body is opened.
type markerBackupBlobs struct{}

func (*markerBackupBlobs) OpenCopyBlob(context.Context, attachment.BlobDescriptor) (io.ReadCloser, error) {
	return nil, errors.New("unexpected attachment read after incompatible backup capture")
}

type markerBackupConfiguration struct{}

func (*markerBackupConfiguration) ReadStoreConfiguration(context.Context) (configuration.Overrides, error) {
	return configuration.Overrides{}, nil
}
