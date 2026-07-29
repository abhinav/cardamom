package execution

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestExecutorDispatchesClaimAndReleaseAttribution(t *testing.T) {
	t.Parallel()

	state := executionTestIssue(t, "an-1", issue.StatusInProgress)
	changes := &executorChangesStub{
		claimOutcome:   IssueClaimed{Issue: state},
		releaseOutcome: IssueReleased{Issue: state},
	}
	reader := &executorIssueReaderStub{view: issue.View{Detail: issue.Detail{
		Issue: issue.Issue{ID: "an-1"},
	}}}
	executor := NewExecutor(changes, reader)

	_, err := executor.ClaimIssue(t.Context(), issue.NewInvocation("worker"), ClaimIssueRequest{
		ID: "an-1", Assignee: "worker",
	})
	require.NoError(t, err)
	assert.Equal(t, issue.MustID("an-1"), changes.claimIssue.IssueID)
	assert.Equal(t, issue.NewActor("worker"), changes.claimIssue.Assignee)

	_, err = executor.ReleaseIssue(
		t.Context(),
		issue.NewInvocation("worker"),
		ReleaseIssueRequest{ID: "an-1"},
	)
	require.NoError(t, err)
	assert.Equal(t, issue.NewActor("worker"), changes.releaseIssue.Actor)
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
		changes := &executorChangesStub{
			claimNextErrors: []error{errNoReady, nil},
			claimOutcome:    IssueClaimed{Issue: state},
		}
		depth := 2
		reader := &executorIssueReaderStub{view: issue.View{
			Detail: issue.Detail{Issue: issue.Issue{ID: "an-1"}},
			Context: &issue.Context{
				Ancestors: []issue.ContextEntry{},
			},
		}}
		executor := NewExecutor(changes, reader)

		result, err := executor.ClaimNext(t.Context(), issue.NewInvocation("worker"), ClaimNextRequest{
			Assignee: "worker", Watch: true, Interval: time.Hour,
			ContextDepth: &depth, UnderID: "an-parent",
			LabelsAll:  []string{"area:execution"},
			LabelsAny:  []string{"phase:a", "phase:b"},
			LabelsNone: []string{"paused"},
		})
		require.NoError(t, err)
		assert.Equal(t, 2, changes.claimNextCalls)
		assert.Equal(t, reader.view, result.Issue)
		assert.Equal(t, issue.MustID("an-parent"), changes.claimNext.UnderID)
		assert.Equal(t, []issue.Label{issue.MustLabel("area:execution")}, changes.claimNext.LabelsAll)
		assert.Equal(t, []issue.Label{
			issue.MustLabel("phase:a"),
			issue.MustLabel("phase:b"),
		}, changes.claimNext.LabelsAny)
		assert.Equal(t, []issue.Label{issue.MustLabel("paused")}, changes.claimNext.LabelsNone)
		assert.Equal(t, issue.ReadRequest{
			IssueID: "an-1", ContextDepth: &depth,
		}, reader.request)
	})
}

func TestExecutorClaimNextRejectsInvalidLabelSelectors(t *testing.T) {
	t.Parallel()

	changes := new(executorChangesStub)
	executor := NewExecutor(changes, new(executorIssueReaderStub))

	_, err := executor.ClaimNext(
		t.Context(),
		issue.NewInvocation("worker"),
		ClaimNextRequest{
			Assignee:   "worker",
			LabelsNone: []string{"-paused"},
		},
	)
	require.ErrorContains(t, err, "label cannot start with + or -")
	assert.Zero(t, changes.claimNextCalls)
}

