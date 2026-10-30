package execution

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestClaimIssueAllowsUnrelatedTitleConflict(t *testing.T) {
	now := time.Unix(20, 0)
	state := loadExecutionState(t, issue.Snapshot{
		ID: issue.MustID("an-1"), Kind: new(issue.KindTask),
		Lifecycle: new(issue.LifecycleOpen), Priority: new(issue.PriorityNormal),
		Created: now, Updated: now,
		Conflicts: []issue.ConflictReference{{ID: "conflict-title", Field: issue.FieldTitle}},
	})
	policy, err := LoadClaimIssue(ClaimIssueSnapshot{
		BoardID: "board", Revision: 1, Candidate: &state, OccurredAt: now,
	})
	require.NoError(t, err)

	out, err := policy.ClaimIssue(ClaimIssue{IssueID: state.ID(), Assignee: "worker"})
	require.NoError(t, err)
	assert.Equal(t, issue.Actor("worker"), out.Issue.Assignee())
	assert.Nil(t, out.Issue.Title())
	assert.Equal(t, state.Conflicts(), out.Issue.Conflicts())
}

func TestClaimIssueReportsUnknownLifecycle(t *testing.T) {
	now := time.Unix(20, 0)
	state := loadExecutionState(t, issue.Snapshot{
		ID: issue.MustID("an-1"), Title: new("Task"), Kind: new(issue.KindTask),
		Priority: new(issue.PriorityNormal), Created: now, Updated: now,
		Conflicts: []issue.ConflictReference{{ID: "conflict-lifecycle", Field: issue.FieldLifecycle}},
	})
	policy, err := LoadClaimIssue(ClaimIssueSnapshot{
		BoardID: "board", Revision: 1, Candidate: &state, OccurredAt: now,
	})
	require.NoError(t, err)

	_, err = policy.ClaimIssue(ClaimIssue{IssueID: state.ID(), Assignee: "worker"})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.ErrorContains(t, err, "conflict-lifecycle")
}

func TestClaimNextCountsConflictOnlyAfterKnownExclusions(t *testing.T) {
	now := time.Unix(20, 0)
	for _, tt := range []struct {
		name        string
		waiting     *issue.WaitingState
		wantSkipped int
	}{
		{name: "UnresolvedTitle", wantSkipped: 3},
		{name: "WaitingUnresolvedTitle", waiting: &issue.WaitingState{Reason: "review", Since: now}, wantSkipped: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := loadExecutionState(t, issue.Snapshot{
				ID: issue.MustID("an-1"), Kind: new(issue.KindTask),
				Lifecycle: new(issue.LifecycleOpen), Priority: new(issue.PriorityNormal),
				Waiting: tt.waiting, Created: now, Updated: now,
				Conflicts: []issue.ConflictReference{{ID: "conflict-title", Field: issue.FieldTitle}},
			})
			policy, err := LoadClaimNext(ClaimNextSnapshot{
				BoardID: "board", Revision: 1, Candidate: &state, OccurredAt: now, SkippedConflicts: 2,
			})
			require.NoError(t, err)
			out, err := policy.ClaimNext(ClaimNext{Assignee: "worker"})
			require.NoError(t, err)
			assert.Nil(t, out.Issue)
			assert.Equal(t, tt.wantSkipped, out.SkippedConflicts)
		})
	}
}

