package information

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	informationdomain "go.abhg.dev/cardamom/internal/information"
	"go.abhg.dev/cardamom/internal/repository/board"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestReader_ReadUsesOneSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "information.db")
	readerStore := openInformationStore(t, path)
	writerStore := openInformationStore(t, path)
	reader := &Reader{
		store: readerStore,
		board: &writeBeforeBoardInventory{writer: writerStore},
	}

	first, err := reader.Read(t.Context())
	require.NoError(t, err)
	assert.Zero(t, first.Revision.Current)
	assert.Zero(t, first.Issues.Total)

	after, err := (&Reader{
		store: readerStore,
		board: canonicalRevisionBoardInventory{},
	}).Read(t.Context())
	require.NoError(t, err)
	assert.Equal(t, informationdomain.Revision{Current: 1}, after.Revision)
	assert.Equal(t, 1, after.Issues.Total)
}

func openInformationStore(t *testing.T, path string) *store.Store {
	t.Helper()
	persistence, err := store.Open(t.Context(), store.Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	return persistence
}

// writeBeforeBoardInventory publishes a revision after Reader pins its View.
type writeBeforeBoardInventory struct {
	writer *store.Store
}

func (i *writeBeforeBoardInventory) ReadIssueInventory(
	ctx context.Context,
	view *store.View,
) (out board.IssueInventory, err error) {
	change, err := i.writer.Change(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	reservation, err := change.ReserveRevision(ctx)
	if err != nil {
		return out, err
	}
	if err := change.PublishRevision(ctx, reservation); err != nil {
		return out, err
	}
	if err := change.Commit(); err != nil {
		return out, err
	}
	return canonicalRevisionBoardInventory{}.ReadIssueInventory(ctx, view)
}

// canonicalRevisionBoardInventory exposes the retained revision as issue total.
type canonicalRevisionBoardInventory struct{}

func (canonicalRevisionBoardInventory) ReadIssueInventory(
	ctx context.Context,
	view *store.View,
) (board.IssueInventory, error) {
	revision, err := view.CanonicalRevision(ctx)
	return board.IssueInventory{Total: int(revision)}, err
}
