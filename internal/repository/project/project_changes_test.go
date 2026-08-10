package project

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestRepositoryEditProjectChangesOnlyNameAtOneRevision(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{
		Path: t.TempDir() + "/board.sqlite3",
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	boardName := "Project board"
	namespace := initializeProjectCatalog(t, persistence, &boardName, Config{})
	insertProject(t, persistence, "project-duplicate", "Renamed")
	repository := New(persistence, Config{})
	before := canonicalRevision(t, persistence)
	configurationBefore := projectConfiguration(t, persistence, namespace.Project.ID())

	edited, err := repository.EditProjectName(t.Context(), project.EditNameRequest{
		ProjectID: namespace.Project.ID(),
		Name:      "  Renamed  ",
	})
	require.NoError(t, err)

	assert.Equal(t, namespace.Project.ID(), edited.ID())
	assert.Equal(t, "Renamed", edited.Name())
	assert.Equal(t, namespace.Project.Created().Unix(), edited.Created().Unix())
	assert.Equal(t, before+1, canonicalRevision(t, persistence))
	assert.Equal(t, configurationBefore, projectConfiguration(t, persistence, edited.ID()))

	boards, err := repository.ListAllBoards(t.Context())
	require.NoError(t, err)
	require.Len(t, boards, 1)
	assert.Equal(t, edited.ID().String(), boards[0].ProjectID())

	projects, err := repository.ListProjects(t.Context())
	require.NoError(t, err)
	require.Len(t, projects, 2)
	assert.Equal(t, []string{"Renamed", "Renamed"}, []string{
		projects[0].Name(), projects[1].Name(),
	})
}

func TestRepositoryEditProjectNoOpDoesNotAdvanceRevision(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{
		Path: t.TempDir() + "/board.sqlite3",
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	namespace := initializeProjectCatalog(t, persistence, nil, Config{})
	repository := New(persistence, Config{})
	before := canonicalRevision(t, persistence)

	edited, err := repository.EditProjectName(t.Context(), project.EditNameRequest{
		ProjectID: namespace.Project.ID(),
		Name:      namespace.Project.Name(),
	})
	require.NoError(t, err)

	assert.Equal(t, namespace.Project.Name(), edited.Name())
	assert.Equal(t, before, canonicalRevision(t, persistence))
}

func TestRepositoryEditProjectRejectsBlankNameWithoutChangingState(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{
		Path: t.TempDir() + "/board.sqlite3",
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	namespace := initializeProjectCatalog(t, persistence, nil, Config{})
	repository := New(persistence, Config{})
	before := canonicalRevision(t, persistence)

	_, err = repository.EditProjectName(t.Context(), project.EditNameRequest{
		ProjectID: namespace.Project.ID(),
		Name:      " ",
	})

	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	assert.Equal(t, before, canonicalRevision(t, persistence))
	projects, listErr := repository.ListProjects(t.Context())
	require.NoError(t, listErr)
	require.Len(t, projects, 1)
	assert.Equal(t, namespace.Project.Name(), projects[0].Name())
}

type storedProjectConfiguration struct {
	prefix   sql.NullString
	strategy sql.NullString
}

func projectConfiguration(
	t *testing.T,
	persistence *store.Store,
	projectID project.ID,
) storedProjectConfiguration {
	t.Helper()
	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	var configuration storedProjectConfiguration
	err = view.QueryRowContext(t.Context(), `
		SELECT issue_id_prefix, issue_id_strategy
		FROM projects
		WHERE id = ?
	`, projectID.String()).Scan(&configuration.prefix, &configuration.strategy)
	require.NoError(t, err)
	return configuration
}
