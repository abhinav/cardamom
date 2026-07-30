package project

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/configuration"
	projectcreation "go.abhg.dev/cardamom/internal/project/creation"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestRepository_CreateProject_commitsOneRevisionWithoutBoard(t *testing.T) {
	persistence, err := store.Open(
		t.Context(),
		store.Config{Path: t.TempDir() + "/board.sqlite3"},
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	initializeProjectCatalog(t, persistence, nil, Config{})
	repository := New(persistence, Config{
		Clock: fixedClock{now: time.Unix(20, 0).UTC()},
		IDSource: &sequenceIDs{steps: []idStep{
			{value: "project-two"},
		}},
	})
	prefix, err := configuration.NewPrefix("second-")
	require.NoError(t, err)
	before := canonicalRevision(t, persistence)

	created, err := repository.CreateProject(
		t.Context(),
		projectcreation.Creation{Name: "Second", Prefix: &prefix},
	)
	require.NoError(t, err)

	assert.Equal(t, mustProjectID(t, "project-two"), created.ID())
	assert.Equal(t, "Second", created.Name())
	assert.Equal(t, time.Unix(20, 0).UTC(), created.Created())
	assert.Equal(t, before+1, canonicalRevision(t, persistence))

	projects, err := repository.ListProjects(t.Context())
	require.NoError(t, err)
	assert.Len(t, projects, 2)
	boards, err := repository.ListAllBoards(t.Context())
	require.NoError(t, err)
	assert.Empty(t, boards)

	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	var storedPrefix sql.NullString
	err = view.QueryRowContext(
		t.Context(),
		`SELECT issue_id_prefix FROM projects WHERE id = ?`,
		created.ID().String(),
	).Scan(&storedPrefix)
	require.NoError(t, err)
	assert.Equal(t, sql.NullString{String: "second-", Valid: true}, storedPrefix)
	require.NoError(t, view.Done())
}

func TestRepository_CreateProject_allowsDuplicateNames(t *testing.T) {
	persistence, err := store.Open(
		t.Context(),
		store.Config{Path: t.TempDir() + "/board.sqlite3"},
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	initializeProjectCatalog(t, persistence, nil, Config{})
	repository := New(persistence, Config{
		IDSource: &sequenceIDs{steps: []idStep{
			{value: "project-two"},
			{value: "project-three"},
		}},
	})
	before := canonicalRevision(t, persistence)

	second, err := repository.CreateProject(
		t.Context(),
		projectcreation.Creation{Name: "Project"},
	)
	require.NoError(t, err)
	third, err := repository.CreateProject(
		t.Context(),
		projectcreation.Creation{Name: "Project"},
	)
	require.NoError(t, err)

	assert.NotEqual(t, second.ID(), third.ID())
	assert.Equal(t, before+2, canonicalRevision(t, persistence))
	projects, err := repository.ListProjects(t.Context())
	require.NoError(t, err)
	require.Len(t, projects, 3)
	assert.Equal(t, []string{"Project", "Project", "Project"}, []string{
		projects[0].Name(),
		projects[1].Name(),
		projects[2].Name(),
	})
}
