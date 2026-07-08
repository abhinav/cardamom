package project

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	boardpkg "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/repository/store"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type idStep struct {
	value string
	err   error
}

type sequenceIDs struct{ steps []idStep }

func (s *sequenceIDs) NewID(string) (string, error) {
	step := s.steps[0]
	s.steps = s.steps[1:]
	return step.value, step.err
}

func canonicalRevision(t *testing.T, persistence *store.Store) int64 {
	t.Helper()
	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	revision, err := view.CanonicalRevision(t.Context())
	require.NoError(t, err)
	require.NoError(t, view.Done())
	return revision
}

func initializeProjectCatalog(
	t *testing.T,
	persistence *store.Store,
	boardName *string,
	config Config,
) project.InitializedNamespace {
	t.Helper()
	namespace, err := initializeFreshProject(
		t.Context(),
		persistence,
		project.StoreInitializationRequest{
			ProjectName: "Project",
			BoardName:   boardName,
		},
		config,
	)
	require.NoError(t, err)
	return namespace
}

func insertProject(
	t *testing.T,
	persistence *store.Store,
	projectID string,
	name string,
) {
	t.Helper()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO projects (id, name, created_at) VALUES (?, ?, 20)
	`, projectID, name)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
}

func mustProjectID(t *testing.T, value string) project.ID {
	t.Helper()
	id, err := project.NewID(value)
	require.NoError(t, err)
	return id
}

func mustBoardID(t *testing.T, value string) boardpkg.ID {
	t.Helper()
	id, err := boardpkg.NewID(value)
	require.NoError(t, err)
	return id
}
