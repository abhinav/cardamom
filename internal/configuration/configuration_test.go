package configuration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/project"
)

func TestService_Resolve_appliesLivePerFieldPrecedence(t *testing.T) {
	prefix := mustPrefix(t, "store-")
	storeSummary := mustByteLimit(t, 3072)
	projectStrategy := IDStrategySequential
	projectSummary := mustByteLimit(t, 4096)
	boardPrefix := mustPrefix(t, "board-")
	boardAttachment := mustByteLimit(t, 512)
	store := &recordingStore{overrides: Overrides{
		Issue: IssueOverrides{
			ID:      IssueIDOverrides{Prefix: &prefix},
			Summary: SummaryOverrides{MaxBytes: &storeSummary},
		},
	}}
	repository := &recordingRepository{layers: DatabaseLayers{
		ProjectID: mustProjectID(t, "project-test"),
		Project: Overrides{Issue: IssueOverrides{
			ID:      IssueIDOverrides{Strategy: &projectStrategy},
			Summary: SummaryOverrides{MaxBytes: &projectSummary},
		}},
		Board: Overrides{
			Issue:      IssueOverrides{ID: IssueIDOverrides{Prefix: &boardPrefix}},
			Attachment: AttachmentOverrides{MaxBytes: &boardAttachment},
		},
	}}
	service := NewService(store, repository)
	boardID := mustBoardID(t, "board-test")

	resolved, err := service.Resolve(t.Context(), boardID)
	require.NoError(t, err)
	assert.Equal(t, Configuration{
		Issue: IssueConfiguration{
			ID: IssueIDConfiguration{
				Prefix: boardPrefix, Strategy: IDStrategySequential,
			},
			Summary: SummaryConfiguration{MaxBytes: projectSummary},
		},
		Attachment: AttachmentConfiguration{MaxBytes: boardAttachment},
	}, resolved.Effective)
	assert.Equal(t, ScopeBoard, resolved.Origins.Issue.ID.Prefix.Scope)
	assert.Equal(t, ScopeProject, resolved.Origins.Issue.ID.Strategy.Scope)
	assert.Equal(t, ScopeProject, resolved.Origins.Issue.Summary.MaxBytes.Scope)
	assert.Equal(t, ScopeBoard, resolved.Origins.Attachment.MaxBytes.Scope)
	assert.Equal(t, "board-test", resolved.Origins.Issue.ID.Prefix.Identity)
	assert.Equal(t, "project-test", resolved.Origins.Issue.ID.Strategy.Identity)

	secondPrefix := mustPrefix(t, "live-")
	store.overrides.Issue.ID.Prefix = &secondPrefix
	repository.layers.Board.Issue.ID.Prefix = nil
	resolved, err = service.Resolve(t.Context(), boardID)
	require.NoError(t, err)
	assert.Equal(t, secondPrefix, resolved.Effective.Issue.ID.Prefix)
	assert.Equal(t, ScopeStore, resolved.Origins.Issue.ID.Prefix.Scope)
	assert.Equal(t, 2, store.reads)
	assert.Equal(t, 2, repository.reads)
}

func TestDefaults(t *testing.T) {
	assert.Equal(t, "cm-", Defaults().Issue.ID.Prefix.String())
	assert.Equal(t, IDStrategyRandom, Defaults().Issue.ID.Strategy)
	assert.Equal(t, uint64(2048), Defaults().Issue.Summary.MaxBytes.Uint64())
	assert.Equal(t, uint64(104857600), Defaults().Attachment.MaxBytes.Uint64())
}

func TestNewPrefixCannotGenerateInvalidIssueIDs(t *testing.T) {
	t.Parallel()

	_, err := NewPrefix("-")
	assert.ErrorContains(t, err, "start with a letter or digit")
}

type recordingStore struct {
	overrides Overrides
	reads     int
}

func (s *recordingStore) ReadStoreConfiguration(context.Context) (Overrides, error) {
	s.reads++
	return s.overrides, nil
}

func (s *recordingStore) UpdateStoreConfiguration(
	context.Context,
	Patch,
) (Overrides, error) {
	return s.overrides, nil
}

type recordingRepository struct {
	layers DatabaseLayers
	reads  int
}

func (r *recordingRepository) ReadConfigurationLayers(
	context.Context,
	board.ID,
) (DatabaseLayers, error) {
	r.reads++
	return r.layers, nil
}

func (r *recordingRepository) UpdateProjectConfiguration(
	context.Context,
	project.ID,
	Patch,
) (Overrides, error) {
	return r.layers.Project, nil
}

func (r *recordingRepository) UpdateBoardConfiguration(
	context.Context,
	board.ID,
	Patch,
) (Overrides, error) {
	return r.layers.Board, nil
}

func mustPrefix(t *testing.T, value string) Prefix {
	t.Helper()
	prefix, err := NewPrefix(value)
	require.NoError(t, err)
	return prefix
}

func mustByteLimit(t *testing.T, value uint64) ByteLimit {
	t.Helper()
	limit, err := NewByteLimit(value)
	require.NoError(t, err)
	return limit
}

func mustProjectID(t *testing.T, value string) project.ID {
	t.Helper()
	id, err := project.NewID(value)
	require.NoError(t, err)
	return id
}

func mustBoardID(t *testing.T, value string) board.ID {
	t.Helper()
	id, err := board.NewID(value)
	require.NoError(t, err)
	return id
}