func TestClaimNextRetainsPrerequisiteConflict(t *testing.T) {
	now := time.Unix(20, 0)
	state := executionTestIssue(t, "an-task", issue.StatusReady)
	for _, tt := range []struct {
		name  string
		field issue.Field
	}{
		{name: "Lifecycle", field: issue.FieldLifecycle},
		{name: "Checkpoint", field: issue.FieldCheckpoint},
	} {
		t.Run(tt.name, func(t *testing.T) {
			prerequisite := executionTestIssue(t, "an-prerequisite", issue.StatusClosed).Snapshot()
			if tt.field == issue.FieldLifecycle {
				prerequisite.Lifecycle = nil
				prerequisite.ClosedAt = nil
			} else {
				prerequisite.Kind = new(issue.KindCheckpoint)
			}
			prerequisite.Conflicts = []issue.ConflictReference{{ID: "prerequisite-conflict", Field: tt.field}}
			dependency := loadExecutionState(t, prerequisite)
			policy, err := LoadClaimNext(ClaimNextSnapshot{
				BoardID: "board", Revision: 1, Candidate: &state, OccurredAt: now,
				Prerequisites: []ClaimPrerequisite{{Lifecycle: dependency.Lifecycle(), Conflicts: dependency.Conflicts()}},
			})
			require.NoError(t, err)
			out, err := policy.ClaimNext(ClaimNext{Assignee: "worker"})
			require.NoError(t, err)
			assert.Nil(t, out.Issue)
			assert.Equal(t, 1, out.SkippedConflicts)

			direct, err := LoadClaimIssue(ClaimIssueSnapshot{
				BoardID: "board", Revision: 1, Candidate: &state, OccurredAt: now,
				Prerequisites: []ClaimPrerequisite{{Lifecycle: dependency.Lifecycle(), Conflicts: dependency.Conflicts()}},
			})
			require.NoError(t, err)
			_, err = direct.ClaimIssue(ClaimIssue{IssueID: state.ID(), Assignee: "worker"})
			assert.ErrorContains(t, err, "prerequisite-conflict")
		})
	}
}

func TestCloseIssuePreservesUnrelatedConflicts(t *testing.T) {
	now := time.Unix(20, 0)
	state := executionTestIssue(t, "an-task", issue.StatusReady).Snapshot()
	state.Title = nil
	state.Conflicts = []issue.ConflictReference{
		{ID: "title-conflict", Field: issue.FieldTitle},
		{ID: "parent-conflict", Field: issue.FieldParent},
	}
	current := loadExecutionState(t, state)
	policy, err := LoadLifecycle(LifecycleSnapshot{BoardID: "board", Revision: 1, Issue: current, OccurredAt: now})
	require.NoError(t, err)
	out, err := policy.CloseIssue(CloseIssue{IssueID: current.ID()})
	require.NoError(t, err)
	assert.Nil(t, out.Issue.Title())
	assert.Equal(t, state.Conflicts, out.Issue.Conflicts())
	assert.Equal(t, new(issue.LifecycleClosed), out.Issue.Lifecycle())
}

func TestCloseIssueDoesNotClearConflictedRecoveryState(t *testing.T) {
	now := time.Unix(20, 0)
	state := executionTestIssue(t, "an-task", issue.StatusReady).Snapshot()
	state.Conflicts = []issue.ConflictReference{{ID: "state-conflict", Field: issue.FieldState}}
	current := loadExecutionState(t, state)
	policy, err := LoadLifecycle(LifecycleSnapshot{BoardID: "board", Revision: 1, Issue: current, OccurredAt: now})
	require.NoError(t, err)
	_, err = policy.CloseIssue(CloseIssue{IssueID: current.ID()})
	assert.ErrorContains(t, err, "state-conflict")
}

func TestClaimNextCountsDisputedRequestedLabel(t *testing.T) {
	now := time.Unix(20, 0)
	label, err := issue.NewLabel("review")
	require.NoError(t, err)
	for _, tt := range []struct {
		name, disputed string
		skipped        int
	}{
		{name: "RequestedMembership", disputed: "review", skipped: 1},
		{name: "UnrelatedMissingLabel", disputed: "other", skipped: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := executionTestIssue(t, "an-task", issue.StatusReady).Snapshot()
			snapshot.Conflicts = []issue.ConflictReference{{ID: "label-conflict", Field: issue.FieldLabels, Key: tt.disputed}}
			state := loadExecutionState(t, snapshot)
			policy, err := LoadClaimNext(ClaimNextSnapshot{BoardID: "board", Revision: 1, Candidate: &state, OccurredAt: now})
			require.NoError(t, err)
			out, err := policy.ClaimNext(ClaimNext{Assignee: "worker", LabelsAll: []issue.Label{label}})
			require.NoError(t, err)
			assert.Nil(t, out.Issue)
			assert.Equal(t, tt.skipped, out.SkippedConflicts)
		})
	}
}
