package boardconflict

import (
	"context"
	"fmt"
	"time"

	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

// ResolveExecution chooses a compatible execution tuple from reviewed values.
// Kind and Lifecycle may change known or implicated fields; they cannot replace
// an unrelated scalar conflict through this operation.
type ResolveExecution struct {
	// ConflictID identifies the assignment set the caller reviewed.
	ConflictID string // required

	// Waiting explicitly retains or clears the contributing waiting assignment.
	Waiting WaitingChoice // required

	// WaitingAssignment selects a positive retained waiting value when several exist.
	// It is omitted when keeping the sole positive value or clearing waiting.
	WaitingAssignment AssignmentID

	// Kind supplies a concrete replacement; nil keeps the retained or known kind.
	Kind *issue.Kind

	// Lifecycle supplies a concrete replacement; nil keeps its retained or known value.
	// A new terminal lifecycle receives OccurredAt as its closure time.
	Lifecycle *issue.Lifecycle

	// OccurredAt attributes newly authored assignments and closure times.
	OccurredAt time.Time // required
}

// WaitingChoice is the finite waiting decision needed to repair execution state.
type WaitingChoice uint8

const (
	_ WaitingChoice = iota
	// KeepWaiting retains the reviewed reason and waiting time.
	KeepWaiting
	// ClearWaiting removes the reviewed waiting state.
	ClearWaiting
)

// ExecutionSnapshot supplies current scalars and access to retained contributors.
// Repair loads only values needed by the chosen execution state. Nil unrelated
// scalar fields stay unresolved throughout the repair.
type ExecutionSnapshot struct {
	// Reader loads only contributors consumed by this repair from the same snapshot.
	Reader ExecutionAssignmentReader // required

	// Kind is the current supported kind, or nil when it has no current value.
	Kind *issue.Kind

	// Lifecycle is the current supported lifecycle, or nil when unresolved.
	Lifecycle *issue.Lifecycle
}

// ResolveExecution repairs waiting compatibility without choosing default values.
// It returns ordinary changed assignments separately from unchanged retained ones.
// The repository must also validate any chosen kind/lifecycle against board
// relationships before publishing this repair in its transaction.
func (c *ConstraintConflict) ResolveExecution(ctx context.Context, request ResolveExecution, snapshot ExecutionSnapshot) (ConstraintRepair, error) {
	if err := c.requireReview(request.ConflictID, ConstraintExecution); err != nil {
		return ConstraintRepair{}, err
	}
	var kinds, lifecycles, waitingValues []AssignmentReference
	var uid issue.UID
	for _, assignment := range c.assignments {
		uid = assignment.Unit.IssueUID
		switch assignment.Unit.Kind {
		case boardhistory.EventIssueKindSet:
			kinds = append(kinds, assignment)
		case boardhistory.EventIssueLifecycleSet:
			lifecycles = append(lifecycles, assignment)
		case boardhistory.EventIssueWaitingSet:
			waitingValues = append(waitingValues, assignment)
		}
	}
	if len(kinds) > 1 && request.Kind == nil {
		return ConstraintRepair{}, errkind.Errorf(errkind.InvalidInput, "choose a concrete kind to resolve its contributing values")
	}
	if len(lifecycles) > 1 && request.Lifecycle == nil {
		return ConstraintRepair{}, errkind.Errorf(errkind.InvalidInput, "choose a concrete lifecycle to resolve its contributing values")
	}
	kind, lifecycle := snapshot.Kind, snapshot.Lifecycle
	var retainedKind, retainedLifecycle, retainedWaiting *RetainedAssignment
	if len(kinds) == 1 {
		value, err := readExecutionAssignment(ctx, snapshot.Reader, kinds[0])
		if err != nil {
			return ConstraintRepair{}, err
		}
		parsed, err := issue.NewKind(value.Value.(*boardhistory.IssueKindSet).Kind)
		if err != nil {
			return ConstraintRepair{}, fmt.Errorf("retained kind: %w", err)
		}
		kind, retainedKind = &parsed, &value
	}
	if len(lifecycles) == 1 {
		value, err := readExecutionAssignment(ctx, snapshot.Reader, lifecycles[0])
		if err != nil {
			return ConstraintRepair{}, err
		}
		parsed, err := issue.NewLifecycle(value.Value.(*boardhistory.IssueLifecycleSet).Lifecycle)
		if err != nil {
			return ConstraintRepair{}, fmt.Errorf("retained lifecycle: %w", err)
		}
		lifecycle, retainedLifecycle = &parsed, &value
	}
	var kindChange *boardhistory.IssueKindSet
	var lifecycleChange *boardhistory.IssueLifecycleSet
	var waitingChange *boardhistory.IssueWaitingSet
	if request.Kind != nil {
		if len(kinds) == 0 && kind == nil {
			return ConstraintRepair{}, errkind.Errorf(errkind.Conflict, "resolve the separate kind conflict before replacing its value")
		}
		if len(kinds) > 1 || kind == nil || *request.Kind != *kind {
			kindChange = &boardhistory.IssueKindSet{IssueUID: uid, Kind: request.Kind.String()}
		}
		kind = request.Kind
	}
	if request.Lifecycle != nil {
		if len(lifecycles) == 0 && lifecycle == nil {
			return ConstraintRepair{}, errkind.Errorf(errkind.Conflict, "resolve the separate lifecycle conflict before replacing its value")
		}
		if len(lifecycles) > 1 || lifecycle == nil || *request.Lifecycle != *lifecycle {
			var closedAt *time.Time
			if request.Lifecycle.Terminal() {
				closedAt = new(request.OccurredAt)
			}
			lifecycleChange = &boardhistory.IssueLifecycleSet{IssueUID: uid, Lifecycle: request.Lifecycle.String(), ClosedAt: closedAt}
		}
		lifecycle = request.Lifecycle
	}
	selectedWaiting, err := chooseConstraintWaiting(request, waitingValues)
	if err != nil {
		return ConstraintRepair{}, err
	}
	// Unknown unrelated scalar values are not evidence of incompatibility.
	// Keeping waiting only fails when a known kind or lifecycle forbids it.
	if request.Waiting == KeepWaiting && (kind != nil && !kind.Executable() || lifecycle != nil && *lifecycle != issue.LifecycleOpen) {
		return ConstraintRepair{}, errkind.Errorf(errkind.InvalidInput, "waiting requires an executable kind and open lifecycle; choose compatible values or clear waiting")
	}
	var waiting *issue.WaitingState
	if request.Waiting == KeepWaiting {
		value, err := readExecutionAssignment(ctx, snapshot.Reader, *selectedWaiting)
		if err != nil {
			return ConstraintRepair{}, err
		}
		waiting, retainedWaiting = value.Value.(*boardhistory.IssueWaitingSet).Waiting, &value
	}
	if len(waitingValues) > 1 || request.Waiting == ClearWaiting {
		waitingChange = &boardhistory.IssueWaitingSet{IssueUID: uid, Waiting: waiting}
	}
	var changed []boardhistory.Payload
	if kindChange != nil {
		changed = append(changed, kindChange)
	}
	if lifecycleChange != nil {
		changed = append(changed, lifecycleChange)
	}
	if waitingChange != nil {
		changed = append(changed, waitingChange)
	}
	var repair ConstraintRepair
	for _, value := range changed {
		event, err := constraintEvent(value, request.OccurredAt)
		if err != nil {
			return ConstraintRepair{}, err
		}
		repair.Author = append(repair.Author, event)
	}
	if kindChange == nil && retainedKind != nil {
		repair.Restore = append(repair.Restore, *retainedKind)
	}
	if lifecycleChange == nil && retainedLifecycle != nil {
		repair.Restore = append(repair.Restore, *retainedLifecycle)
	}
	if waitingChange == nil && retainedWaiting != nil {
		repair.Restore = append(repair.Restore, *retainedWaiting)
	}
	return repair, nil
}

// ExecutionAssignmentReader loads a reviewed contributor from retained history.
// Implementations project the requested unit from its originating event and
// return its supported typed payload. The repair does not modify that value.
// Calls remain in the coherent snapshot supplying the conflict metadata.
type ExecutionAssignmentReader interface {
	ReadExecutionAssignment(context.Context, AssignmentReference) (boardhistory.Payload, error)
}

func readExecutionAssignment(ctx context.Context, reader ExecutionAssignmentReader, assignment AssignmentReference) (RetainedAssignment, error) {
	value, err := reader.ReadExecutionAssignment(ctx, assignment)
	if err != nil {
		return RetainedAssignment{}, fmt.Errorf("read execution assignment %q: %w", assignment.AlternativeID, err)
	}
	return RetainedAssignment{AlternativeID: assignment.AlternativeID, Value: value}, nil
}

// chooseConstraintWaiting interprets only an explicit waiting decision.
// Presence metadata identifies a sole positive contender without reading its
// reason. Several positive values require review by assignment ID.
// Clearing waiting returns nil because no retained value is selected.
func chooseConstraintWaiting(request ResolveExecution, values []AssignmentReference) (*AssignmentReference, error) {
	if request.Waiting == ClearWaiting {
		if request.WaitingAssignment != "" {
			return nil, errkind.Errorf(errkind.InvalidInput, "clearing waiting does not select a retained waiting value")
		}
		return nil, nil
	}
	if request.Waiting != KeepWaiting {
		return nil, errkind.Errorf(errkind.InvalidInput, "choose whether to keep or clear waiting")
	}
	var selected AssignmentReference
	positiveCount := 0
	for _, assignment := range values {
		if request.WaitingAssignment != "" {
			if assignment.AlternativeID != request.WaitingAssignment {
				continue
			}
			if !assignment.WaitingPresent {
				return nil, errkind.Errorf(errkind.InvalidInput, "selected waiting assignment clears waiting; choose clear waiting")
			}
			return &assignment, nil
		}
		if assignment.WaitingPresent {
			positiveCount++
			selected = assignment
		}
	}
	if request.WaitingAssignment != "" {
		return nil, errkind.Errorf(errkind.Conflict, "waiting assignment %q is not in the reviewed constraint", request.WaitingAssignment)
	}
	if positiveCount != 1 {
		return nil, errkind.Errorf(errkind.InvalidInput, "select one reviewed positive waiting assignment")
	}
	return &selected, nil
}
