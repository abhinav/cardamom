package execution

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
	"go.uber.org/mock/gomock"
)

func TestExecutorDispatchesClaimAndReleaseAttribution(t *testing.T) {
	t.Parallel()

	state := executionTestIssue(t, "an-1", issue.StatusInProgress)
	view := issue.View{Detail: issue.Detail{
		Issue: issue.Issue{ID: "an-1"},
	}}
	changes := NewMockChanges(gomock.NewController(t))
	changes.EXPECT().ClaimIssue(gomock.Any(), ClaimIssue{
		IssueID: issue.MustID("an-1"), Assignee: issue.NewActor("worker"),
	}).Return(IssueClaimed{Issue: state}, nil)
	changes.EXPECT().ReleaseIssue(gomock.Any(), ReleaseIssue{
		IssueID: issue.MustID("an-1"), Actor: issue.NewActor("worker"),
	}).Return(IssueReleased{Issue: state}, nil)
	reader := NewMockIssueReader(gomock.NewController(t))
	reader.EXPECT().ReadIssue(gomock.Any(), issue.ReadRequest{
		IssueID: "an-1",
	}).Return(view, nil).Times(2)
	executor := NewExecutor(changes, reader)

	_, err := executor.ClaimIssue(t.Context(), issue.NewInvocation("worker"), ClaimIssueRequest{
		ID: "an-1", Assignee: "worker",
	})
	require.NoError(t, err)

	_, err = executor.ReleaseIssue(
		t.Context(),
		issue.NewInvocation("worker"),
		ReleaseIssueRequest{ID: "an-1"},
	)
	require.NoError(t, err)
}

func TestIssueProjectionPreservesStructuredState(t *testing.T) {
	t.Parallel()

	updatedAt := time.Unix(3, 0).UTC()
	state := executionTestIssue(t, "an-1", issue.StatusInProgress)
	recovery, err := issue.NewRecoveryState(
		"Preserve the diagnostic position.",
		"Inspect the secondary relay.",
		issue.NewActor("worker"),
		updatedAt,
	)
	require.NoError(t, err)
	state = state.WithRecoveryState(recovery, updatedAt)

	projected := issueProjection(state, 7)

	assert.Equal(t, new("Preserve the diagnostic position."), projected.State)
	assert.Equal(t, new("Inspect the secondary relay."), projected.NextAction)
}

func TestExecutorClaimWatchRetriesWithoutSleeping(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		state := executionTestIssue(t, "an-1", issue.StatusInProgress)
		depth := 2
		view := issue.View{
			Detail: issue.Detail{Issue: issue.Issue{ID: "an-1"}},
			Context: &issue.Context{
				Ancestors: []issue.ContextEntry{},
			},
		}
		command := ClaimNext{
			UnderID: issue.MustID("an-parent"), Assignee: issue.NewActor("worker"),
			LabelsAll: []issue.Label{issue.MustLabel("area:execution")},
			LabelsAny: []issue.Label{
				issue.MustLabel("phase:a"),
				issue.MustLabel("phase:b"),
			},
			LabelsNone: []issue.Label{issue.MustLabel("paused")},
		}
		changes := NewMockChanges(gomock.NewController(t))
		gomock.InOrder(
			changes.EXPECT().ClaimNext(gomock.Any(), command).Return(NextIssueClaimed{SkippedConflicts: 3}, nil),
			changes.EXPECT().ClaimNext(gomock.Any(), command).Return(NextIssueClaimed{Issue: &state, SkippedConflicts: 2}, nil),
		)
		reader := NewMockIssueReader(gomock.NewController(t))
		reader.EXPECT().ReadIssue(gomock.Any(), issue.ReadRequest{
			IssueID: "an-1", ContextDepth: &depth,
		}).Return(view, nil)
		executor := NewExecutor(changes, reader)

		result, err := executor.ClaimNext(t.Context(), issue.NewInvocation("worker"), ClaimNextRequest{
			Assignee: "worker", Watch: true, Interval: time.Hour,
			ContextDepth: &depth, UnderID: "an-parent",
			LabelsAll:  []string{"area:execution"},
			LabelsAny:  []string{"phase:a", "phase:b"},
			LabelsNone: []string{"paused"},
		})
		require.NoError(t, err)
		assert.Equal(t, &view, result.Issue)
		assert.Equal(t, 2, result.SkippedConflicts)
	})
}

