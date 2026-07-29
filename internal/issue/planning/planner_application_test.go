package planning

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	issuekernel "go.abhg.dev/cardamom/internal/issue"
)

func TestPlannerResolvesConfigurationForEachOperation(t *testing.T) {
	changes := &fakeChanges{}
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
	planner := NewPlanner(changes, &fakeReaders{}, &PlannerOptions{
		BoardID: boardID, Configuration: configurations,
	})

	_, err = planner.CreateIssue(
		t.Context(),
		issuekernel.NewInvocation("planner"),
		CreateIssueRequest{Title: "First", Priority: 1, Summary: "four"},
	)
	require.NoError(t, err)
	assert.Equal(t, first.Issue.ID, changes.issueIDConfiguration)

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

	changes := &fakeChanges{}
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
	require.Len(t, changes.applyDocument.issues, 1)
	entry := changes.applyDocument.issues[0]
	assert.Equal(t, IssueAlias("build"), *entry.alias)
	assert.Equal(t, issuekernel.MustID("an-build"), *entry.id)
	assert.Equal(t, ExternalKey("source:1"), *entry.key)
	assert.Equal(t, issuekernel.MustID("an-prereq"), (*entry.dependsOn)[0].id)
	assert.Equal(t, IssueAlias("workstream"), entry.parent.reference.alias)
	assert.Equal(t, ApplyModeDryRun, changes.applyMode)
}

func TestApplyDocumentReportsUniqueDurableReferences(t *testing.T) {
	t.Parallel()

	changes := &fakeChanges{}
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
	}, changes.applyDocument.ReferencedIssueIDs())
}

func TestPlannerUsesWorkstreamKindAtApplicationBoundary(t *testing.T) {
	t.Parallel()

	changes := &fakeChanges{}
	planner := newTestPlanner(t, changes)

	_, err := planner.CreateIssue(t.Context(), issuekernel.NewInvocation("planner"), CreateIssueRequest{
		Title: "Persistent deliverable", Type: "workstream", Priority: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, issuekernel.KindWorkstream, changes.createIssue.Kind)

	kind := "workstream"
	_, err = planner.EditIssue(t.Context(), issuekernel.NewInvocation("planner"), EditIssueRequest{
		ID: "an-1", Type: &kind,
	})
	require.NoError(t, err)
	require.NotNil(t, changes.editIssue.Kind)
	assert.Equal(t, issuekernel.KindWorkstream, *changes.editIssue.Kind)

	for _, removed := range []string{"mission", "epic"} {
		_, err = planner.CreateIssue(t.Context(), issuekernel.NewInvocation("planner"), CreateIssueRequest{
			Title: "Removed kind", Type: removed, Priority: 1,
		})
		assert.ErrorContains(t, err, `invalid type "`+removed+`"`)
	}
}

func TestCreateIssueNormalizesParentAtApplicationBoundary(t *testing.T) {
	t.Parallel()

	changes := &fakeChanges{}
	planner := newTestPlanner(t, changes)

	_, err := planner.CreateIssue(t.Context(), issuekernel.NewInvocation("planner"), CreateIssueRequest{
		Title: "Contained task", Type: "task", Priority: 1, Parent: "an-parent",
	})
	require.NoError(t, err)
	require.NotNil(t, changes.createIssue.Parent)
	assert.Equal(t, issuekernel.MustID("an-parent"), *changes.createIssue.Parent)
}

func TestApplyDocumentPublishesRepositoryReceipt(t *testing.T) {
	t.Parallel()

	changes := &fakeChanges{applyOutcome: DocumentApplied{
		Receipt: ApplyReceipt{
			Entries: []ApplyReceiptEntry{{
				InputIndex: 0, Alias: new("build"), ID: new("an-build"),
				Action: ApplyActionCreate,
			}},
			Counts: ApplyCounts{Create: 1},
		},
		CommittedRevision: CommittedRevision{Revision: 9},
	}}
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

	changes := &fakeChanges{}
	planner := newTestPlanner(t, changes)
	labels := []string{"docs", "urgent"}

	_, err := planner.EditIssue(t.Context(), issuekernel.NewInvocation("operator"), EditIssueRequest{
		ID: "an-1", Labels: &labels,
	})
	require.NoError(t, err)
	require.NotNil(t, changes.editIssue.ReplaceLabels)
	assert.Equal(t, []issuekernel.Label{issuekernel.MustLabel("docs"), issuekernel.MustLabel("urgent")}, *changes.editIssue.ReplaceLabels)
}

func TestEditIssuePublishesCompletePostCommitView(t *testing.T) {
	t.Parallel()

	issue := applicationTestIssue(t, "an-1", issuekernel.StatusReady)
	changes := &fakeChanges{editOutcome: IssueEdited{
		Issue:             issue,
		CommittedRevision: CommittedRevision{Revision: 9},
	}}
	readers := &fakeReaders{issue: issuekernel.View{Detail: issuekernel.Detail{
		Issue:  issuekernel.Issue{ID: "an-1", Revision: 9},
		Labels: []string{"docs"}, Blocks: []issuekernel.Reference{{ID: "an-2"}},
	}}}
	planner := newTestPlannerWithReaders(t, changes, readers)

	result, err := planner.EditIssue(t.Context(), issuekernel.NewInvocation("operator"), EditIssueRequest{
		ID: "an-1", Title: new("Renamed"),
	})
	require.NoError(t, err)
	assert.Equal(t, readers.issue.Detail, result.Issue)
	assert.Equal(t, issuekernel.ReadRequest{IssueID: "an-1"}, readers.readIssueRequest)
	assert.Equal(t, 1, readers.readIssueCalls)
}

type fakeReaders struct {
	issue            issuekernel.View
	readIssueRequest issuekernel.ReadRequest
	readIssueCalls   int
}

func (f *fakeReaders) ReadIssue(
	_ context.Context,
	request issuekernel.ReadRequest,
) (issuekernel.View, error) {
	f.readIssueCalls++
	f.readIssueRequest = request
	return f.issue, nil
}

type fakeChanges struct {
	issueIDConfiguration configuration.IssueIDConfiguration
	createIssue          CreateIssue
	editIssue            EditIssue
	applyDocument        ApplyDocument
	applyMode            ApplyMode
	applyOutcome         DocumentApplied
	editOutcome          IssueEdited
}

func (f *fakeChanges) CreateIssue(
	_ context.Context,
	issueIDConfiguration configuration.IssueIDConfiguration,
	command CreateIssue,
) (IssueCreated, error) {
	f.issueIDConfiguration = issueIDConfiguration
	f.createIssue = command
	return IssueCreated{}, nil
}

func (f *fakeChanges) EditIssue(
	_ context.Context,
	command EditIssue,
) (IssueEdited, error) {
	f.editIssue = command
	return f.editOutcome, nil
}

func (f *fakeChanges) ApplyDocument(
	_ context.Context,
	issueIDConfiguration configuration.IssueIDConfiguration,
	command ApplyDocument,
	mode ApplyMode,
) (DocumentApplied, error) {
	f.issueIDConfiguration = issueIDConfiguration
	f.applyDocument = command
	f.applyMode = mode
	return f.applyOutcome, nil
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

func newTestPlanner(t *testing.T, changes *fakeChanges) *Planner {
	t.Helper()
	return newTestPlannerWithReaders(t, changes, &fakeReaders{})
}

func newTestPlannerWithReaders(
	t *testing.T,
	changes *fakeChanges,
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