func TestExecutorExposesEligibilityReads(t *testing.T) {
	t.Parallel()

	reader := &executorIssueReaderStub{
		ready:       []issue.Summary{{Issue: issue.Issue{ID: "an-ready"}}},
		blocked:     []issue.Summary{{Issue: issue.Issue{ID: "an-blocked"}}},
		checkpoints: []issue.CheckpointView{{Issue: issue.Issue{ID: "an-gate"}}},
	}
	executor := NewExecutor(new(executorChangesStub), reader)

	ready, err := executor.ListReadyIssues(t.Context(), issue.ListReadyRequest{Limit: 3})
	require.NoError(t, err)
	assert.Equal(t, reader.ready, ready)

	blocked, err := executor.ListBlockedIssues(t.Context(), issue.ListBlockedRequest{Limit: 4})
	require.NoError(t, err)
	assert.Equal(t, reader.blocked, blocked)

	checkpoints, err := executor.ListActionableCheckpoints(t.Context())
	require.NoError(t, err)
	assert.Equal(t, reader.checkpoints, checkpoints)
}

type executorChangesStub struct {
	claimIssue      ClaimIssue
	claimNext       ClaimNext
	releaseIssue    ReleaseIssue
	claimOutcome    IssueClaimed
	releaseOutcome  IssueReleased
	claimNextErrors []error
	claimNextCalls  int
}

func (s *executorChangesStub) ClaimIssue(
	_ context.Context,
	command ClaimIssue,
) (IssueClaimed, error) {
	s.claimIssue = command
	return s.claimOutcome, nil
}

func (s *executorChangesStub) ClaimNext(
	_ context.Context,
	command ClaimNext,
) (IssueClaimed, error) {
	s.claimNext = command
	call := s.claimNextCalls
	s.claimNextCalls++
	if call < len(s.claimNextErrors) && s.claimNextErrors[call] != nil {
		return IssueClaimed{}, s.claimNextErrors[call]
	}
	return s.claimOutcome, nil
}

func (s *executorChangesStub) ReleaseIssue(
	_ context.Context,
	command ReleaseIssue,
) (IssueReleased, error) {
	s.releaseIssue = command
	return s.releaseOutcome, nil
}

func (*executorChangesStub) CloseIssue(
	context.Context,
	CloseIssue,
) (IssueClosed, error) {
	return IssueClosed{}, nil
}

func (*executorChangesStub) ReopenIssue(
	context.Context,
	ReopenIssue,
) (IssueReopened, error) {
	return IssueReopened{}, nil
}

func (*executorChangesStub) CancelIssues(
	context.Context,
	CancelIssues,
) (IssuesCancelled, error) {
	return IssuesCancelled{}, nil
}

func (*executorChangesStub) ApproveCheckpoint(
	context.Context,
	ApproveCheckpoint,
) (CheckpointResolved, error) {
	return CheckpointResolved{}, nil
}

func (*executorChangesStub) DenyCheckpoint(
	context.Context,
	DenyCheckpoint,
) (CheckpointResolved, error) {
	return CheckpointResolved{}, nil
}

type executorIssueReaderStub struct {
	request     issue.ReadRequest
	view        issue.View
	ready       []issue.Summary
	blocked     []issue.Summary
	checkpoints []issue.CheckpointView
}

func (s *executorIssueReaderStub) ReadIssue(
	_ context.Context,
	request issue.ReadRequest,
) (issue.View, error) {
	s.request = request
	return s.view, nil
}

func (s *executorIssueReaderStub) ListReadyIssues(
	context.Context,
	issue.ListReadyRequest,
) ([]issue.Summary, error) {
	return s.ready, nil
}

func (s *executorIssueReaderStub) ListBlockedIssues(
	context.Context,
	issue.ListBlockedRequest,
) ([]issue.Summary, error) {
	return s.blocked, nil
}

func (s *executorIssueReaderStub) ListActionableCheckpoints(
	context.Context,
) ([]issue.CheckpointView, error) {
	return s.checkpoints, nil
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
		ID:          issue.MustID(id),
		Title:       id,
		Kind:        issue.KindTask,
		Lifecycle:   lifecycle,
		Priority:    issue.PriorityNormal,
		ActiveClaim: claim,
		Created:     time.Unix(1, 0).UTC(),
		Updated:     time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)
	return state
}
