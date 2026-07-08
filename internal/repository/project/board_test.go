package project

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	boardpkg "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestRepositoryCreatesAndListsBoards(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	initializeProjectCatalog(t, persistence, new("First board"), Config{})
	repository := New(persistence, Config{
		Clock: fixedClock{now: time.Unix(20, 0).UTC()},
		IDSource: &sequenceIDs{steps: []idStep{
			{value: "board-two"},
		}},
	})

	projects, err := repository.ListProjects(t.Context())
	require.NoError(t, err)
	require.Len(t, projects, 1)
	board, err := boardpkg.NewService(repository, repository).Create(t.Context(), boardpkg.NewInvocation(""), boardpkg.CreateRequest{
		ProjectID: projects[0].ID().String(), Name: "Second board",
	})
	require.NoError(t, err)
	allBoards, err := repository.ListAllBoards(t.Context())
	require.NoError(t, err)
	selectedBoard, err := repository.Board(t.Context(), board.ID())
	require.NoError(t, err)

	assert.Equal(t, mustBoardID(t, "board-two"), board.ID())
	assert.Len(t, allBoards, 2)
	assert.Equal(t, board, selectedBoard)
	_, err = repository.SoleBoard(t.Context())
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
}

func TestRepositoryRollsBackBoardCreationForUnknownProject(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	initializeProjectCatalog(t, persistence, new("First board"), Config{})
	repository := New(persistence, Config{
		Clock:    fixedClock{now: time.Unix(20, 0).UTC()},
		IDSource: &sequenceIDs{steps: []idStep{{value: "board-invalid"}}},
	})
	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	before, err := view.CanonicalRevision(t.Context())
	require.NoError(t, err)
	require.NoError(t, view.Done())

	_, err = boardpkg.NewService(repository, repository).Create(t.Context(), boardpkg.NewInvocation(""), boardpkg.CreateRequest{
		ProjectID: mustProjectID(t, "project-missing").String(), Name: "Invalid board",
	})
	assert.Error(t, err)

	view, err = persistence.View(t.Context())
	require.NoError(t, err)
	after, err := view.CanonicalRevision(t.Context())
	require.NoError(t, err)
	require.NoError(t, view.Done())
	assert.Equal(t, before, after)
}

func TestRepositoryEditsBoardSettingsAtOneCanonicalRevision(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	initializeProjectCatalog(t, persistence, new("First board"), Config{})
	repository := New(persistence, Config{
		Clock: fixedClock{now: time.Unix(20, 0).UTC()},
	})
	board, err := repository.SoleBoard(t.Context())
	require.NoError(t, err)
	before := canonicalRevision(t, persistence)
	name := "Release operations"
	description := "# Readiness"

	edited, err := boardpkg.NewService(repository, repository).EditSettings(t.Context(), boardpkg.NewInvocation("  captain  "), boardpkg.EditRequest{
		BoardID: board.ID(),
		Settings: boardpkg.SettingsEdit{
			Name: &name, Description: boardpkg.ReplaceDescription(&description),
		},
	})
	require.NoError(t, err)

	assert.Equal(t, name, edited.Name())
	assert.Equal(t, &description, edited.Description())
	assert.Equal(t, before+1, canonicalRevision(t, persistence))
	stored, err := repository.Board(t.Context(), board.ID())
	require.NoError(t, err)
	assert.Equal(t, edited, stored)
}

func TestRepositoryReturnsCompleteBoardsFromEveryRead(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	initializeProjectCatalog(t, persistence, new("First board"), Config{})
	repository := New(persistence, Config{})
	board, err := repository.SoleBoard(t.Context())
	require.NoError(t, err)
	description := "# Readiness"
	board, err = boardpkg.NewService(repository, repository).EditSettings(t.Context(), boardpkg.NewInvocation(""), boardpkg.EditRequest{
		BoardID: board.ID(),
		Settings: boardpkg.SettingsEdit{
			Description: boardpkg.ReplaceDescription(&description),
		},
	})
	require.NoError(t, err)
	allBoards, err := repository.ListAllBoards(t.Context())
	require.NoError(t, err)
	selected, err := repository.Board(t.Context(), board.ID())
	require.NoError(t, err)
	sole, err := repository.SoleBoard(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []*boardpkg.State{board}, allBoards)
	assert.Equal(t, board, selected)
	assert.Equal(t, board, sole)
}

