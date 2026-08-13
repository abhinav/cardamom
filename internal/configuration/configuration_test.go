package configuration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/project"
	"go.uber.org/mock/gomock"
)

func TestService_Resolve_appliesLivePerFieldPrecedence(t *testing.T) {
	prefix := mustPrefix(t, "store-")
	storeSummary := mustByteLimit(t, 3072)
	storePins := mustCountLimit(t, 6)
	projectStrategy := IDStrategySequential
	projectSummary := mustByteLimit(t, 4096)
	boardPrefix := mustPrefix(t, "board-")
	boardAttachment := mustByteLimit(t, 512)
	boardPins := mustCountLimit(t, 3)
	storeOverrides := Overrides{
		Issue: IssueOverrides{
			ID:      IssueIDOverrides{Prefix: &prefix},
			Summary: SummaryOverrides{MaxBytes: &storeSummary},
		},
		Board: BoardOverrides{Pins: PinOverrides{MaxCount: &storePins}},
	}
	layers := DatabaseLayers{
		ProjectID: mustProjectID(t, "project-test"),
		Project: Overrides{Issue: IssueOverrides{
			ID:      IssueIDOverrides{Strategy: &projectStrategy},
			Summary: SummaryOverrides{MaxBytes: &projectSummary},
		}},
		Board: Overrides{
			Issue:      IssueOverrides{ID: IssueIDOverrides{Prefix: &boardPrefix}},
			Attachment: AttachmentOverrides{MaxBytes: &boardAttachment},
			Board:      BoardOverrides{Pins: PinOverrides{MaxCount: &boardPins}},
		},
	}
	store := NewMockStore(gomock.NewController(t))
	store.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(storeOverrides, nil)
	repository := NewMockRepository(gomock.NewController(t))
	repository.EXPECT().ReadConfigurationLayers(gomock.Any(), gomock.Any()).Return(layers, nil)
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
		Board:      BoardConfiguration{Pins: PinConfiguration{MaxCount: boardPins}},
	}, resolved.Effective)
	assert.Equal(t, ScopeBoard, resolved.Origins.Issue.ID.Prefix.Scope)
	assert.Equal(t, ScopeProject, resolved.Origins.Issue.ID.Strategy.Scope)
	assert.Equal(t, ScopeProject, resolved.Origins.Issue.Summary.MaxBytes.Scope)
	assert.Equal(t, ScopeBoard, resolved.Origins.Attachment.MaxBytes.Scope)
	assert.Equal(t, ScopeBoard, resolved.Origins.Board.Pins.MaxCount.Scope)
	assert.Equal(t, "board-test", resolved.Origins.Issue.ID.Prefix.Identity)
	assert.Equal(t, "project-test", resolved.Origins.Issue.ID.Strategy.Identity)

	secondPrefix := mustPrefix(t, "live-")
	storeOverrides.Issue.ID.Prefix = &secondPrefix
	layers.Board.Issue.ID.Prefix = nil
	store.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(storeOverrides, nil)
	repository.EXPECT().ReadConfigurationLayers(gomock.Any(), boardID).Return(layers, nil)
	resolved, err = service.Resolve(t.Context(), boardID)
	require.NoError(t, err)
	assert.Equal(t, secondPrefix, resolved.Effective.Issue.ID.Prefix)
	assert.Equal(t, ScopeStore, resolved.Origins.Issue.ID.Prefix.Scope)
}

func TestService_ResolveProject_stopsAtProjectLayer(t *testing.T) {
	storePrefix := mustPrefix(t, "store-")
	projectPrefix := mustPrefix(t, "project-")
	projectStrategy := IDStrategySequential
	storeOverrides := Overrides{Issue: IssueOverrides{
		ID: IssueIDOverrides{Prefix: &storePrefix},
	}}
	projectOverrides := Overrides{Issue: IssueOverrides{ID: IssueIDOverrides{
		Prefix: &projectPrefix, Strategy: &projectStrategy,
	}}}
	store := NewMockStore(gomock.NewController(t))
	store.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(storeOverrides, nil)
	repository := NewMockRepository(gomock.NewController(t))
	projectID := mustProjectID(t, "project-test")
	repository.EXPECT().ReadProjectConfiguration(gomock.Any(), projectID).Return(projectOverrides, nil)
	service := NewService(store, repository)
	service.SetStoreIdentity("/stores/test")

	resolved, err := service.ResolveProject(t.Context(), projectID)
	require.NoError(t, err)

	assert.Equal(t, projectPrefix, resolved.Effective.Issue.ID.Prefix)
	assert.Equal(t, IDStrategySequential, resolved.Effective.Issue.ID.Strategy)
	assert.Equal(t, ScopeProject, resolved.Origins.Issue.ID.Prefix.Scope)
	assert.Equal(t, ScopeProject, resolved.Origins.Issue.ID.Strategy.Scope)
	assert.Equal(t, "/stores/test", resolved.Store.Source.Identity)
	assert.Equal(t, "project-test", resolved.Project.Source.Identity)
}

