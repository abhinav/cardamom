package project

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestRepositoryPersistsProjectAndBoardConfiguration(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{
		Path: t.TempDir() + "/board.sqlite3",
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	namespace := initializeProjectCatalog(t, persistence, new("Board"), Config{})
	require.NotNil(t, namespace.Board)
	repository := New(persistence, Config{
		Clock: fixedClock{now: time.Unix(30, 0).UTC()},
	})
	service := configuration.NewService(emptyConfigurationStore{}, repository)
	projectPrefix, err := configuration.NewPrefix("project-")
	require.NoError(t, err)
	boardMaximum, err := configuration.NewByteLimit(4096)
	require.NoError(t, err)

	_, err = service.Update(
		t.Context(),
		configuration.NewInvocation(" captain "),
		configuration.UpdateRequest{
			BoardID: namespace.Board.ID(), Scope: configuration.ScopeProject,
			Patch: configuration.Patch{
				Fields: []configuration.Field{configuration.FieldIssueIDPrefix},
				Overrides: configuration.Overrides{
					Issue: configuration.IssueOverrides{
						ID: configuration.IssueIDOverrides{Prefix: &projectPrefix},
					},
				},
			},
		},
	)
	require.NoError(t, err)
	_, err = service.Update(
		t.Context(),
		configuration.NewInvocation("captain"),
		configuration.UpdateRequest{
			BoardID: namespace.Board.ID(), Scope: configuration.ScopeProject,
			Patch: configuration.Patch{
				Fields: []configuration.Field{configuration.FieldIssueIDPrefix},
				Overrides: configuration.Overrides{
					Issue: configuration.IssueOverrides{
						ID: configuration.IssueIDOverrides{Prefix: &projectPrefix},
					},
				},
			},
		},
	)
	require.NoError(t, err)
	_, err = service.Update(
		t.Context(),
		configuration.NewInvocation("captain"),
		configuration.UpdateRequest{
			BoardID: namespace.Board.ID(), Scope: configuration.ScopeBoard,
			Patch: configuration.Patch{
				Fields: []configuration.Field{configuration.FieldAttachmentMaxBytes},
				Overrides: configuration.Overrides{
					Attachment: configuration.AttachmentOverrides{MaxBytes: &boardMaximum},
				},
			},
		},
	)
	require.NoError(t, err)

	layers, err := repository.ReadConfigurationLayers(t.Context(), namespace.Board.ID())
	require.NoError(t, err)
	assert.Equal(t, namespace.Project.ID(), layers.ProjectID)
	assert.Equal(t, projectPrefix, *layers.Project.Issue.ID.Prefix)
	assert.Nil(t, layers.Project.Attachment.MaxBytes)
	assert.Nil(t, layers.Board.Issue.ID.Prefix)
	assert.Equal(t, boardMaximum, *layers.Board.Attachment.MaxBytes)
	assert.Equal(t, int64(3), canonicalRevision(t, persistence))
}

func TestRepositoryReadsProjectConfigurationWithoutBoard(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{
		Path: t.TempDir() + "/board.sqlite3",
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	namespace := initializeProjectCatalog(t, persistence, nil, Config{})
	repository := New(persistence, Config{})
	prefix, err := configuration.NewPrefix("project-")
	require.NoError(t, err)
	_, err = repository.UpdateProjectConfiguration(
		t.Context(),
		namespace.Project.ID(),
		configuration.Patch{
			Fields: []configuration.Field{configuration.FieldIssueIDPrefix},
			Overrides: configuration.Overrides{Issue: configuration.IssueOverrides{
				ID: configuration.IssueIDOverrides{Prefix: &prefix},
			}},
		},
	)
	require.NoError(t, err)

	overrides, err := repository.ReadProjectConfiguration(
		t.Context(), namespace.Project.ID(),
	)
	require.NoError(t, err)

	require.NotNil(t, overrides.Issue.ID.Prefix)
	assert.Equal(t, prefix, *overrides.Issue.ID.Prefix)
}

type emptyConfigurationStore struct{}

func (emptyConfigurationStore) ReadStoreConfiguration(
	context.Context,
) (configuration.Overrides, error) {
	return configuration.Overrides{}, nil
}

func (emptyConfigurationStore) UpdateStoreConfiguration(
	context.Context,
	configuration.Patch,
) (configuration.Overrides, error) {
	return configuration.Overrides{}, nil
}
