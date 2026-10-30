package execution

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestEligibility(t *testing.T) {
	claimed := &issue.ActiveClaim{Actor: "engineer", StartedAt: 1_700_000_000}
	waiting := &issue.Waiting{Reason: "Root review", Since: 1_700_000_000}

	tests := []struct {
		name       string
		issue      issue.Issue
		blocked    bool
		ready      bool
		waiting    bool
		isBlocked  bool
		checkpoint bool
	}{
		{name: "ReadyTask", issue: eligibilityIssue("task", "open"), ready: true},
		{name: "ReadyWorkstream", issue: eligibilityIssue("workstream", "open"), ready: true},
		{name: "ActionableCheckpoint", issue: eligibilityIssue("checkpoint", "open"), checkpoint: true},
		{name: "Routine", issue: eligibilityIssue("routine", "open")},
		{name: "Closed", issue: eligibilityIssue("task", "closed")},
		{name: "Cancelled", issue: eligibilityIssue("task", "cancelled")},
		{name: "Claimed", issue: issue.Issue{Type: new("task"), Lifecycle: new("open"), ActiveClaim: claimed}},
		{name: "Waiting", issue: issue.Issue{Type: new("task"), Lifecycle: new("open"), Waiting: waiting}, waiting: true},
		{name: "WaitingRoutine", issue: issue.Issue{Type: new("routine"), Lifecycle: new("open"), Waiting: waiting}},
		{name: "BlockedTask", issue: eligibilityIssue("task", "open"), blocked: true, isBlocked: true},
		{name: "BlockedCheckpoint", issue: eligibilityIssue("checkpoint", "open"), blocked: true, isBlocked: true},
		{
			name:    "WaitingBlockedTask",
			issue:   issue.Issue{Type: new("task"), Lifecycle: new("open"), Waiting: waiting},
			blocked: true,
			waiting: true,
		},
		{name: "BlockedRoutine", issue: eligibilityIssue("routine", "open"), blocked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eligibility, err := EvaluateEligibility(issue.Summary{
				Issue: tt.issue, Blocked: tt.blocked,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.ready, eligibility.ReadyForClaim())
			assert.Equal(t, tt.waiting, eligibility.WaitingForContinuation())
			assert.Equal(t, tt.isBlocked, eligibility.Blocked())
			assert.Equal(t, tt.checkpoint, eligibility.ActionableCheckpoint())
		})
	}
}

func TestEligibilitySkipsConflictsAfterKnownExclusions(t *testing.T) {
	conflict := []issue.ConflictReference{{ID: "conflict-title", Field: issue.FieldTitle}}
	tests := []struct {
		name    string
		issue   issue.Issue
		blocked bool
		skipped bool
	}{
		{name: "TitleConflict", issue: issue.Issue{Type: new("task"), Lifecycle: new("open"), Conflicts: conflict}, skipped: true},
		{
			name:    "UnknownKind",
			issue:   issue.Issue{Lifecycle: new("open"), Conflicts: []issue.ConflictReference{{ID: "conflict-kind", Field: issue.FieldKind}}},
			skipped: true,
		},
		{
			name:    "UnknownLifecycle",
			issue:   issue.Issue{Type: new("task"), Conflicts: []issue.ConflictReference{{ID: "conflict-lifecycle", Field: issue.FieldLifecycle}}},
			skipped: true,
		},
		{name: "Closed", issue: issue.Issue{Type: new("task"), Lifecycle: new("closed"), Conflicts: conflict}},
		{
			name:  "ClosedUnknownKind",
			issue: issue.Issue{Lifecycle: new("closed"), Conflicts: []issue.ConflictReference{{ID: "conflict-kind", Field: issue.FieldKind}}},
		},
		{
			name:  "Claimed",
			issue: issue.Issue{Type: new("task"), Lifecycle: new("open"), ActiveClaim: &issue.ActiveClaim{Actor: "worker", StartedAt: 1}, Conflicts: conflict},
		},
		{name: "Routine", issue: issue.Issue{Type: new("routine"), Lifecycle: new("open"), Conflicts: conflict}},
		{name: "Checkpoint", issue: issue.Issue{Type: new("checkpoint"), Lifecycle: new("open"), Conflicts: conflict}},
		{
			name:  "Waiting",
			issue: issue.Issue{Type: new("task"), Lifecycle: new("open"), Waiting: &issue.Waiting{Reason: "review", Since: 1}, Conflicts: conflict},
		},
		{
			name:  "WaitingUnknownLifecycle",
			issue: issue.Issue{Type: new("task"), Waiting: &issue.Waiting{Reason: "review", Since: 1}, Conflicts: []issue.ConflictReference{{ID: "conflict-lifecycle", Field: issue.FieldLifecycle}}},
		},
		{name: "Blocked", issue: issue.Issue{Type: new("task"), Lifecycle: new("open"), Conflicts: conflict}, blocked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eligibility, err := EvaluateEligibility(issue.Summary{Issue: tt.issue, Blocked: tt.blocked})
			require.NoError(t, err)
			assert.Equal(t, tt.skipped, eligibility.SkippedForConflict())
			assert.False(t, eligibility.ReadyForClaim())
			assert.False(t, eligibility.ActionableCheckpoint())
		})
	}
}

func TestEligibilityRetainsInheritedConflictReason(t *testing.T) {
	eligibility, err := EvaluateEligibility(issue.Summary{
		Issue:                eligibilityIssue("task", "open"),
		EligibilityConflicts: []issue.ConflictReference{{ID: "ancestor-summary", Field: issue.FieldSummary}},
	})
	require.NoError(t, err)
	assert.True(t, eligibility.SkippedForConflict())
	assert.False(t, eligibility.ReadyForClaim())
}

func eligibilityIssue(kind, lifecycle string) issue.Issue {
	return issue.Issue{Type: new(kind), Lifecycle: new(lifecycle)}
}