func TestService_UpdateProject_returnsProjectScopedView(t *testing.T) {
	projectID := mustProjectID(t, "project-test")
	projectPrefix := mustPrefix(t, "project-")
	strategy := IDStrategySequential
	patch := Patch{
		Fields: []Field{FieldIssueIDStrategy},
		Overrides: Overrides{Issue: IssueOverrides{
			ID: IssueIDOverrides{Strategy: &strategy},
		}},
	}
	store := NewMockStore(gomock.NewController(t))
	store.EXPECT().ReadStoreConfiguration(gomock.Any()).Return(Overrides{}, nil)
	repository := NewMockRepository(gomock.NewController(t))
	repository.EXPECT().UpdateProjectConfiguration(
		gomock.Any(), projectID, patch,
	).Return(Overrides{}, nil)
	repository.EXPECT().ReadProjectConfiguration(
		gomock.Any(), projectID,
	).Return(Overrides{Issue: IssueOverrides{ID: IssueIDOverrides{
		Prefix: &projectPrefix, Strategy: &strategy,
	}}}, nil)
	service := NewService(store, repository)
	service.SetStoreIdentity("/stores/test")

	resolved, err := service.UpdateProject(
		t.Context(),
		NewInvocation("captain"),
		ProjectUpdateRequest{ProjectID: projectID, Patch: patch},
	)
	require.NoError(t, err)

	assert.Equal(t, projectPrefix, resolved.Effective.Issue.ID.Prefix)
	assert.Equal(t, IDStrategySequential, resolved.Effective.Issue.ID.Strategy)
	assert.Equal(t, ScopeProject, resolved.Origins.Issue.ID.Strategy.Scope)
}

func TestDefaults(t *testing.T) {
	assert.Equal(t, "cm-", Defaults().Issue.ID.Prefix.String())
	assert.Equal(t, IDStrategyRandom, Defaults().Issue.ID.Strategy)
	assert.Equal(t, uint64(2048), Defaults().Issue.Summary.MaxBytes.Uint64())
	assert.Equal(t, uint64(104857600), Defaults().Attachment.MaxBytes.Uint64())
	assert.Equal(t, uint64(8), Defaults().Board.Pins.MaxCount.Uint64())
}

func TestNewPrefixCannotGenerateInvalidIssueIDs(t *testing.T) {
	t.Parallel()

	_, err := NewPrefix("-")
	assert.ErrorContains(t, err, "start with a letter or digit")
}

func TestInferredPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give string
		want string
	}{
		{name: "Lowercase", give: "Cardamom", want: "cardamom-"},
		{name: "Separators", give: "mission__control", want: "mission-control-"},
		{
			name: "LengthLimit",
			give: "1234__An Extremely Long Project",
			want: "1234-an-extreme-",
		},
		{name: "Fallback", give: "---___", want: "cm-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, InferredPrefix(tt.give).String())
		})
	}
}

func TestSelectInitializationPrefix(t *testing.T) {
	t.Parallel()

	storePrefix := mustPrefix(t, "store-")
	explicit := "explicit-"

	inferred, err := SelectInitializationPrefix("Project Name", nil, Overrides{})
	require.NoError(t, err)
	require.NotNil(t, inferred.FreshProject)
	assert.Equal(t, "project-name-", inferred.FreshProject.String())
	assert.Nil(t, inferred.RetainedProject)

	inherited, err := SelectInitializationPrefix("Project Name", nil, Overrides{
		Issue: IssueOverrides{ID: IssueIDOverrides{Prefix: &storePrefix}},
	})
	require.NoError(t, err)
	assert.Nil(t, inherited.FreshProject)
	assert.Nil(t, inherited.RetainedProject)

	overridden, err := SelectInitializationPrefix("Project Name", &explicit, Overrides{
		Issue: IssueOverrides{ID: IssueIDOverrides{Prefix: &storePrefix}},
	})
	require.NoError(t, err)
	require.NotNil(t, overridden.FreshProject)
	require.NotNil(t, overridden.RetainedProject)
	assert.Equal(t, explicit, overridden.FreshProject.String())
	assert.Equal(t, explicit, overridden.RetainedProject.String())
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

func mustCountLimit(t *testing.T, value uint64) board.PinLimit {
	t.Helper()
	limit, err := board.NewPinLimit(value)
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
