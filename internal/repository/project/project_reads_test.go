package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestRepositoryListsProjectsByNameAndIdentity(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{
		Path: t.TempDir() + "/board.sqlite3",
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	insertProject(t, persistence, "project-zulu", "Zulu")
	insertProject(t, persistence, "project-alpha-b", "Alpha")
	insertProject(t, persistence, "project-alpha-a", "Alpha")

	projects, err := New(persistence, Config{}).ListProjects(t.Context())
	require.NoError(t, err)

	require.Len(t, projects, 3)
	assert.Equal(t, "project-alpha-a", projects[0].ID().String())
	assert.Equal(t, "project-alpha-b", projects[1].ID().String())
	assert.Equal(t, "project-zulu", projects[2].ID().String())
}
