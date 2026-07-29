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
		isBlocked  bool
		checkpoint bool
	}{
		{name: "ReadyTask", issue: eligibilityIssue("task", "open"), ready: true},
		{name: "ReadyWorkstream", issue: eligibilityIssue("workstream", "open"), ready: true},
		{name: "ActionableCheckpoint", issue: eligibilityIssue("checkpoint", "open"), checkpoint: true},
		{name: "Routine", issue: eligibilityIssue("routine", "open")},
		{name: "Closed", issue: eligibilityIssue("task", "closed")},
		{name: "Cancelled", issue: eligibilityIssue("task", "cancelled")},
		{name: "Claimed", issue: issue.Issue{Type: "task", Lifecycle: "open", ActiveClaim: claimed}},
		{name: "Waiting", issue: issue.Issue{Type: "task", Lifecycle: "open", Waiting: waiting}},
		{name: "BlockedTask", issue: eligibilityIssue("task", "open"), blocked: true, isBlocked: true},
		{name: "BlockedCheckpoint", issue: eligibilityIssue("checkpoint", "open"), blocked: true, isBlocked: true},
		{name: "WaitingBlockedTask", issue: issue.Issue{Type: "task", Lifecycle: "open", Waiting: waiting}, blocked: true},
		{name: "BlockedRoutine", issue: eligibilityIssue("routine", "open"), blocked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eligibility, err := EvaluateEligibility(issue.Summary{
				Issue: tt.issue, Blocked: tt.blocked,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.ready, eligibility.ReadyForClaim())
			assert.Equal(t, tt.isBlocked, eligibility.Blocked())
			assert.Equal(t, tt.checkpoint, eligibility.ActionableCheckpoint())
		})
	}
}

func eligibilityIssue(kind, lifecycle string) issue.Issue {
	return issue.Issue{Type: kind, Lifecycle: lifecycle}
}
