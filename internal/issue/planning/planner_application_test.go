package planning

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	issuekernel "go.abhg.dev/cardamom/internal/issue"
	"go.uber.org/mock/gomock"
)

func TestPlannerResolvesConfigurationForEachOperation(t *testing.T) {
	changes := NewMockChanges(gomock.NewController(t))
	first := configuration.Defaults()
	firstPrefix, err := configuration.NewPrefix("first-")
	require.NoError(t, err)
	first.Issue.ID.Prefix = firstPrefix
	first.Issue.Summary.MaxBytes, err = configuration.NewByteLimit(4)
	require.NoError(t, err)
	second := first
	second.Issue.Summary.MaxBytes, err = configuration.NewByteLimit(2)
	require.NoError(t, err)
	configurations := &changingConfiguration{values: []configuration.Configuration{first, second}}
	boardID, err := board.NewID("board-test")
	require.NoError(t, err)
	changes.EXPECT().CreateIssue(gomock.Any(), first.Issue.ID, gomock.Any()).Return(IssueCreated{}, nil)
	reader := NewMockIssueReader(gomock.NewController(t))
	reader.EXPECT().ReadIssue(gomock.Any(), gomock.Any()).Return(issuekernel.View{}, nil).AnyTimes()
	planner := NewPlanner(changes, reader, &PlannerOptions{
		BoardID: boardID, Configuration: configurations,
	})

	_, err = planner.CreateIssue(
		t.Context(),
		issuekernel.NewInvocation("planner"),
		CreateIssueRequest{Title: "First", Priority: 1, Summary: "four"},
	)
	require.NoError(t, err)

	_, err = planner.CreateIssue(
		t.Context(),
		issuekernel.NewInvocation("planner"),
		CreateIssueRequest{Title: "Second", Priority: 1, Summary: "four"},
	)
	assert.ErrorContains(t, err, "summary is 4 bytes; maximum is 2 bytes")
	assert.Equal(t, 2, configurations.calls)
}

