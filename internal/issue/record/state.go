package record

import (
	"context"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

// SetState replaces one issue's mutable State.
type SetState struct {
	// IssueID identifies the issue whose state changes.
	IssueID issue.ID
	// Author is the normalized actor replacing the State.
	Author issue.Actor
	// Text is the complete replacement Markdown source.
	Text string
	// NextAction is the optional planned transition from Text.
	NextAction string
}

// ClearState removes one issue's mutable State.
type ClearState struct {
	// IssueID identifies the issue whose State is removed.
	IssueID issue.ID
}

// AppendState appends text to one issue's mutable state.
type AppendState struct {
	// IssueID identifies the issue whose state changes.
	IssueID issue.ID
	// Author is the normalized actor appending to the State.
	Author issue.Actor
	// Text is the Markdown source appended to the existing state.
	Text string
}

// StateSet is the semantic outcome of replacing one state.
type StateSet struct {
	// Issue is the changed issue state.
	Issue issue.State
	// State is the complete state after the change.
	State string
	// CommittedRevision is populated after persistence publishes the change.
	CommittedRevision
}

// StateAppended is the semantic outcome of appending to one state.
type StateAppended struct {
	// Issue is the changed issue state.
	Issue issue.State
	// State is the complete state after the append.
	State string
	// Changed reports whether persistence must replace the returned state.
	Changed bool
	// CommittedRevision is populated after persistence publishes the change.
	CommittedRevision
}

// StateResult reports the committed issue after a state change.
type StateResult struct {
	// Issue is the caller-facing issue projection after the change.
	Issue issue.Issue
}

// SetStateRequest supplies caller input for replacing or appending a state.
type SetStateRequest struct {
	// IssueID identifies the issue whose state changes.
	IssueID string
	// Text is the replacement or appended Markdown source.
	Text string
	// NextAction is the optional planned transition for replacement.
	NextAction string
}

// ClearStateRequest identifies the issue whose state is cleared.
type ClearStateRequest struct {
	// IssueID identifies the issue whose state is cleared.
	IssueID string
}

// SetState applies replacement policy to the loaded snapshot.
func (p *Policy) SetState(command SetState) (StateSet, error) {
	if command.IssueID != p.snapshot.Issue.ID() {
		return StateSet{}, ErrIncompleteSnapshot
	}
	recovery, err := issue.NewRecoveryState(
		command.Text,
		command.NextAction,
		command.Author,
		p.snapshot.OccurredAt,
	)
	if err != nil {
		return StateSet{}, err
	}
	state := p.snapshot.Issue.WithRecoveryState(recovery, p.snapshot.OccurredAt)
	return StateSet{
		Issue: state,
		State: state.RecoveryState(),
	}, nil
}

// ClearState applies discard policy to the loaded snapshot.
func (p *Policy) ClearState(command ClearState) (StateSet, error) {
	if command.IssueID != p.snapshot.Issue.ID() {
		return StateSet{}, ErrIncompleteSnapshot
	}
	state := p.snapshot.Issue.WithRecoveryState(nil, p.snapshot.OccurredAt)
	return StateSet{Issue: state}, nil
}

// AppendState applies append policy to the loaded snapshot.
func (p *Policy) AppendState(command AppendState) (StateAppended, error) {
	if command.IssueID != p.snapshot.Issue.ID() {
		return StateAppended{}, ErrIncompleteSnapshot
	}
	if command.Text == "" {
		return StateAppended{
			Issue: p.snapshot.Issue, State: p.snapshot.Issue.RecoveryState(),
		}, nil
	}
	if command.Author == "" {
		return StateAppended{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: state author required",
		)
	}
	current := p.snapshot.Issue.RecoveryStateRecord()
	recoveryState := command.Text
	nextAction := ""
	if current != nil {
		recoveryState = current.Body + "\n" + command.Text
		nextAction = current.NextAction
	}
	recovery, err := issue.NewRecoveryState(
		recoveryState,
		nextAction,
		command.Author,
		p.snapshot.OccurredAt,
	)
	if err != nil {
		return StateAppended{}, err
	}
	state := p.snapshot.Issue.WithRecoveryState(recovery, p.snapshot.OccurredAt)
	return StateAppended{
		Issue:   state,
		State:   state.RecoveryState(),
		Changed: true,
	}, nil
}

// SetState validates caller input and replaces one state.
func (r *Recorder) SetState(
	ctx context.Context,
	invocation issue.Invocation,
	request SetStateRequest,
) (StateResult, error) {
	id, err := issue.NewID(request.IssueID)
	if err != nil {
		return StateResult{}, err
	}
	outcome, err := r.changes.SetState(
		ctx,
		SetState{
			IssueID:    id,
			Author:     issue.NewActor(invocation.Actor()),
			Text:       request.Text,
			NextAction: request.NextAction,
		},
	)
	if err != nil {
		return StateResult{}, err
	}
	view, err := r.readIssue(ctx, outcome.Issue.ID())
	if err != nil {
		return StateResult{}, err
	}
	return StateResult{Issue: view.Detail.Issue}, nil
}

// ClearState validates caller input and clears one state.
func (r *Recorder) ClearState(
	ctx context.Context,
	_ issue.Invocation,
	request ClearStateRequest,
) (StateResult, error) {
	id, err := issue.NewID(request.IssueID)
	if err != nil {
		return StateResult{}, err
	}
	outcome, err := r.changes.ClearState(ctx, ClearState{IssueID: id})
	if err != nil {
		return StateResult{}, err
	}
	view, err := r.readIssue(ctx, outcome.Issue.ID())
	if err != nil {
		return StateResult{}, err
	}
	return StateResult{Issue: view.Detail.Issue}, nil
}

// AppendState validates caller input and appends to one state.
func (r *Recorder) AppendState(
	ctx context.Context,
	invocation issue.Invocation,
	request SetStateRequest,
) (StateResult, error) {
	id, err := issue.NewID(request.IssueID)
	if err != nil {
		return StateResult{}, err
	}
	outcome, err := r.changes.AppendState(
		ctx,
		AppendState{
			IssueID: id,
			Author:  issue.NewActor(invocation.Actor()),
			Text:    request.Text,
		},
	)
	if err != nil {
		return StateResult{}, err
	}
	view, err := r.readIssue(ctx, outcome.Issue.ID())
	if err != nil {
		return StateResult{}, err
	}
	return StateResult{Issue: view.Detail.Issue}, nil
}

// CommitStateDisposition selects the State retained after a commit.
type CommitStateDisposition int

const (
	// CommitStateRetain preserves current State after committing it.
	CommitStateRetain CommitStateDisposition = iota

	// CommitStateReplace installs the command's next State.
	CommitStateReplace

	// CommitStateClear removes State after committing it.
	CommitStateClear
)

// StateReplacement is the complete State installed after an atomic commit.
type StateReplacement struct {
	// Body is the nonblank replacement recovery position.
	Body string

	// NextAction is the optional planned transition from Body.
	NextAction string
}

// CommitState snapshots changed State and applies one atomic disposition.
type CommitState struct {
	// IssueID identifies the issue whose State is committed.
	IssueID issue.ID

	// Committer is the actor preserving the State in immutable history.
	Committer issue.Actor

	// Disposition selects retained, replacement, or absent resulting State.
	Disposition CommitStateDisposition

	// Replacement is required only for CommitStateReplace.
	Replacement StateReplacement
}

// StateCommitted is the semantic outcome of one State commit.
type StateCommitted struct {
	// Issue is the State after the selected disposition.
	Issue issue.State

	// LogEntry is nil when current State was already linked or absent.
	LogEntry *LogEntry

	// Changed reports whether State or Log persistence must change.
	Changed bool

	// CommittedRevision is populated after persistence publishes a change.
	CommittedRevision
}

// CommitStateRequest supplies caller input for one explicit State commit.
type CommitStateRequest struct {
	// IssueID identifies the issue whose State is committed.
	IssueID string

	// Disposition selects retained, replacement, or absent resulting State.
	Disposition CommitStateDisposition

	// Replacement is required only for CommitStateReplace.
	Replacement StateReplacement
}

// CommitStateResult reports one explicit State commit.
type CommitStateResult struct {
	// Issue is the caller-facing issue projection after the commit.
	Issue issue.Issue

	// State is the complete resulting mutable State.
	State *issue.RecoveryState

	// LogEntry is the newly created snapshot, when present.
	LogEntry *issue.LogEntry
}

// CommitState applies State commit policy to the loaded snapshot.
func (p *Policy) CommitState(command CommitState) (StateCommitted, error) {
	if command.IssueID != p.snapshot.Issue.ID() {
		return StateCommitted{}, ErrIncompleteSnapshot
	}

	current := p.snapshot.Issue.RecoveryStateRecord()
	out := StateCommitted{Issue: p.snapshot.Issue}
	if current != nil && current.SnapshotLogEntryID == nil {
		if command.Committer == "" {
			return StateCommitted{}, errkind.Errorf(
				errkind.InvalidInput,
				"invalid input: state committer required",
			)
		}
		out.LogEntry = &LogEntry{
			IssueID:    command.IssueID,
			Kind:       issue.LogEntryKindStateSnapshot,
			Author:     current.Author,
			Committer:  command.Committer,
			Body:       current.Body,
			NextAction: current.NextAction,
			Created:    new(p.snapshot.OccurredAt),
		}
		out.Changed = true
	}

	switch command.Disposition {
	case CommitStateRetain:
	case CommitStateReplace:
		if command.Committer == "" {
			return StateCommitted{}, errkind.Errorf(
				errkind.InvalidInput,
				"invalid input: state committer required",
			)
		}
		next, err := issue.NewRecoveryState(
			command.Replacement.Body,
			command.Replacement.NextAction,
			command.Committer,
			p.snapshot.OccurredAt,
		)
		if err != nil {
			return StateCommitted{}, err
		}
		out.Issue = p.snapshot.Issue.WithRecoveryState(
			next,
			p.snapshot.OccurredAt,
		)
		out.Changed = true
	case CommitStateClear:
		if current != nil {
			out.Issue = p.snapshot.Issue.WithRecoveryState(
				nil,
				p.snapshot.OccurredAt,
			)
			out.Changed = true
		}
	default:
		return StateCommitted{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: unknown State commit disposition %d",
			command.Disposition,
		)
	}
	return out, nil
}

// CommitState validates caller input and atomically commits one State.
func (r *Recorder) CommitState(
	ctx context.Context,
	invocation issue.Invocation,
	request CommitStateRequest,
) (CommitStateResult, error) {
	id, err := issue.NewID(request.IssueID)
	if err != nil {
		return CommitStateResult{}, err
	}
	outcome, err := r.changes.CommitState(ctx, CommitState{
		IssueID:     id,
		Committer:   issue.NewActor(invocation.Actor()),
		Disposition: request.Disposition,
		Replacement: request.Replacement,
	})
	if err != nil {
		return CommitStateResult{}, err
	}
	view, err := r.readIssue(ctx, outcome.Issue.ID())
	if err != nil {
		return CommitStateResult{}, err
	}
	result := CommitStateResult{
		Issue: view.Detail.Issue,
		State: view.Detail.State,
	}
	if outcome.LogEntry != nil {
		entry := logEntryProjection(*outcome.LogEntry)
		result.LogEntry = &entry
	}
	return result, nil
}
