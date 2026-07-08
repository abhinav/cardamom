package execution

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestLifecyclePolicyReportsInvalidTransitions(t *testing.T) {
	t.Parallel()

	now := time.Unix(20, 0).UTC()
	closed := loadExecutionState(t, issue.Snapshot{
		ID: issue.MustID("an-closed"), Title: "Closed task", Kind: issue.KindTask,
		Lifecycle: issue.LifecycleClosed, Priority: issue.PriorityNormal,
		Created: now, Updated: now, ClosedAt: &now,
	})
	policy, err := LoadLifecycle(LifecycleSnapshot{
		BoardID: mustExecutionBoardID(t), Revision: 1, Issue: closed, OccurredAt: now,
	})
	require.NoError(t, err)
	_, err = policy.CloseIssue(CloseIssue{IssueID: closed.ID()})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.EqualError(t, err, "close requires lifecycle open or cancelled; issue lifecycle is closed")

	open := loadExecutionState(t, issue.Snapshot{
		ID: issue.MustID("an-open"), Title: "Open task", Kind: issue.KindTask,
		Lifecycle: issue.LifecycleOpen, Priority: issue.PriorityNormal,
		Created: now, Updated: now,
	})
	policy, err = LoadLifecycle(LifecycleSnapshot{
		BoardID: mustExecutionBoardID(t), Revision: 1, Issue: open, OccurredAt: now,
	})
	require.NoError(t, err)
	_, err = policy.ReopenIssue(ReopenIssue{IssueID: open.ID()})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.EqualError(t, err, "reopen requires lifecycle closed or cancelled; issue lifecycle is open")
}

func TestLifecyclePolicyClosesCancelledIssue(t *testing.T) {
	t.Parallel()

	createdAt := time.Unix(10, 0).UTC()
	occurredAt := time.Unix(20, 0).UTC()
	cancelled := loadExecutionState(t, issue.Snapshot{
		ID: issue.MustID("an-cancelled"), Title: "Cancelled task", Kind: issue.KindTask,
		Lifecycle: issue.LifecycleCancelled, Priority: issue.PriorityNormal,
		Created: createdAt, Updated: createdAt, ClosedAt: &createdAt,
	})
	policy, err := LoadLifecycle(LifecycleSnapshot{
		BoardID: mustExecutionBoardID(t), Revision: 1, Issue: cancelled,
		OccurredAt: occurredAt,
	})
	require.NoError(t, err)

	closed, err := policy.CloseIssue(CloseIssue{IssueID: cancelled.ID()})
	require.NoError(t, err)
	assert.Equal(t, issue.LifecycleClosed, closed.Issue.Lifecycle())
}