func TestApplyDocumentNormalizesTypedReferencesAndPresence(t *testing.T) {
	t.Parallel()

	changes := NewMockChanges(gomock.NewController(t))
	var applied ApplyDocument
	var appliedMode ApplyMode
	changes.EXPECT().ApplyDocument(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(
			_ context.Context,
			_ configuration.IssueIDConfiguration,
			command ApplyDocument,
			mode ApplyMode,
		) (DocumentApplied, error) {
			applied = command
			appliedMode = mode
			return DocumentApplied{}, nil
		},
	)
	planner := newTestPlanner(t, changes)
	priority := 1
	labels := []string{"backend"}
	dependencies := []ApplyIssueReference{{Kind: ApplyReferenceID, ID: "an-prereq"}}
	_, err := planner.ApplyDocument(t.Context(), issuekernel.NewInvocation("planner"), ApplyDocumentRequest{
		Version: 1, OnExisting: ApplyExistingUpdate, Mode: ApplyModeDryRun,
		Issues: []ApplyIssue{{
			Alias: new("build"), ID: new("an-build"), Key: new("source:1"),
			Title: new("Build"), Type: new("task"), Priority: &priority,
			Labels: &labels, DependsOn: &dependencies,
			Parent: ApplyParentChange{
				Kind:      ParentReplace,
				Reference: ApplyIssueReference{Kind: ApplyReferenceAlias, Alias: "workstream"},
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, applied.issues, 1)
	entry := applied.issues[0]
	assert.Equal(t, IssueAlias("build"), *entry.alias)
	assert.Equal(t, issuekernel.MustID("an-build"), *entry.id)
	assert.Equal(t, ExternalKey("source:1"), *entry.key)
	assert.Equal(t, issuekernel.MustID("an-prereq"), (*entry.dependsOn)[0].id)
	assert.Equal(t, IssueAlias("workstream"), entry.parent.reference.alias)
	assert.Equal(t, ApplyModeDryRun, appliedMode)
}

func TestApplyDocumentReportsUniqueDurableReferences(t *testing.T) {
	t.Parallel()

	changes := NewMockChanges(gomock.NewController(t))
	var applied ApplyDocument
	changes.EXPECT().ApplyDocument(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(
			_ context.Context,
			_ configuration.IssueIDConfiguration,
			command ApplyDocument,
			_ ApplyMode,
		) (DocumentApplied, error) {
			applied = command
			return DocumentApplied{}, nil
		},
	)
	planner := newTestPlanner(t, changes)
	dependencies := []ApplyIssueReference{
		{Kind: ApplyReferenceID, ID: "an-parent"},
		{Kind: ApplyReferenceID, ID: "an-dependency"},
		{Kind: ApplyReferenceAlias, Alias: "local"},
		{Kind: ApplyReferenceKey, Key: "source:local"},
	}
	_, err := planner.ApplyDocument(t.Context(), issuekernel.NewInvocation("planner"), ApplyDocumentRequest{
		Version: 1, OnExisting: ApplyExistingUpdate, Mode: ApplyModeDryRun,
		Issues: []ApplyIssue{
			{
				ID: new("an-target"),
				Parent: ApplyParentChange{
					Kind: ParentReplace,
					Reference: ApplyIssueReference{
						Kind: ApplyReferenceID,
						ID:   "an-parent",
					},
				},
				DependsOn: &dependencies,
			},
			{ID: new("an-target")},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []issuekernel.ID{
		issuekernel.MustID("an-target"),
		issuekernel.MustID("an-parent"),
		issuekernel.MustID("an-dependency"),
	}, applied.ReferencedIssueIDs())
}

func TestPlannerUsesWorkstreamKindAtApplicationBoundary(t *testing.T) {
	t.Parallel()

	changes := NewMockChanges(gomock.NewController(t))
	var created CreateIssue
	changes.EXPECT().CreateIssue(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(
			_ context.Context,
			_ configuration.IssueIDConfiguration,
			command CreateIssue,
		) (IssueCreated, error) {
			created = command
			return IssueCreated{}, nil
		},
	)
	var edited EditIssue
	changes.EXPECT().EditIssue(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, command EditIssue) (IssueEdited, error) {
			edited = command
			return IssueEdited{}, nil
		},
	)
	planner := newTestPlanner(t, changes)

	_, err := planner.CreateIssue(t.Context(), issuekernel.NewInvocation("planner"), CreateIssueRequest{
		Title: "Persistent deliverable", Type: "workstream", Priority: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, issuekernel.KindWorkstream, created.Kind)

	kind := "workstream"
	_, err = planner.EditIssue(t.Context(), issuekernel.NewInvocation("planner"), EditIssueRequest{
		ID: "an-1", Type: &kind,
	})
	require.NoError(t, err)
	require.NotNil(t, edited.Kind)
	assert.Equal(t, issuekernel.KindWorkstream, *edited.Kind)

	for _, removed := range []string{"mission", "epic"} {
		_, err = planner.CreateIssue(t.Context(), issuekernel.NewInvocation("planner"), CreateIssueRequest{
			Title: "Removed kind", Type: removed, Priority: 1,
		})
		assert.ErrorContains(t, err, `invalid type "`+removed+`"`)
	}
}

func TestCreateIssueNormalizesParentAtApplicationBoundary(t *testing.T) {
	t.Parallel()

	changes := NewMockChanges(gomock.NewController(t))
	var created CreateIssue
	changes.EXPECT().CreateIssue(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(
			_ context.Context,
			_ configuration.IssueIDConfiguration,
			command CreateIssue,
		) (IssueCreated, error) {
			created = command
			return IssueCreated{}, nil
		},
	)
	planner := newTestPlanner(t, changes)

	_, err := planner.CreateIssue(t.Context(), issuekernel.NewInvocation("planner"), CreateIssueRequest{
		Title: "Contained task", Type: "task", Priority: 1, Parent: "an-parent",
	})
	require.NoError(t, err)
	require.NotNil(t, created.Parent)
	assert.Equal(t, issuekernel.MustID("an-parent"), *created.Parent)
}

func TestPlannerNormalizesDirectExternalKeys(t *testing.T) {
	t.Parallel()

	changes := NewMockChanges(gomock.NewController(t))
	var created CreateIssue
	changes.EXPECT().CreateIssue(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(
			_ context.Context,
			_ configuration.IssueIDConfiguration,
			command CreateIssue,
		) (IssueCreated, error) {
			created = command
			return IssueCreated{}, nil
		},
	)
	var edited EditIssue
	changes.EXPECT().EditIssue(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, command EditIssue) (IssueEdited, error) {
			edited = command
			return IssueEdited{}, nil
		},
	)
	planner := newTestPlanner(t, changes)

	_, err := planner.CreateIssue(t.Context(), issuekernel.NewInvocation("planner"), CreateIssueRequest{
		Title: "Produced task", Type: "task", Priority: 1, Key: new(" source:task "),
	})
	require.NoError(t, err)
	require.NotNil(t, created.ExternalKey)
	assert.Equal(t, ExternalKey(" source:task "), *created.ExternalKey)

	key := " source:other "
	_, err = planner.EditIssue(t.Context(), issuekernel.NewInvocation("planner"), EditIssueRequest{
		ID: "an-1", Key: &key,
	})
	require.NoError(t, err)
	require.NotNil(t, edited.ExternalKey)
	assert.Equal(t, ExternalKey(" source:other "), *edited.ExternalKey)

	empty := ""
	_, err = planner.CreateIssue(t.Context(), issuekernel.NewInvocation("planner"), CreateIssueRequest{
		Title: "Rejected task", Type: "task", Priority: 1, Key: &empty,
	})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestApplyDocumentPublishesRepositoryReceipt(t *testing.T) {
	t.Parallel()

	changes := NewMockChanges(gomock.NewController(t))
	outcome := DocumentApplied{
		Receipt: ApplyReceipt{
			Entries: []ApplyReceiptEntry{{
				InputIndex: 0, Alias: new("build"), ID: new("an-build"),
				Action: ApplyActionCreate,
			}},
			Counts: ApplyCounts{Create: 1},
		},
		CommittedRevision: CommittedRevision{Revision: 9},
	}
	changes.EXPECT().ApplyDocument(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(outcome, nil)
	planner := newTestPlanner(t, changes)

	result, err := planner.ApplyDocument(t.Context(), issuekernel.NewInvocation("planner"), ApplyDocumentRequest{
		Version: 1, Mode: ApplyModeCommit,
		Issues: []ApplyIssue{{Alias: new("build"), Title: new("Build"), Type: new("task")}},
	})
	require.NoError(t, err)
	require.NotNil(t, result.Revision)
	assert.Equal(t, int64(9), *result.Revision)
	assert.Equal(t, ApplyCounts{Create: 1}, result.Counts)
}

func TestEditIssueReplacesLabelsInOneEditCommand(t *testing.T) {
	t.Parallel()

	changes := NewMockChanges(gomock.NewController(t))
	var edited EditIssue
	changes.EXPECT().EditIssue(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, command EditIssue) (IssueEdited, error) {
			edited = command
			return IssueEdited{}, nil
		},
	)
	planner := newTestPlanner(t, changes)
	labels := []string{"docs", "urgent"}

	_, err := planner.EditIssue(t.Context(), issuekernel.NewInvocation("operator"), EditIssueRequest{
		ID: "an-1", Labels: &labels,
	})
	require.NoError(t, err)
	require.NotNil(t, edited.ReplaceLabels)
	assert.Equal(t, []issuekernel.Label{issuekernel.MustLabel("docs"), issuekernel.MustLabel("urgent")}, *edited.ReplaceLabels)
}

func TestEditIssuePublishesCompletePostCommitView(t *testing.T) {
	t.Parallel()

	issue := applicationTestIssue(t, "an-1", issuekernel.StatusReady)
	changes := NewMockChanges(gomock.NewController(t))
	editOutcome := IssueEdited{
		Issue:             issue,
		CommittedRevision: CommittedRevision{Revision: 9},
	}
	changes.EXPECT().EditIssue(gomock.Any(), gomock.Any()).Return(editOutcome, nil)
	view := issuekernel.View{Detail: issuekernel.Detail{
		Issue:  issuekernel.Issue{ID: "an-1", Revision: 9},
		Labels: []string{"docs"}, Blocks: []issuekernel.Reference{{ID: "an-2"}},
	}}
	readers := NewMockIssueReader(gomock.NewController(t))
	readers.EXPECT().ReadIssue(gomock.Any(), issuekernel.ReadRequest{IssueID: "an-1"}).Return(view, nil)
	planner := newTestPlannerWithReaders(t, changes, readers)

	result, err := planner.EditIssue(t.Context(), issuekernel.NewInvocation("operator"), EditIssueRequest{
		ID: "an-1", Title: new("Renamed"),
	})
	require.NoError(t, err)
	assert.Equal(t, view.Detail, result.Issue)
}

type changingConfiguration struct {
	values []configuration.Configuration
	calls  int
}

func (c *changingConfiguration) ResolveConfiguration(
	context.Context,
	board.ID,
) (configuration.Configuration, error) {
	value := c.values[c.calls]
	c.calls++
	return value, nil
}

func newTestPlanner(t *testing.T, changes Changes) *Planner {
	t.Helper()
	reader := NewMockIssueReader(gomock.NewController(t))
	reader.EXPECT().ReadIssue(gomock.Any(), gomock.Any()).Return(issuekernel.View{}, nil).AnyTimes()
	return newTestPlannerWithReaders(t, changes, reader)
}

func newTestPlannerWithReaders(
	t *testing.T,
	changes Changes,
	readers IssueReader,
) *Planner {
	t.Helper()
	return NewPlanner(changes, readers, nil)
}

func applicationTestIssue(t *testing.T, id string, status issuekernel.Status) issuekernel.State {
	t.Helper()
	lifecycle := status.Lifecycle()
	var activeClaim *issuekernel.ClaimState
	if status == issuekernel.StatusInProgress {
		lifecycle = issuekernel.LifecycleOpen
		activeClaim = &issuekernel.ClaimState{
			Actor:     issuekernel.NewActor("worker"),
			StartedAt: time.Unix(2, 0),
		}
	}
	state, err := issuekernel.Load(issuekernel.Snapshot{
		ID:          issuekernel.MustID(id),
		Title:       id,
		Kind:        issuekernel.KindTask,
		Lifecycle:   lifecycle,
		Priority:    issuekernel.PriorityNormal,
		ActiveClaim: activeClaim,
		Created:     time.Unix(1, 0),
		Updated:     time.Unix(2, 0),
	})
	require.NoError(t, err)
	return state
}
