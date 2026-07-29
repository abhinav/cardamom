package planning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	issuekernel "go.abhg.dev/cardamom/internal/issue"
)

func TestApplyExistingPolicyText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  ApplyExistingPolicy
		text  string
	}{
		{name: "Default", want: ApplyExistingError, text: "error"},
		{name: "Error", value: "error", want: ApplyExistingError, text: "error"},
		{name: "Skip", value: "skip", want: ApplyExistingSkip, text: "skip"},
		{name: "Update", value: "update", want: ApplyExistingUpdate, text: "update"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, err := NewApplyExistingPolicy(test.value)
			require.NoError(t, err)
			assert.Equal(t, test.want, policy)
			assert.Equal(t, test.text, policy.String())
		})
	}

	_, err := NewApplyExistingPolicy("merge")
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	assert.ErrorContains(t, err, `invalid existing issue policy "merge"`)
}

func TestApplyActionString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", ApplyActionUnknown.String())
	assert.Equal(t, "create", ApplyActionCreate.String())
	assert.Equal(t, "update", ApplyActionUpdate.String())
	assert.Equal(t, "skip", ApplyActionSkip.String())
	assert.Equal(t, "no_change", ApplyActionNoChange.String())
}

func TestCreateIssueNormalizesInput(t *testing.T) {
	t.Parallel()

	key := ExternalKey("source:build")
	board, err := LoadCreate(CreateSnapshot{
		BoardID:     mustBoardID(t, "board"),
		Revision:    domainboard.Revision(4),
		AllocatedID: issuekernel.MustID("an-7"),
		ExistingIDs: []issuekernel.ID{issuekernel.MustID("an-1")},
		OccurredAt:  time.Unix(10, 0).UTC(),
	})
	require.NoError(t, err)

	parent := issuekernel.MustID("an-1")
	out, err := board.CreateIssue(CreateIssue{
		Title:       "  Build boundary  ",
		Kind:        issuekernel.KindTask,
		Priority:    issuekernel.PriorityHigh,
		Labels:      []issuekernel.Label{issuekernel.MustLabel("arch"), issuekernel.MustLabel("arch")},
		DependsOn:   []issuekernel.ID{issuekernel.MustID("an-1")},
		Parent:      &parent,
		ExternalKey: &key,
	})
	require.NoError(t, err)
	assert.Equal(t, "Build boundary", out.Issue.Title())
	assert.Equal(t, []issuekernel.Label{issuekernel.MustLabel("arch")}, out.Labels)
	require.NotNil(t, out.Parent)
	assert.Equal(t, issuekernel.MustID("an-1"), *out.Parent)
	assert.Equal(t, &key, out.ExternalKey)
}

func TestCreateIssueRejectsBoundExternalKey(t *testing.T) {
	t.Parallel()

	key := ExternalKey("source:build")
	board, err := LoadCreate(CreateSnapshot{
		BoardID:     mustBoardID(t, "board"),
		Revision:    domainboard.Revision(4),
		AllocatedID: issuekernel.MustID("an-7"),
		ExternalKeyOwner: &ExternalKeyOwner{
			Key: key, IssueID: issuekernel.MustID("an-1"),
		},
		OccurredAt: time.Unix(10, 0).UTC(),
	})
	require.NoError(t, err)

	_, err = board.CreateIssue(CreateIssue{
		Title: "Build boundary", Kind: issuekernel.KindTask,
		Priority: issuekernel.PriorityNormal, ExternalKey: &key,
	})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.EqualError(t, err, `external key "source:build" belongs to issue "an-1"`)
}

