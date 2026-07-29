// Package record owns mutable state, immutable log entries, and durable results
// for issues on one board.
package record

import (
	"context"
	"errors"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/must"
)

// ErrIncompleteSnapshot reports a code-owned incomplete record snapshot.
var ErrIncompleteSnapshot = errors.New("incomplete issue record snapshot")

// CommittedRevision identifies the board revision published after a record
// change commits.
type CommittedRevision struct {
	// Revision is shared by every projection in the committed result.
	Revision board.Revision
}

// Changes persists finite issue record changes. Each method owns its complete
// atomic projection boundary.
type Changes interface {
	// SetState replaces or clears one mutable issue state.
	SetState(context.Context, SetState) (StateSet, error)
	// ClearState removes one mutable issue state.
	ClearState(context.Context, ClearState) (StateSet, error)
	// AppendState appends to one mutable issue state.
	AppendState(context.Context, AppendState) (StateAppended, error)
	// CommitState preserves changed State and applies its requested disposition.
	CommitState(context.Context, CommitState) (StateCommitted, error)
	// AddLogEntry appends one immutable attributed log entry.
	AddLogEntry(context.Context, AddLogEntry) (LogEntryAdded, error)
	// SetResult replaces one issue's durable result.
	SetResult(context.Context, SetResult) (ResultSet, error)
}

// Reader supplies the coherent issue records exposed by Recorder reads.
type Reader interface {
	issue.ViewReader

	// ListLogEntries reads one issue's log entries in durable order.
	ListLogEntries(context.Context, issue.LogListRequest) ([]issue.LogEntry, error)

	// ReadResult reads one issue's current durable outcome.
	ReadResult(context.Context, issue.ResultRequest) (issue.Result, error)
}

// Recorder owns caller-facing issue record use cases for one board.
type Recorder struct {
	changes Changes
	issues  Reader
}

// NewRecorder constructs a Recorder from its direct required collaborators.
// It panics when either collaborator is nil because process composition must
// supply both dependencies.
func NewRecorder(changes Changes, issues Reader) *Recorder {
	must.NotBeNilf(changes, "issue record Changes is required")
	must.NotBeNilf(issues, "issue record IssueReader is required")
	return &Recorder{changes: changes, issues: issues}
}

// GetStateRequest identifies the issue whose mutable state should be read.
type GetStateRequest struct {
	// IssueID identifies the issue that owns the state.
	IssueID string
}

// GetStateResult contains one issue's current mutable recovery state.
type GetStateResult struct {
	// IssueID identifies the issue that owns the state.
	IssueID string

	// State is nil when the issue has no current recovery State.
	State *issue.RecoveryState
}

// GetState returns one issue's current mutable recovery state.
func (r *Recorder) GetState(ctx context.Context, req GetStateRequest) (GetStateResult, error) {
	id, err := issue.NewID(req.IssueID)
	if err != nil {
		return GetStateResult{}, err
	}
	view, err := r.readIssue(ctx, id)
	if err != nil {
		return GetStateResult{}, err
	}
	return GetStateResult{
		IssueID: view.Detail.Issue.ID,
		State:   view.Detail.State,
	}, nil
}

// ListLogEntries returns one issue's log entries in durable order.
func (r *Recorder) ListLogEntries(
	ctx context.Context,
	req issue.LogListRequest,
) ([]issue.LogEntry, error) {
	return r.issues.ListLogEntries(ctx, req)
}

// GetResultRequest identifies the issue whose durable result should be read.
type GetResultRequest struct {
	// IssueID identifies the issue that owns the result.
	IssueID string
}

// GetResult returns one issue's current durable outcome.
func (r *Recorder) GetResult(ctx context.Context, req GetResultRequest) (issue.Result, error) {
	return r.issues.ReadResult(ctx, issue.ResultRequest{IssueID: req.IssueID})
}

// Snapshot contains one issue and the transaction values needed to evaluate
// a state, log entry, or result change.
type Snapshot struct {
	// BoardID identifies the board loaded by the writer transaction.
	BoardID board.ID
	// Revision identifies the canonical state observed by the writer.
	Revision board.Revision
	// Issue is the complete current state of the requested issue.
	Issue issue.State
	// OccurredAt timestamps the record and issue update.
	OccurredAt time.Time
}

// Policy evaluates state, log entry, and result changes for one issue.
type Policy struct{ snapshot Snapshot }

// Load validates a record snapshot and restores its policy.
func Load(snapshot Snapshot) (*Policy, error) {
	if snapshot.BoardID == "" || snapshot.Revision.Validate() != nil ||
		snapshot.Issue.ID() == "" || snapshot.OccurredAt.IsZero() {
		return nil, ErrIncompleteSnapshot
	}
	return &Policy{snapshot: snapshot}, nil
}

func (r *Recorder) readIssue(ctx context.Context, id issue.ID) (issue.View, error) {
	return r.issues.ReadIssue(ctx, issue.ReadRequest{IssueID: id.String()})
}
