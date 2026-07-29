package changeconnect

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"go.abhg.dev/cardamom/internal/board"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	repositoryproject "go.abhg.dev/cardamom/internal/repository/project"
	"go.abhg.dev/cardamom/internal/repository/store"
)

var (
	_ CanonicalRevisionReader = (*store.Store)(nil)
	_ ChangeBoardLister       = (*repositoryproject.Repository)(nil)
)

func TestPollingSourceCatchUpAndReconnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		revisions := &testCanonicalRevisionReader{revision: 7}
		source := NewPollingSource(PollingConfig{
			Revisions: revisions,
			Boards:    &testChangeBoardLister{},
			Interval:  time.Hour,
		})
		request := WatchRequest{BoardIDs: []board.ID{"board-1"}}

		first, err := source.Subscribe(t.Context(), request)
		require.NoError(t, err)
		change, err := first.Receive(t.Context())
		require.NoError(t, err)
		assert.Equal(t, CommittedChange{
			BoardID: "board-1", Revision: 7,
		}, change)

		revisions.revision = 10
		change, err = first.Receive(t.Context())
		require.NoError(t, err)
		assert.Equal(t, uint64(10), change.Revision)

		reconnected, err := source.Subscribe(t.Context(), request)
		require.NoError(t, err)
		change, err = reconnected.Receive(t.Context())
		require.NoError(t, err)
		assert.Equal(t, uint64(10), change.Revision)
	})
}

func TestPollingSourceCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := NewPollingSource(PollingConfig{
			Revisions: &testCanonicalRevisionReader{},
			Boards:    &testChangeBoardLister{},
			Interval:  time.Hour,
		})
		subscription, err := source.Subscribe(t.Context(), WatchRequest{
			BoardIDs: []board.ID{"board-1"},
		})
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err = subscription.Receive(ctx)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestPollingSourceEmitsOneInvalidationPerBoard(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		projectOne := testProject(t, "project-1", "Project One")
		boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", nil)
		boardTwo := testBoard(t, "board-2", projectOne.ID(), "Board Two", nil)
		source := NewPollingSource(PollingConfig{
			Revisions: &testCanonicalRevisionReader{revision: 4},
			Boards: &testChangeBoardLister{boards: []*board.State{
				boardOne,
				boardTwo,
			}},
			Interval: time.Hour,
		})
		subscription, err := source.Subscribe(t.Context(), WatchRequest{
			BoardIDs:  []board.ID{"board-1", "board-2"},
			AllBoards: true,
		})
		require.NoError(t, err)

		first, err := subscription.Receive(t.Context())
		require.NoError(t, err)
		second, err := subscription.Receive(t.Context())
		require.NoError(t, err)
		assert.Equal(t, board.ID("board-1"), first.BoardID)
		assert.Equal(t, board.ID("board-2"), second.BoardID)
		assert.Equal(t, first.Revision, second.Revision)
	})
}

func TestPollingSourceFindsBoardCreatedAfterEmptySubscription(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		projectOne := testProject(t, "project-1", "Project One")
		boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", nil)
		revisions := &testCanonicalRevisionReader{}
		boards := &testChangeBoardLister{}
		source := NewPollingSource(PollingConfig{
			Revisions: revisions,
			Boards:    boards,
			Interval:  time.Hour,
		})
		subscription, err := source.Subscribe(t.Context(), WatchRequest{
			AllBoards: true,
		})
		require.NoError(t, err)

		revisions.revision = 1
		boards.boards = []*board.State{boardOne}
		change, err := subscription.Receive(t.Context())
		require.NoError(t, err)
		assert.Equal(t, board.ID("board-1"), change.BoardID)
		assert.Equal(t, uint64(1), change.Revision)
	})
}

type testCanonicalRevisionReader struct {
	revision int64
	err      error
}

// CanonicalRevision returns the scripted committed head.
func (r *testCanonicalRevisionReader) CanonicalRevision(context.Context) (int64, error) {
	return r.revision, r.err
}

type testChangeBoardLister struct {
	boards []*board.State
	err    error
}

// ListAllBoards returns the scripted committed board catalog.
func (l *testChangeBoardLister) ListAllBoards(context.Context) ([]*board.State, error) {
	return l.boards, l.err
}