func TestCreateIssueRejectsMissingParent(t *testing.T) {
	t.Parallel()

	board, err := LoadCreate(CreateSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 1,
		AllocatedID: issuekernel.MustID("an-2"), ExistingIDs: []issuekernel.ID{issuekernel.MustID("an-1")},
		OccurredAt: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	parent := issuekernel.MustID("an-missing")
	_, err = board.CreateIssue(CreateIssue{
		Title: "Missing parent", Kind: issuekernel.KindTask, Priority: issuekernel.PriorityNormal,
		Parent: &parent,
	})
	assert.Equal(t, errkind.NotFound, errkind.Of(err))
}

func TestCreateIssueRejectsSelfParent(t *testing.T) {
	t.Parallel()

	allocated := issuekernel.MustID("an-2")
	board, err := LoadCreate(CreateSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 1,
		AllocatedID: allocated, ExistingIDs: []issuekernel.ID{issuekernel.MustID("an-1")},
		OccurredAt: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	_, err = board.CreateIssue(CreateIssue{
		Title: "Self parent", Kind: issuekernel.KindTask, Priority: issuekernel.PriorityNormal,
		Parent: &allocated,
	})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestCreateIssueRejectsInvalidLabelInsteadOfDroppingIt(t *testing.T) {
	t.Parallel()

	board, err := LoadCreate(CreateSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 1,
		AllocatedID: issuekernel.MustID("an-1"), OccurredAt: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	_, err = board.CreateIssue(CreateIssue{
		Title: "invalid label", Kind: issuekernel.KindTask, Priority: issuekernel.PriorityNormal,
		Labels: []issuekernel.Label{""},
	})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestEditIssueRejectsSelfDependenciesAndCycles(t *testing.T) {
	t.Parallel()

	child := loadIssue(t, "an-child", issuekernel.KindTask, issuekernel.StatusReady)
	parent := issuekernel.MustID("an-parent")
	board, err := LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 1, Issue: child,
		ExistingIDs: []issuekernel.ID{child.ID(), parent},
		DependencyAncestors: map[issuekernel.ID][]issuekernel.ID{
			parent: {child.ID()},
		},
		OccurredAt: time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)

	_, err = board.EditIssue(EditIssue{
		IssueID: child.ID(), AddDependencies: []issuekernel.ID{child.ID()},
	})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	assert.EqualError(t, err, "an issue cannot be its own dependency")
	_, err = board.EditIssue(EditIssue{
		IssueID: child.ID(), AddDependencies: []issuekernel.ID{parent},
	})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	assert.EqualError(t, err, "dependency graph must remain acyclic")
}

func TestEditIssueRequiresExistingDependencyRemoval(t *testing.T) {
	t.Parallel()

	child := loadIssue(t, "an-child", issuekernel.KindTask, issuekernel.StatusReady)
	parent := issuekernel.MustID("an-parent")
	board, err := LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 1, Issue: child,
		OccurredAt: time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)

	_, err = board.EditIssue(EditIssue{
		IssueID: child.ID(), RemoveDependencies: []issuekernel.ID{parent},
	})
	assert.Equal(t, errkind.NotFound, errkind.Of(err))
	assert.EqualError(t, err, `issue does not depend on "an-parent"`)
}

func TestWorkstreamCanHaveContainmentParent(t *testing.T) {
	t.Parallel()

	workstream := loadIssue(t, "an-workstream", issuekernel.KindWorkstream, issuekernel.StatusReady)
	parent := issuekernel.MustID("an-parent")
	board, err := LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 1, Issue: workstream,
		ExistingIDs: []issuekernel.ID{workstream.ID(), parent}, OccurredAt: time.Unix(20, 0).UTC(),
	})
	require.NoError(t, err)
	edited, err := board.EditIssue(EditIssue{IssueID: workstream.ID(), ParentSet: true, Parent: parent})
	require.NoError(t, err)
	assert.Equal(t, &parent, edited.Parent)
}

func TestEditIssueOwnsScalarAndKindTransitionPolicy(t *testing.T) {
	t.Parallel()

	now := time.Unix(20, 0).UTC()
	issue := loadIssue(t, "an-task", issuekernel.KindTask, issuekernel.StatusReady)
	child := loadIssue(t, "an-child", issuekernel.KindTask, issuekernel.StatusReady)
	board, err := LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 3, Issue: issue,
		DirectChildren: []issuekernel.State{child}, OccurredAt: now,
	})
	require.NoError(t, err)
	title := "  Workstream control  "
	kind := issuekernel.KindWorkstream
	out, err := board.EditIssue(EditIssue{IssueID: issue.ID(), Title: &title, Kind: &kind})
	require.NoError(t, err)
	assert.Equal(t, "Workstream control", out.Issue.Title())
	assert.Equal(t, issuekernel.KindWorkstream, out.Issue.Kind())
	assert.Equal(t, now, out.Issue.Updated())
	assert.True(t, out.Changed)

	board, err = LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 3, Issue: issue,
		OccurredAt: now,
	})
	require.NoError(t, err)
	out, err = board.EditIssue(EditIssue{IssueID: issue.ID(), Kind: &kind})
	require.NoError(t, err)
	assert.Equal(t, issuekernel.KindWorkstream, out.Issue.Kind())
}

