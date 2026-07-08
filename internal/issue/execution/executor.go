// Package execution owns issue eligibility, custody, terminal lifecycle
// transitions, and checkpoint resolution for one board.
package execution

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/must"
)

// ErrIncompleteSnapshot reports a code-owned incomplete execution snapshot.
var ErrIncompleteSnapshot = errors.New("incomplete board execution snapshot")

// CommittedRevision identifies the canonical board revision published after
// an execution change commits.
type CommittedRevision struct {
	// Revision is shared by every projection in the committed result.
	Revision board.Revision
}

// Changes executes finite issue execution operations against one board. Each
// method owns its complete atomic projection boundary.
type Changes interface {
	// ClaimIssue persists an explicit claim.
	ClaimIssue(context.Context, ClaimIssue) (IssueClaimed, error)
	// ClaimNext selects and persists the next eligible claim atomically.
	ClaimNext(context.Context, ClaimNext) (IssueClaimed, error)
	// ReleaseIssue relinquishes execution custody.
	ReleaseIssue(context.Context, ReleaseIssue) (IssueReleased, error)
	// CloseIssue persists successful completion.
	CloseIssue(context.Context, CloseIssue) (IssueClosed, error)
	// ReopenIssue returns a terminal issue to the open lifecycle.
	ReopenIssue(context.Context, ReopenIssue) (IssueReopened, error)
	// CancelIssues terminates requested roots and their dependent closure.
	CancelIssues(context.Context, CancelIssues) (IssuesCancelled, error)
	// ApproveCheckpoint persists an approved checkpoint decision.
	ApproveCheckpoint(context.Context, ApproveCheckpoint) (CheckpointResolved, error)
	// DenyCheckpoint persists a denied checkpoint and its cancellation closure.
	DenyCheckpoint(context.Context, DenyCheckpoint) (CheckpointResolved, error)
}

// IssueReader supplies post-commit issue projections required by execution
// results.
type IssueReader interface {
	// ReadIssue returns one issue view from the Executor's board.
	ReadIssue(context.Context, issue.ReadRequest) (issue.View, error)

	// ListReadyIssues returns claimable executable issues in domain order.
	ListReadyIssues(context.Context, issue.ListReadyRequest) ([]issue.Summary, error)

	// ListBlockedIssues returns unfinished issues with unresolved prerequisites.
	ListBlockedIssues(context.Context, issue.ListBlockedRequest) ([]issue.Summary, error)

	// ListActionableCheckpoints returns checkpoints ready for human resolution.
	ListActionableCheckpoints(context.Context) ([]issue.CheckpointView, error)
}

// Executor owns caller-facing issue execution use cases for one board.
type Executor struct {
	changes Changes
	issues  IssueReader
}

// NewExecutor constructs an Executor from its direct required collaborators.
// It panics when either collaborator is nil because process composition must
// supply both dependencies.
func NewExecutor(changes Changes, issues IssueReader) *Executor {
	must.NotBeNilf(changes, "issue execution Changes is required")
	must.NotBeNilf(issues, "issue execution IssueReader is required")
	return &Executor{changes: changes, issues: issues}
}

func (e *Executor) readIssue(
	ctx context.Context,
	id issue.ID,
	contextDepth *int,
) (issue.View, error) {
	return e.issues.ReadIssue(ctx, issue.ReadRequest{
		IssueID: id.String(), ContextDepth: contextDepth,
	})
}

// ListReadyIssues returns claimable executable issues in domain order.
func (e *Executor) ListReadyIssues(
	ctx context.Context,
	req issue.ListReadyRequest,
) ([]issue.Summary, error) {
	return e.issues.ListReadyIssues(ctx, req)
}

// ListBlockedIssues returns unfinished issues with unresolved prerequisites.
func (e *Executor) ListBlockedIssues(
	ctx context.Context,
	req issue.ListBlockedRequest,
) ([]issue.Summary, error) {
	return e.issues.ListBlockedIssues(ctx, req)
}

// ListActionableCheckpoints returns checkpoints ready for human resolution.
func (e *Executor) ListActionableCheckpoints(
	ctx context.Context,
) ([]issue.CheckpointView, error) {
	return e.issues.ListActionableCheckpoints(ctx)
}

func validateSnapshot(boardID board.ID, revision board.Revision) error {
	if boardID == "" {
		return ErrIncompleteSnapshot
	}
	if err := revision.Validate(); err != nil {
		return ErrIncompleteSnapshot
	}
	return nil
}

func validateIssueSnapshot(
	boardID board.ID,
	revision board.Revision,
	state issue.State,
) error {
	if err := validateSnapshot(boardID, revision); err != nil || state.ID() == "" {
		return ErrIncompleteSnapshot
	}
	return nil
}