func TestExecutorClaimNextRejectsInvalidLabelSelectors(t *testing.T) {
	t.Parallel()

	changes := NewMockChanges(gomock.NewController(t))
	executor := NewExecutor(changes, NewMockIssueReader(gomock.NewController(t)))

	_, err := executor.ClaimNext(
		t.Context(),
		issue.NewInvocation("worker"),
		ClaimNextRequest{
			Assignee:   "worker",
			LabelsNone: []string{"-paused"},
		},
	)
	require.ErrorContains(t, err, "label cannot start with + or -")
}

func TestExecutorClaimNextPreservesSkippedCountWithoutReadyWork(t *testing.T) {
	changes := NewMockChanges(gomock.NewController(t))
	changes.EXPECT().ClaimNext(gomock.Any(), ClaimNext{Assignee: "worker", LabelsAll: []issue.Label{}, LabelsAny: []issue.Label{}, LabelsNone: []issue.Label{}}).
		Return(NextIssueClaimed{SkippedConflicts: 4}, nil)
	executor := NewExecutor(changes, NewMockIssueReader(gomock.NewController(t)))

	result, err := executor.ClaimNext(t.Context(), issue.NewInvocation("worker"), ClaimNextRequest{Assignee: "worker"})
	require.NoError(t, err)
	assert.Nil(t, result.Issue)
	assert.Equal(t, 4, result.SkippedConflicts)
}

func TestExecutorClaimWatchPreservesSkippedCountOnCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		changes := NewMockChanges(gomock.NewController(t))
		changes.EXPECT().ClaimNext(gomock.Any(), ClaimNext{Assignee: "worker", LabelsAll: []issue.Label{}, LabelsAny: []issue.Label{}, LabelsNone: []issue.Label{}}).
			DoAndReturn(func(context.Context, ClaimNext) (NextIssueClaimed, error) {
				cancel()
				return NextIssueClaimed{SkippedConflicts: 2}, nil
			})
		executor := NewExecutor(changes, NewMockIssueReader(gomock.NewController(t)))

		result, err := executor.ClaimNext(ctx, issue.NewInvocation("worker"), ClaimNextRequest{Assignee: "worker", Watch: true})
		assert.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, result.Issue)
		assert.Equal(t, 2, result.SkippedConflicts)
	})
}

func TestExecutorExposesEligibilityReads(t *testing.T) {
	t.Parallel()

	readyIssues := []issue.Summary{{Issue: issue.Issue{ID: "an-ready"}}}
	blockedIssues := []issue.Summary{{Issue: issue.Issue{ID: "an-blocked"}}}
	expectedCheckpoints := []issue.CheckpointView{{ID: "an-gate"}}
	reader := NewMockIssueReader(gomock.NewController(t))
	reader.EXPECT().ListReadyIssues(gomock.Any(), issue.ListReadyRequest{Limit: 3}).Return(readyIssues, nil)
	reader.EXPECT().ListBlockedIssues(gomock.Any(), issue.ListBlockedRequest{Limit: 4}).Return(blockedIssues, nil)
	reader.EXPECT().ListActionableCheckpoints(gomock.Any()).Return(expectedCheckpoints, nil)
	executor := NewExecutor(NewMockChanges(gomock.NewController(t)), reader)

	ready, err := executor.ListReadyIssues(t.Context(), issue.ListReadyRequest{Limit: 3})
	require.NoError(t, err)
	assert.Equal(t, readyIssues, ready)

	blocked, err := executor.ListBlockedIssues(t.Context(), issue.ListBlockedRequest{Limit: 4})
	require.NoError(t, err)
	assert.Equal(t, blockedIssues, blocked)

	checkpoints, err := executor.ListActionableCheckpoints(t.Context())
	require.NoError(t, err)
	assert.Equal(t, expectedCheckpoints, checkpoints)
}

func executionTestIssue(
	t *testing.T,
	id string,
	status issue.Status,
) issue.State {
	t.Helper()
	lifecycle := status.Lifecycle()
	var claim *issue.ClaimState
	if status == issue.StatusInProgress {
		lifecycle = issue.LifecycleOpen
		claim = &issue.ClaimState{
			Actor:     issue.NewActor("worker"),
			StartedAt: time.Unix(2, 0).UTC(),
		}
	}
	state, err := issue.Load(issue.Snapshot{
		UID:         executionTestUID(t),
		ID:          issue.MustID(id),
		Title:       new(id),
		Kind:        new(issue.KindTask),
		Lifecycle:   new(lifecycle),
		Priority:    new(issue.PriorityNormal),
		ActiveClaim: claim,
		Created:     time.Unix(1, 0).UTC(),
		Updated:     time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)
	return state
}