func TestEditIssueBindsExternalKeyIdempotently(t *testing.T) {
	t.Parallel()

	now := time.Unix(20, 0).UTC()
	current := loadIssue(t, "an-task", issuekernel.KindTask, issuekernel.StatusReady)
	key := ExternalKey("source:task")

	board, err := LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 3, Issue: current,
		OccurredAt: now,
	})
	require.NoError(t, err)
	out, err := board.EditIssue(EditIssue{IssueID: current.ID(), ExternalKey: &key})
	require.NoError(t, err)
	assert.True(t, out.Changed)
	assert.Equal(t, now, out.Issue.Updated())
	assert.Equal(t, &key, out.ExternalKey)

	board, err = LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 3, Issue: current,
		ExternalKeyOwner: &ExternalKeyOwner{Key: key, IssueID: current.ID()},
		OccurredAt:       now,
	})
	require.NoError(t, err)
	out, err = board.EditIssue(EditIssue{IssueID: current.ID(), ExternalKey: &key})
	require.NoError(t, err)
	assert.False(t, out.Changed)
	assert.Nil(t, out.ExternalKey)
	assert.Equal(t, current.Updated(), out.Issue.Updated())
}

func TestEditIssueRejectsExternalKeyOwnedByAnotherIssue(t *testing.T) {
	t.Parallel()

	current := loadIssue(t, "an-task", issuekernel.KindTask, issuekernel.StatusReady)
	key := ExternalKey("source:task")
	board, err := LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 3, Issue: current,
		ExternalKeyOwner: &ExternalKeyOwner{
			Key: key, IssueID: issuekernel.MustID("an-other"),
		},
		OccurredAt: time.Unix(20, 0).UTC(),
	})
	require.NoError(t, err)

	_, err = board.EditIssue(EditIssue{IssueID: current.ID(), ExternalKey: &key})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.EqualError(t, err, `external key "source:task" belongs to issue "an-other"`)
}

func TestEditIssueAllowsRoutineExecutableKindTransitions(t *testing.T) {
	t.Parallel()

	now := time.Unix(20, 0).UTC()
	for _, current := range []issuekernel.Kind{issuekernel.KindTask, issuekernel.KindWorkstream, issuekernel.KindRoutine} {
		for _, target := range []issuekernel.Kind{issuekernel.KindTask, issuekernel.KindWorkstream, issuekernel.KindRoutine} {
			issue := loadIssue(t, "an-edit", current, issuekernel.StatusReady)
			board, err := LoadEdit(EditSnapshot{
				BoardID: mustBoardID(t, "board"), Revision: 1, Issue: issue,
				OccurredAt: now,
			})
			require.NoError(t, err)
			edited, err := board.EditIssue(EditIssue{IssueID: issue.ID(), Kind: &target})
			require.NoErrorf(t, err, "%s to %s", current, target)
			assert.Equal(t, target, edited.Issue.Kind())
		}
	}
}

