package execution

import (
	"context"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

// ApproveCheckpoint closes one unresolved checkpoint.
type ApproveCheckpoint struct {
	// IssueID identifies the checkpoint to approve.
	IssueID issue.ID
	// Actor commits changed State before checkpoint closure.
	Actor issue.Actor
	// Reason is optional Markdown rationale persisted with the decision.
	Reason string
}

// DenyCheckpoint cancels one unresolved checkpoint and its dependent closure.
type DenyCheckpoint struct {
	// IssueID identifies the checkpoint to deny.
	IssueID issue.ID
	// Actor commits changed State for every cancelled issue.
	Actor issue.Actor
	// Reason is optional Markdown rationale persisted with the decision.
	Reason string
}

// ApproveCheckpointSnapshot contains the state needed to approve one checkpoint.
type ApproveCheckpointSnapshot struct {
	// BoardID identifies the board loaded by the writer transaction.
	BoardID board.ID
	// Revision identifies the canonical state observed by the writer.
	Revision board.Revision
	// Issue is the checkpoint's complete current state.
	Issue issue.State
	// Prerequisites is the checkpoint's complete readiness dependency set.
	Prerequisites []issue.State
	// TerminalParents contains the checkpoint's direct-parent snapshot.
	TerminalParents []TerminalParentSnapshot
	// OccurredAt timestamps the checkpoint resolution.
	OccurredAt time.Time
}

// DenyCheckpointSnapshot contains the state needed to deny one checkpoint and
// cancel its dependent closure.
type DenyCheckpointSnapshot struct {
	// BoardID identifies the board loaded by the writer transaction.
	BoardID board.ID
	// Revision identifies the canonical state observed by the writer.
	Revision board.Revision
	// Issue is the checkpoint's complete current state.
	Issue issue.State
	// Prerequisites is the checkpoint's complete readiness dependency set.
	Prerequisites []issue.State
	// TransitiveDependents is the closure cancelled by a denial.
	TransitiveDependents []issue.State
	// TerminalParents contains parent state for all candidate transitions.
	TerminalParents []TerminalParentSnapshot
	// OccurredAt timestamps the checkpoint and dependent transitions.
	OccurredAt time.Time
}

type checkpointSnapshot struct {
	BoardID              board.ID
	Revision             board.Revision
	Issue                issue.State
	Prerequisites        []issue.State
	TransitiveDependents []issue.State
	TerminalParents      []TerminalParentSnapshot
	OccurredAt           time.Time
}

// ApproveCheckpointPolicy evaluates an approval against a validated snapshot.
type ApproveCheckpointPolicy struct{ snapshot checkpointSnapshot }

// DenyCheckpointPolicy evaluates a denial against a validated snapshot.
type DenyCheckpointPolicy struct{ snapshot checkpointSnapshot }

// CheckpointResolved is the semantic outcome of a checkpoint decision.
type CheckpointResolved struct {
	// Decision is the immutable checkpoint outcome prepared for persistence.
	Decision issue.CheckpointDecision
	// Issue is the checkpoint state after resolution.
	Issue issue.State
	// Affected contains states cancelled by a denial, including the checkpoint.
	Affected []issue.State
	// ParentsWithoutOpenChildren contains transaction-derived stable IDs.
	ParentsWithoutOpenChildren []issue.ID
	// CommittedRevision is populated after persistence publishes the change.
	CommittedRevision
}

// ResolveCheckpointResult reports one caller-facing checkpoint decision.
type ResolveCheckpointResult struct {
	// Decision is the committed checkpoint decision.
	Decision issue.CheckpointDecisionView
	// Issue is the approved checkpoint, or nil for a denial.
	Issue *issue.Issue
	// Cancelled contains states cancelled by a denial.
	Cancelled []issue.Issue
	// ParentsWithoutOpenChildren contains stable parent IDs from the resolution
	// transaction.
	ParentsWithoutOpenChildren []string
}

// CheckpointRequest supplies one checkpoint decision and optional reason.
type CheckpointRequest struct {
	// IssueID identifies the checkpoint to resolve.
	IssueID string
	// Reason is the optional Markdown rationale for the checkpoint outcome.
	Reason string
}

// LoadApproveCheckpoint validates a checkpoint approval snapshot.
func LoadApproveCheckpoint(
	snapshot ApproveCheckpointSnapshot,
) (*ApproveCheckpointPolicy, error) {
	validated, err := loadCheckpoint(checkpointSnapshot{
		BoardID: snapshot.BoardID, Revision: snapshot.Revision,
		Issue: snapshot.Issue, Prerequisites: snapshot.Prerequisites,
		TerminalParents: snapshot.TerminalParents, OccurredAt: snapshot.OccurredAt,
	})
	if err != nil {
		return nil, err
	}
	return &ApproveCheckpointPolicy{snapshot: validated}, nil
}

// LoadDenyCheckpoint validates a checkpoint denial snapshot.
func LoadDenyCheckpoint(snapshot DenyCheckpointSnapshot) (*DenyCheckpointPolicy, error) {
	validated, err := loadCheckpoint(checkpointSnapshot(snapshot))
	if err != nil {
		return nil, err
	}
	return &DenyCheckpointPolicy{snapshot: validated}, nil
}

func loadCheckpoint(snapshot checkpointSnapshot) (checkpointSnapshot, error) {
	if err := validateIssueSnapshot(snapshot.BoardID, snapshot.Revision, snapshot.Issue); err != nil ||
		snapshot.OccurredAt.IsZero() {
		return checkpointSnapshot{}, ErrIncompleteSnapshot
	}
	candidates := append([]issue.State{snapshot.Issue}, snapshot.TransitiveDependents...)
	if err := validateTerminalParentSnapshots(snapshot.TerminalParents, candidates); err != nil {
		return checkpointSnapshot{}, err
	}
	return snapshot, nil
}

// ApproveCheckpoint applies approval policy to the loaded snapshot.
func (p *ApproveCheckpointPolicy) ApproveCheckpoint(
	command ApproveCheckpoint,
) (CheckpointResolved, error) {
	return resolveCheckpoint(p.snapshot, checkpointResolution{
		IssueID: command.IssueID, Actor: command.Actor,
		Outcome: issue.CheckpointApproved,
		Reason:  command.Reason,
	})
}

// DenyCheckpoint applies denial policy to the loaded snapshot.
func (p *DenyCheckpointPolicy) DenyCheckpoint(
	command DenyCheckpoint,
) (CheckpointResolved, error) {
	return resolveCheckpoint(p.snapshot, checkpointResolution{
		IssueID: command.IssueID, Actor: command.Actor,
		Outcome: issue.CheckpointDenied,
		Reason:  command.Reason,
	})
}

type checkpointResolution struct {
	IssueID issue.ID
	Actor   issue.Actor
	Outcome issue.CheckpointOutcome
	Reason  string
}

func resolveCheckpoint(
	snapshot checkpointSnapshot,
	command checkpointResolution,
) (CheckpointResolved, error) {
	if command.IssueID != snapshot.Issue.ID() {
		return CheckpointResolved{}, ErrIncompleteSnapshot
	}
	if snapshot.Issue.Kind() != issue.KindCheckpoint {
		return CheckpointResolved{}, errkind.Errorf(
			errkind.InvalidInput,
			"checkpoint resolution requires type checkpoint; issue type is %s",
			snapshot.Issue.Kind(),
		)
	}
	if snapshot.Issue.Lifecycle().Terminal() {
		return CheckpointResolved{}, errkind.Errorf(errkind.Conflict, "checkpoint already resolved")
	}
	for _, prerequisite := range snapshot.Prerequisites {
		if prerequisite.Status() != issue.StatusClosed {
			return CheckpointResolved{}, errkind.Errorf(
				errkind.Conflict,
				"checkpoint resolution requires all dependencies to be closed",
			)
		}
	}

	state := snapshot.Issue
	var affected []issue.State
	if command.Outcome == issue.CheckpointApproved {
		var err error
		state, err = terminalState(state, issue.LifecycleClosed, snapshot.OccurredAt)
		if err != nil {
			return CheckpointResolved{}, err
		}
	} else {
		closure := append([]issue.State{state}, snapshot.TransitiveDependents...)
		seen := make(map[issue.ID]struct{}, len(closure))
		for _, current := range closure {
			if _, ok := seen[current.ID()]; ok || current.Lifecycle().Terminal() {
				continue
			}
			seen[current.ID()] = struct{}{}
			cancelled, err := terminalState(
				current,
				issue.LifecycleCancelled,
				snapshot.OccurredAt,
			)
			if err != nil {
				return CheckpointResolved{}, err
			}
			if cancelled.ID() == state.ID() {
				state = cancelled
			}
			affected = append(affected, cancelled)
		}
	}
	return CheckpointResolved{
		Decision: issue.CheckpointDecision{
			Outcome: command.Outcome, Reason: command.Reason,
			DecidedAt: snapshot.OccurredAt,
		},
		Issue: state, Affected: affected,
		ParentsWithoutOpenChildren: parentIDsWithoutOpenChildren(
			snapshot.TerminalParents,
			checkpointTransitions(command.Outcome, state, affected),
		),
	}, nil
}

// ApproveCheckpoint validates caller input and approves one checkpoint.
func (e *Executor) ApproveCheckpoint(
	ctx context.Context,
	invocation issue.Invocation,
	req CheckpointRequest,
) (ResolveCheckpointResult, error) {
	id, err := issue.NewID(req.IssueID)
	if err != nil {
		return ResolveCheckpointResult{}, err
	}
	outcome, err := e.changes.ApproveCheckpoint(
		ctx,
		ApproveCheckpoint{
			IssueID: id,
			Actor:   issue.NewActor(invocation.Actor()),
			Reason:  req.Reason,
		},
	)
	return checkpointResult(outcome, err)
}

// DenyCheckpoint validates caller input and denies one checkpoint.
func (e *Executor) DenyCheckpoint(
	ctx context.Context,
	invocation issue.Invocation,
	req CheckpointRequest,
) (ResolveCheckpointResult, error) {
	id, err := issue.NewID(req.IssueID)
	if err != nil {
		return ResolveCheckpointResult{}, err
	}
	outcome, err := e.changes.DenyCheckpoint(
		ctx,
		DenyCheckpoint{
			IssueID: id,
			Actor:   issue.NewActor(invocation.Actor()),
			Reason:  req.Reason,
		},
	)
	return checkpointResult(outcome, err)
}

func checkpointResult(
	outcome CheckpointResolved,
	err error,
) (ResolveCheckpointResult, error) {
	if err != nil {
		return ResolveCheckpointResult{}, err
	}
	var resolved *issue.Issue
	if outcome.Decision.Outcome == issue.CheckpointApproved && outcome.Issue.ID() != "" {
		value := issueProjection(outcome.Issue, outcome.Revision)
		resolved = &value
	}
	return ResolveCheckpointResult{
		Decision: issue.CheckpointDecisionView{
			Outcome:   outcome.Decision.Outcome.String(),
			Reason:    outcome.Decision.Reason,
			DecidedAt: outcome.Decision.DecidedAt.Unix(),
			Revision:  outcome.Decision.Revision,
		},
		Issue:                      resolved,
		Cancelled:                  issueProjections(outcome.Affected, outcome.Revision),
		ParentsWithoutOpenChildren: parentIDStrings(outcome.ParentsWithoutOpenChildren),
	}, nil
}

func checkpointTransitions(
	outcome issue.CheckpointOutcome,
	state issue.State,
	affected []issue.State,
) []issue.State {
	if outcome == issue.CheckpointApproved {
		return []issue.State{state}
	}
	return affected
}
