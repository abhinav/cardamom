package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestInitializerInitializesProjectCatalog(t *testing.T) {
	for _, test := range []struct {
		name      string
		boardName *string
		wantBoard string
		ids       []idStep
	}{
		{
			name: "FirstBoard", boardName: new("Planning"), wantBoard: "Planning",
			ids: []idStep{
				{value: "project-initial"},
				{value: "board-initial"},
			},
		},
		{
			name: "NoBoard",
			ids: []idStep{
				{value: "project-initial"},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "board.sqlite3")
			result, err := NewInitializer(Config{
				Clock:    fixedClock{now: time.Unix(10, 0).UTC()},
				IDSource: &sequenceIDs{steps: test.ids},
			}).InitializeStore(
				t.Context(),
				project.StoreInitializationRequest{
					Dir: dir, ProjectName: "cardamom", BoardName: test.boardName,
				},
			)
			require.NoError(t, err)
			require.NotNil(t, result.Namespace)
			assert.True(t, result.DatabaseWritten)
			assert.Equal(t, int(store.SchemaVersion()), result.SchemaVersion)
			assert.Equal(t, mustProjectID(t, "project-initial"), result.Namespace.Project.ID())
			assert.Equal(t, "cardamom", result.Namespace.Project.Name())
			assert.Equal(t, int64(10), result.Namespace.Project.Created().Unix())
			if test.wantBoard == "" {
				assert.Nil(t, result.Namespace.Board)
			} else {
				require.NotNil(t, result.Namespace.Board)
				assert.Equal(t, mustBoardID(t, "board-initial"), result.Namespace.Board.ID())
				assert.Equal(t, test.wantBoard, result.Namespace.Board.Name())
			}

			persistence, err := store.Open(t.Context(), store.Config{Path: path})
			require.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
			view, err := persistence.View(t.Context())
			require.NoError(t, err)
			revision, err := view.CanonicalRevision(t.Context())
			require.NoError(t, err)
			require.NoError(t, view.Done())
			assert.EqualValues(t, 1, revision)

			repository := New(persistence, Config{})
			projects, err := repository.ListProjects(t.Context())
			require.NoError(t, err)
			require.Len(t, projects, 1)
			assert.Equal(t, "cardamom", projects[0].Name())
			boards, err := repository.ListAllBoards(t.Context())
			require.NoError(t, err)
			if test.wantBoard == "" {
				assert.Empty(t, boards)
			} else {
				require.Len(t, boards, 1)
				assert.Equal(t, test.wantBoard, boards[0].Name())
			}
		})
	}
}

func TestInitializerDoesNotPublishPartialNamespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.sqlite3")
	identityErr := errors.New("entropy unavailable")
	boardName := "Planning"

	result, err := NewInitializer(Config{
		Clock: fixedClock{now: time.Unix(10, 0).UTC()},
		IDSource: &sequenceIDs{steps: []idStep{
			{value: "project-initial"},
			{err: identityErr},
		}},
	}).InitializeStore(t.Context(), project.StoreInitializationRequest{
		Dir: dir, ProjectName: "cardamom", BoardName: &boardName,
	})

	assert.ErrorIs(t, err, identityErr)
	assert.False(t, result.DatabaseWritten)
	assert.Nil(t, result.Namespace)
	_, statErr := os.Stat(path)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