func TestEditIssueRejectsContradictoryGraphAndLabelChanges(t *testing.T) {
	t.Parallel()

	issue := loadIssue(t, "an-task", issuekernel.KindTask, issuekernel.StatusReady)
	dependency := issuekernel.MustID("an-dependency")
	label := issuekernel.MustLabel("docs")
	board, err := LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 1, Issue: issue,
		Labels: []issuekernel.Label{label}, Dependencies: []issuekernel.ID{dependency},
		ExistingIDs: []issuekernel.ID{dependency}, OccurredAt: time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)

	_, err = board.EditIssue(EditIssue{
		IssueID: issue.ID(), AddDependencies: []issuekernel.ID{dependency},
		RemoveDependencies: []issuekernel.ID{dependency},
	})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	_, err = board.EditIssue(EditIssue{
		IssueID: issue.ID(), AddLabels: []issuekernel.Label{label}, RemoveLabels: []issuekernel.Label{label},
	})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestEditIssueReplacesLabelsAtomically(t *testing.T) {
	t.Parallel()

	issue := loadIssue(t, "an-task", issuekernel.KindTask, issuekernel.StatusReady)
	board, err := LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 1, Issue: issue,
		Labels:     []issuekernel.Label{issuekernel.MustLabel("old"), issuekernel.MustLabel("keep")},
		OccurredAt: time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)
	replacement := []issuekernel.Label{issuekernel.MustLabel("keep"), issuekernel.MustLabel("new")}

	out, err := board.EditIssue(EditIssue{
		IssueID: issue.ID(), ReplaceLabels: &replacement,
	})
	require.NoError(t, err)
	assert.Equal(t, replacement, out.Labels)
	assert.True(t, out.Changed)

	_, err = board.EditIssue(EditIssue{
		IssueID: issue.ID(), ReplaceLabels: &replacement,
		AddLabels: []issuekernel.Label{issuekernel.MustLabel("other")},
	})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestEditIssueAddsAndRemovesLabels(t *testing.T) {
	t.Parallel()

	issue := loadIssue(t, "an-task", issuekernel.KindTask, issuekernel.StatusReady)
	board, err := LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 2, Issue: issue,
		Labels: []issuekernel.Label{
			issuekernel.MustLabel("existing"),
			issuekernel.MustLabel("remove"),
		},
		OccurredAt: time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)
	out, err := board.EditIssue(EditIssue{
		IssueID: issue.ID(),
		AddLabels: []issuekernel.Label{
			issuekernel.MustLabel("new"),
			issuekernel.MustLabel("new"),
		},
		RemoveLabels: []issuekernel.Label{issuekernel.MustLabel("remove")},
	})
	require.NoError(t, err)
	assert.Equal(t, []issuekernel.Label{issuekernel.MustLabel("existing"), issuekernel.MustLabel("new")}, out.Labels)
	assert.True(t, out.Changed)
}

func TestEditIssueContainmentDistinguishesNoOpReplacementAndRemoval(t *testing.T) {
	t.Parallel()

	child := issuekernel.MustID("an-child")
	childIssue := loadIssue(t, child.String(), issuekernel.KindTask, issuekernel.StatusReady)
	parent := issuekernel.MustID("an-parent")
	other := issuekernel.MustID("an-other")
	board, err := LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 1, Issue: childIssue,
		Parent:      &parent,
		ExistingIDs: []issuekernel.ID{child, parent, other},
		OccurredAt:  time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)
	out, err := board.EditIssue(EditIssue{IssueID: child, ParentSet: true, Parent: parent})
	require.NoError(t, err)
	assert.False(t, out.Changed)

	board, err = LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 1, Issue: childIssue,
		Parent: &parent, ContainmentAncestors: []issuekernel.ID{},
		ExistingIDs: []issuekernel.ID{child, parent, other},
		OccurredAt:  time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)
	out, err = board.EditIssue(EditIssue{IssueID: child, ParentSet: true, Parent: other})
	require.NoError(t, err)
	require.NotNil(t, out.Parent)
	assert.Equal(t, other, *out.Parent)

	board, err = LoadEdit(EditSnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 1, Issue: childIssue,
		Parent: &parent, ExistingIDs: []issuekernel.ID{child, parent},
		OccurredAt: time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)
	out, err = board.EditIssue(EditIssue{IssueID: child, ParentSet: true})
	require.NoError(t, err)
	assert.Nil(t, out.Parent)
}