func TestClaimPolicyReportsInvalidCandidateState(t *testing.T) {
	t.Parallel()

	now := time.Unix(20, 0).UTC()
	claimedAt := time.Unix(10, 0).UTC()
	waiting, err := issue.NewWaitingState("review response arrives", claimedAt)
	require.NoError(t, err)
	openPrerequisite := loadExecutionState(t, issue.Snapshot{
		ID: issue.MustID("an-prerequisite"), Title: "Open prerequisite", Kind: issue.KindTask,
		Lifecycle: issue.LifecycleOpen, Priority: issue.PriorityNormal,
		Created: claimedAt, Updated: claimedAt,
	})
	tests := []struct {
		name          string
		candidate     issue.Snapshot
		prerequisites []issue.State
		automatic     bool
		want          string
	}{
		{
			name: "ClosedLifecycle",
			candidate: issue.Snapshot{
				ID: issue.MustID("an-closed"), Title: "Closed task", Kind: issue.KindTask,
				Lifecycle: issue.LifecycleClosed, Priority: issue.PriorityNormal,
				Created: claimedAt, Updated: claimedAt, ClosedAt: &claimedAt,
			},
			want: "claim requires lifecycle open; issue lifecycle is closed",
		},
		{
			name: "ActiveClaim",
			candidate: issue.Snapshot{
				ID: issue.MustID("an-claimed"), Title: "Claimed task", Kind: issue.KindTask,
				Lifecycle: issue.LifecycleOpen, Priority: issue.PriorityNormal,
				ActiveClaim: &issue.ClaimState{Actor: issue.NewActor("owner"), StartedAt: claimedAt},
				Created:     claimedAt, Updated: claimedAt,
			},
			want: `claim requires an unclaimed issue; active claim belongs to "owner"`,
		},
		{
			name: "NonExecutableType",
			candidate: issue.Snapshot{
				ID: issue.MustID("an-checkpoint"), Title: "Checkpoint", Kind: issue.KindCheckpoint,
				Lifecycle: issue.LifecycleOpen, Priority: issue.PriorityNormal,
				Created: claimedAt, Updated: claimedAt,
			},
			want: "claim requires an executable issue; issue type is checkpoint",
		},
		{
			name: "WaitingAutomaticSelection",
			candidate: issue.Snapshot{
				ID: issue.MustID("an-waiting"), Title: "Waiting task", Kind: issue.KindTask,
				Lifecycle: issue.LifecycleOpen, Priority: issue.PriorityNormal, Waiting: waiting,
				Created: claimedAt, Updated: claimedAt,
			},
			automatic: true,
			want:      "automatic claim excludes waiting issues; claim the issue by ID",
		},
		{
			name: "UnresolvedDependency",
			candidate: issue.Snapshot{
				ID: issue.MustID("an-blocked"), Title: "Blocked task", Kind: issue.KindTask,
				Lifecycle: issue.LifecycleOpen, Priority: issue.PriorityNormal,
				Created: claimedAt, Updated: claimedAt,
			},
			prerequisites: []issue.State{openPrerequisite},
			want:          "claim requires all dependencies to be closed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := loadExecutionState(t, tt.candidate)
			var err error
			if tt.automatic {
				policy, loadErr := LoadClaimNext(ClaimNextSnapshot{
					BoardID: mustExecutionBoardID(t), Revision: 1, Candidate: &candidate,
					Prerequisites: tt.prerequisites, OccurredAt: now,
				})
				require.NoError(t, loadErr)
				_, err = policy.ClaimNext(ClaimNext{Assignee: issue.NewActor("worker")})
			} else {
				policy, loadErr := LoadClaimIssue(ClaimIssueSnapshot{
					BoardID: mustExecutionBoardID(t), Revision: 1, Candidate: &candidate,
					Prerequisites: tt.prerequisites, OccurredAt: now,
				})
				require.NoError(t, loadErr)
				_, err = policy.ClaimIssue(ClaimIssue{
					IssueID: candidate.ID(), Assignee: issue.NewActor("worker"),
				})
			}
			assert.Equal(t, errkind.Conflict, errkind.Of(err))
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestCheckpointPolicyReportsInvalidResolutionState(t *testing.T) {
	t.Parallel()

	now := time.Unix(20, 0).UTC()
	task := loadExecutionState(t, issue.Snapshot{
		ID: issue.MustID("an-task"), Title: "Task", Kind: issue.KindTask,
		Lifecycle: issue.LifecycleOpen, Priority: issue.PriorityNormal,
		Created: now, Updated: now,
	})
	policy, err := LoadApproveCheckpoint(ApproveCheckpointSnapshot{
		BoardID: mustExecutionBoardID(t), Revision: 1, Issue: task, OccurredAt: now,
	})
	require.NoError(t, err)
	_, err = policy.ApproveCheckpoint(ApproveCheckpoint{IssueID: task.ID()})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	assert.EqualError(t, err, "checkpoint resolution requires type checkpoint; issue type is task")

	checkpoint := loadExecutionState(t, issue.Snapshot{
		ID: issue.MustID("an-checkpoint"), Title: "Checkpoint", Kind: issue.KindCheckpoint,
		Lifecycle: issue.LifecycleOpen, Priority: issue.PriorityNormal,
		Created: now, Updated: now,
	})
	prerequisite := loadExecutionState(t, issue.Snapshot{
		ID: issue.MustID("an-prerequisite"), Title: "Prerequisite", Kind: issue.KindTask,
		Lifecycle: issue.LifecycleOpen, Priority: issue.PriorityNormal,
		Created: now, Updated: now,
	})
	policy, err = LoadApproveCheckpoint(ApproveCheckpointSnapshot{
		BoardID: mustExecutionBoardID(t), Revision: 1, Issue: checkpoint,
		Prerequisites: []issue.State{prerequisite}, OccurredAt: now,
	})
	require.NoError(t, err)
	_, err = policy.ApproveCheckpoint(ApproveCheckpoint{IssueID: checkpoint.ID()})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.EqualError(t, err, "checkpoint resolution requires all dependencies to be closed")
}

func loadExecutionState(t *testing.T, snapshot issue.Snapshot) issue.State {
	t.Helper()
	state, err := issue.Load(snapshot)
	require.NoError(t, err)
	return state
}

func mustExecutionBoardID(t *testing.T) board.ID {
	t.Helper()
	id, err := board.NewID("board")
	require.NoError(t, err)
	return id
}