func TestRepositoryDoesNotPublishRevisionForNoOpBoardSettingsEdit(t *testing.T) {
	description := "# Readiness"
	for _, test := range []struct {
		name    string
		request func(*boardpkg.State) boardpkg.EditRequest
	}{
		{
			name: "Name",
			request: func(board *boardpkg.State) boardpkg.EditRequest {
				name := board.Name()
				return boardpkg.EditRequest{
					BoardID: board.ID(), Settings: boardpkg.SettingsEdit{Name: &name},
				}
			},
		},
		{
			name: "Description",
			request: func(board *boardpkg.State) boardpkg.EditRequest {
				return boardpkg.EditRequest{
					BoardID: board.ID(),
					Settings: boardpkg.SettingsEdit{
						Description: boardpkg.ReplaceDescription(&description),
					},
				}
			},
		},
		{
			name: "Combined",
			request: func(board *boardpkg.State) boardpkg.EditRequest {
				name := board.Name()
				return boardpkg.EditRequest{
					BoardID: board.ID(),
					Settings: boardpkg.SettingsEdit{
						Name: &name, Description: boardpkg.ReplaceDescription(&description),
					},
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			persistence, err := store.Open(t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"})
			require.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
			initializeProjectCatalog(t, persistence, new("First board"), Config{})
			repository := New(persistence, Config{})
			board, err := repository.SoleBoard(t.Context())
			require.NoError(t, err)
			board, err = boardpkg.NewService(repository, repository).EditSettings(t.Context(), boardpkg.NewInvocation(""), boardpkg.EditRequest{
				BoardID: board.ID(),
				Settings: boardpkg.SettingsEdit{
					Description: boardpkg.ReplaceDescription(&description),
				},
			})
			require.NoError(t, err)
			before := canonicalRevision(t, persistence)
			repository = New(persistence, Config{IDSource: &sequenceIDs{}})

			edited, err := boardpkg.NewService(repository, repository).EditSettings(t.Context(), boardpkg.NewInvocation(""), test.request(board))

			require.NoError(t, err)
			assert.Equal(t, board, edited)
			assert.Equal(t, before, canonicalRevision(t, persistence))
			stored, err := repository.Board(t.Context(), board.ID())
			require.NoError(t, err)
			assert.Equal(t, board, stored)
		})
	}
}

func TestRepositoryRejectsWholeBoardSettingsEditBeforePersistence(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	initializeProjectCatalog(t, persistence, new("First board"), Config{})
	repository := New(persistence, Config{})
	board, err := repository.SoleBoard(t.Context())
	require.NoError(t, err)
	before := canonicalRevision(t, persistence)
	name := "Release operations"
	blank := "  "

	_, err = boardpkg.NewService(repository, repository).EditSettings(t.Context(), boardpkg.NewInvocation(""), boardpkg.EditRequest{
		BoardID: board.ID(),
		Settings: boardpkg.SettingsEdit{
			Name: &name, Description: boardpkg.ReplaceDescription(&blank),
		},
	})

	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	assert.Equal(t, before, canonicalRevision(t, persistence))
	stored, readErr := repository.Board(t.Context(), board.ID())
	require.NoError(t, readErr)
	assert.Equal(t, board, stored)
}

func TestRepositoryIdentityFailureDoesNotPublishBoardRevision(t *testing.T) {
	identityErr := errors.New("entropy unavailable")
	persistence, err := store.Open(t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	initializeProjectCatalog(t, persistence, new("First board"), Config{})
	repository := New(persistence, Config{
		IDSource: &sequenceIDs{steps: []idStep{{err: identityErr}}},
	})
	before := canonicalRevision(t, persistence)
	projects, err := repository.ListProjects(t.Context())
	require.NoError(t, err)

	_, err = boardpkg.NewService(repository, repository).Create(t.Context(), boardpkg.NewInvocation(""), boardpkg.CreateRequest{
		ProjectID: projects[0].ID().String(), Name: "Board",
	})

	assert.ErrorIs(t, err, identityErr)
	assert.Equal(t, before, canonicalRevision(t, persistence))
	boards, err := repository.ListAllBoards(t.Context())
	require.NoError(t, err)
	assert.Len(t, boards, 1)
}

func TestRepository_ListAllBoards_stableProjectNameIdentityOrder(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	namespace := initializeProjectCatalog(t, persistence, new("Zulu"), Config{
		Clock: fixedClock{now: time.Unix(10, 0).UTC()},
		IDSource: &sequenceIDs{steps: []idStep{
			{value: "project-a"},
			{value: "board-a-zulu"},
		}},
	})
	insertProject(t, persistence, "project-b", "Second project")
	repository := New(persistence, Config{
		Clock: fixedClock{now: time.Unix(30, 0).UTC()},
		IDSource: &sequenceIDs{steps: []idStep{
			{value: "board-a-alpha-b"},
			{value: "board-a-alpha-a"},
			{value: "board-b-alpha"},
		}},
	})

	_, err = boardpkg.NewService(repository, repository).Create(t.Context(), boardpkg.NewInvocation(""), boardpkg.CreateRequest{
		ProjectID: namespace.Project.ID().String(), Name: "Alpha",
	})
	require.NoError(t, err)
	_, err = boardpkg.NewService(repository, repository).Create(t.Context(), boardpkg.NewInvocation(""), boardpkg.CreateRequest{
		ProjectID: namespace.Project.ID().String(), Name: "Alpha",
	})
	require.NoError(t, err)
	_, err = boardpkg.NewService(repository, repository).Create(t.Context(), boardpkg.NewInvocation(""), boardpkg.CreateRequest{
		ProjectID: mustProjectID(t, "project-b").String(), Name: "Alpha",
	})
	require.NoError(t, err)

	boards, err := repository.ListAllBoards(t.Context())
	require.NoError(t, err)

	require.Len(t, boards, 4)
	assert.Equal(t, []boardpkg.ID{
		mustBoardID(t, "board-a-alpha-a"),
		mustBoardID(t, "board-a-alpha-b"),
		mustBoardID(t, "board-a-zulu"),
		mustBoardID(t, "board-b-alpha"),
	}, []boardpkg.ID{
		boards[0].ID(), boards[1].ID(), boards[2].ID(), boards[3].ID(),
	})
}

func TestRepository_CreateBoard_persistsDescription(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	namespace := initializeProjectCatalog(t, persistence, nil, Config{})
	repository := New(persistence, Config{
		Clock: fixedClock{now: time.Unix(20, 0).UTC()},
		IDSource: &sequenceIDs{steps: []idStep{
			{value: "board-described"},
		}},
	})
	description := "# Operating contract"

	created, err := boardpkg.NewService(repository, repository).Create(t.Context(), boardpkg.NewInvocation("  captain  "), boardpkg.CreateRequest{
		ProjectID:   namespace.Project.ID().String(),
		Name:        "Planning",
		Description: &description,
	})
	require.NoError(t, err)

	assert.Equal(t, &description, created.Description())
	stored, err := repository.Board(t.Context(), created.ID())
	require.NoError(t, err)
	assert.Equal(t, &description, stored.Description())
}

func TestRepository_CreateBoard_blankDescriptionRollsBack(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	namespace := initializeProjectCatalog(t, persistence, nil, Config{})
	repository := New(persistence, Config{
		IDSource: &sequenceIDs{steps: []idStep{{value: "board-invalid"}}},
	})
	before := canonicalRevision(t, persistence)
	blank := "  "

	_, err = boardpkg.NewService(repository, repository).Create(t.Context(), boardpkg.NewInvocation(""), boardpkg.CreateRequest{
		ProjectID: namespace.Project.ID().String(), Name: "Planning", Description: &blank,
	})

	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	assert.Equal(t, before, canonicalRevision(t, persistence))
	boards, readErr := repository.ListAllBoards(t.Context())
	require.NoError(t, readErr)
	assert.Empty(t, boards)
}