func TestApplyDocumentPlansTypedReferencesWithoutDurableState(t *testing.T) {
	t.Parallel()

	labels := []string{"plan"}
	dependencies := []ApplyIssueReference{{
		Kind: ApplyReferenceKey, Key: " producer key ",
	}}
	request := ApplyDocumentRequest{
		Version: 1, Mode: ApplyModeDryRun,
		Issues: []ApplyIssue{
			{
				Alias: new("parent"), Key: new(" producer key "),
				Title: new("Parent"), Type: new("workstream"), Labels: &labels,
			},
			{
				Alias: new("child"), Title: new("Child"), Type: new("task"),
				DependsOn: &dependencies,
				Parent: ApplyParentChange{
					Kind: ParentReplace,
					Reference: ApplyIssueReference{
						Kind: ApplyReferenceAlias, Alias: "parent",
					},
				},
			},
		},
	}
	document, err := applyDocumentCommand(request, configuration.Defaults().Issue.Summary.MaxBytes)
	require.NoError(t, err)
	policy, err := LoadApply(ApplySnapshot{
		BoardID: mustBoardID(t, "board"), Revision: 1,
		Issues: map[issuekernel.ID]ApplyIssueSnapshot{}, Mode: ApplyModeDryRun,
	})
	require.NoError(t, err)

	out, err := policy.ApplyDocument(document)
	require.NoError(t, err)
	assert.True(t, out.Receipt.DryRun)
	assert.Equal(t, ApplyCounts{Create: 2}, out.Receipt.Counts)
	assert.Equal(t, []ApplyAction{ApplyActionCreate, ApplyActionCreate}, []ApplyAction{
		out.Receipt.Entries[0].Action,
		out.Receipt.Entries[1].Action,
	})
	assert.Nil(t, out.Receipt.Entries[0].ID)
	require.Len(t, out.Applied, 2)
	assert.Empty(t, out.Applied[0].Issue.ID())
	assert.Equal(t, ExternalKey(" producer key "), *document.issues[0].key)
}

func TestApplyDocumentReferenceValuesRequireExplicitNamespaces(t *testing.T) {
	t.Parallel()

	_, err := NewIssueAlias("contains spaces")
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	_, err = NewExternalKey("")
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	key, err := NewExternalKey(" producer key ")
	require.NoError(t, err)
	assert.Equal(t, ExternalKey(" producer key "), key)
	_, err = applyReferenceCommand(ApplyIssueReference{})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestPlanningSnapshotsRejectIncompleteConsistencyInputs(t *testing.T) {
	t.Parallel()

	_, err := LoadCreate(CreateSnapshot{})
	assert.ErrorIs(t, err, ErrIncompleteSnapshot)
}

func mustBoardID(t *testing.T, value string) domainboard.ID {
	t.Helper()
	id, err := domainboard.NewID(value)
	require.NoError(t, err)
	return id
}

func loadIssue(t *testing.T, id string, kind issuekernel.Kind, status issuekernel.Status) issuekernel.State {
	t.Helper()
	lifecycle := status.Lifecycle()
	var activeClaim *issuekernel.ClaimState
	if status == issuekernel.StatusInProgress {
		lifecycle = issuekernel.LifecycleOpen
		activeClaim = &issuekernel.ClaimState{Actor: issuekernel.NewActor("worker"), StartedAt: time.Unix(1, 0).UTC()}
	}
	issue, err := issuekernel.Load(issuekernel.Snapshot{
		ID:          issuekernel.MustID(id),
		Title:       id,
		Kind:        kind,
		Lifecycle:   lifecycle,
		Priority:    issuekernel.PriorityNormal,
		ActiveClaim: activeClaim,
		Created:     time.Unix(1, 0).UTC(),
		Updated:     time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	return issue
}
