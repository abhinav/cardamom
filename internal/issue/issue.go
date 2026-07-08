// Package issue owns board-scoped issue behavior, caller operations, and the
// contracts implemented by issue persistence.
package issue

import (
	"strings"
	"time"
	"unicode"

	"go.abhg.dev/cardamom/internal/errkind"
)

// Snapshot is the complete persisted representation needed to load
// one issue. It contains semantic values only and carries no storage metadata.
type Snapshot struct {
	ID    ID
	Title string
	Kind  Kind
	// Lifecycle is the persisted open, closed, or cancelled state.
	Lifecycle Lifecycle
	Priority  Priority
	// ActiveClaim is nil when no actor has current execution custody.
	ActiveClaim   *ClaimState
	Created       time.Time
	Updated       time.Time
	ClosedAt      *time.Time
	Waiting       *WaitingState
	Summary       string
	Details       string
	RecoveryState *RecoveryState
	Result        string
}

// RecoveryState is one issue's mutable recovery position and optional
// transition.
//
// Author and UpdatedAt are both absent when attribution is unavailable.
// SnapshotLogEntryID links this version to immutable Log history.
type RecoveryState struct {
	// Body is the nonblank Markdown recovery position.
	Body string

	// NextAction is the optional nonblank Markdown transition from Body.
	NextAction string

	// Author is the actor that last changed the State, or empty when unknown.
	Author Actor

	// UpdatedAt is absent when the State update time is unknown.
	UpdatedAt *time.Time

	// SnapshotLogEntryID identifies matching committed State content.
	SnapshotLogEntryID *LogID
}

// ClaimState is the current durable execution custody of an issue.
// StartedAt is the start of this claim attempt, not issue creation time.
type ClaimState struct {
	// Actor owns custody until release or a terminal lifecycle transition.
	Actor     Actor
	StartedAt time.Time
}

// WaitingState records why an open issue has no custody but is not available
// to automatic claim selection.
type WaitingState struct {
	// Reason names the directed continuation, acceptance,
	// or condition required next.
	Reason string

	// Since is the time custody was released into waiting status.
	Since time.Time
}

// State is an immutable durable board entity. Load validates every persisted
// value so policy methods never operate on malformed state. State methods use
// value semantics; operations that change state return a modified copy.
type State struct {
	id            ID
	title         string
	kind          Kind
	lifecycle     Lifecycle
	priority      Priority
	activeClaim   *ClaimState
	created       time.Time
	updated       time.Time
	closedAt      *time.Time
	waiting       *WaitingState
	summary       string
	details       string
	recoveryState *RecoveryState
	result        string
}

// Load validates and restores one durable issue state.
func Load(snapshot Snapshot) (State, error) {
	if _, err := NewID(snapshot.ID.String()); err != nil {
		return State{}, err
	}
	title := strings.TrimSpace(snapshot.Title)
	if title == "" {
		return State{}, errkind.Errorf(errkind.InvalidInput, "invalid input: title required")
	}
	if _, err := NewKind(snapshot.Kind.String()); err != nil {
		return State{}, err
	}
	if _, err := NewLifecycle(snapshot.Lifecycle.String()); err != nil {
		return State{}, err
	}
	if _, err := NewPriority(snapshot.Priority.Int()); err != nil {
		return State{}, err
	}
	if snapshot.Created.IsZero() || snapshot.Updated.IsZero() {
		return State{}, errkind.Errorf(errkind.InvalidInput, "invalid input: issue timestamps required")
	}
	if snapshot.ActiveClaim != nil {
		if snapshot.ActiveClaim.Actor == "" || NewActor(snapshot.ActiveClaim.Actor.String()) != snapshot.ActiveClaim.Actor || snapshot.ActiveClaim.StartedAt.IsZero() {
			return State{}, errkind.Errorf(errkind.InvalidInput, "invalid input: active claim requires a normalized actor and start time")
		}
		if !snapshot.Kind.Executable() || snapshot.Lifecycle != LifecycleOpen {
			return State{}, errkind.Errorf(errkind.InvalidInput, "invalid input: only open executable issues can have an active claim")
		}
	}
	if snapshot.Waiting != nil {
		if snapshot.ActiveClaim != nil || !snapshot.Kind.Executable() ||
			snapshot.Lifecycle != LifecycleOpen {
			return State{}, errkind.Errorf(
				errkind.InvalidInput,
				"invalid input: only unclaimed open executable issues can be waiting",
			)
		}
		if err := validateWaiting(*snapshot.Waiting); err != nil {
			return State{}, err
		}
	}
	if err := validateRecoveryState(snapshot.RecoveryState); err != nil {
		return State{}, err
	}
	return State{
		id:            snapshot.ID,
		title:         title,
		kind:          snapshot.Kind,
		lifecycle:     snapshot.Lifecycle,
		priority:      snapshot.Priority,
		activeClaim:   cloneActiveClaim(snapshot.ActiveClaim),
		created:       snapshot.Created,
		updated:       snapshot.Updated,
		closedAt:      cloneTime(snapshot.ClosedAt),
		waiting:       cloneWaiting(snapshot.Waiting),
		summary:       snapshot.Summary,
		details:       snapshot.Details,
		recoveryState: cloneRecoveryState(snapshot.RecoveryState),
		result:        snapshot.Result,
	}, nil
}

// ID returns the stable issue identity.
func (i State) ID() ID { return i.id }

// Title returns the normalized issue title.
func (i State) Title() string { return i.title }

// Kind returns the issue's workflow kind.
func (i State) Kind() Kind { return i.kind }

// Lifecycle returns the persisted lifecycle state.
func (i State) Lifecycle() Lifecycle { return i.lifecycle }

// Status returns the caller-visible status derived from lifecycle and custody.
func (i State) Status() Status {
	if i.activeClaim != nil {
		return StatusInProgress
	}
	if i.waiting != nil {
		return StatusWaiting
	}
	switch i.lifecycle {
	case LifecycleOpen:
		return StatusReady
	case LifecycleClosed:
		return StatusClosed
	case LifecycleCancelled:
		return StatusCancelled
	default:
		return statusUnknown
	}
}

// Priority returns the issue's scheduling priority.
func (i State) Priority() Priority { return i.priority }

// ActiveClaim returns a copy of the current claim, if any.
func (i State) ActiveClaim() *ClaimState { return cloneActiveClaim(i.activeClaim) }

// Assignee returns the actor holding the current claim.
func (i State) Assignee() Actor {
	if i.activeClaim == nil {
		return ""
	}
	return i.activeClaim.Actor
}

// Created returns the issue creation time.
func (i State) Created() time.Time { return i.created }

// Updated returns the most recent semantic update time.
func (i State) Updated() time.Time { return i.updated }

// StartedAt returns the current claim's start time, if claimed.
func (i State) StartedAt() *time.Time {
	if i.activeClaim == nil {
		return nil
	}
	return cloneTime(&i.activeClaim.StartedAt)
}

// ClosedAt returns the terminal transition time, if terminal.
func (i State) ClosedAt() *time.Time { return cloneTime(i.closedAt) }

// Waiting returns the issue's current waiting projection, if any.
func (i State) Waiting() *WaitingState { return cloneWaiting(i.waiting) }

// Summary returns the concise stable contract inherited by descendants.
func (i State) Summary() string { return i.summary }

// Details returns expanded stable material disclosed on demand.
func (i State) Details() string { return i.details }

// RecoveryState returns the mutable recovery position.
func (i State) RecoveryState() string {
	if i.recoveryState == nil {
		return ""
	}
	return i.recoveryState.Body
}

// RecoveryStateRecord returns a copy of the complete mutable recovery record.
func (i State) RecoveryStateRecord() *RecoveryState {
	return cloneRecoveryState(i.recoveryState)
}

// WithRecoveryState returns a copy carrying State and its issue update time.
func (i State) WithRecoveryState(state *RecoveryState, updated time.Time) State {
	i.recoveryState = cloneRecoveryState(state)
	i.updated = updated
	return i
}

// WithRecoveryStateSnapshot returns a copy linked to matching Log history.
func (i State) WithRecoveryStateSnapshot(id *LogID) State {
	if i.recoveryState != nil {
		i.recoveryState = cloneRecoveryState(i.recoveryState)
		i.recoveryState.SnapshotLogEntryID = cloneLogID(id)
	}
	return i
}

// Result returns the issue's durable result.
func (i State) Result() string { return i.result }

// Snapshot returns the complete semantic representation for persistence.
func (i State) Snapshot() Snapshot {
	return Snapshot{
		ID:            i.id,
		Title:         i.title,
		Kind:          i.kind,
		Lifecycle:     i.lifecycle,
		Priority:      i.priority,
		ActiveClaim:   cloneActiveClaim(i.activeClaim),
		Created:       i.created,
		Updated:       i.updated,
		ClosedAt:      cloneTime(i.closedAt),
		Waiting:       cloneWaiting(i.waiting),
		Summary:       i.summary,
		Details:       i.details,
		RecoveryState: cloneRecoveryState(i.recoveryState),
		Result:        i.result,
	}
}

// NewRecoveryState constructs attributed mutable recovery State.
func NewRecoveryState(
	body string,
	nextAction string,
	author Actor,
	updatedAt time.Time,
) (*RecoveryState, error) {
	state := &RecoveryState{
		Body: body, NextAction: nextAction,
		Author: author, UpdatedAt: new(updatedAt),
	}
	if err := validateRecoveryState(state); err != nil {
		return nil, err
	}
	return state, nil
}

func validateRecoveryState(state *RecoveryState) error {
	if state == nil {
		return nil
	}
	if strings.TrimSpace(state.Body) == "" {
		return errkind.Errorf(errkind.InvalidInput, "invalid input: state required")
	}
	if state.NextAction != "" && strings.TrimSpace(state.NextAction) == "" {
		return errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: state next action must not be blank",
		)
	}
	if (state.Author == "") != (state.UpdatedAt == nil) {
		return errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: state attribution is incomplete",
		)
	}
	if state.UpdatedAt != nil && state.UpdatedAt.IsZero() {
		return errkind.Errorf(errkind.InvalidInput, "invalid input: state update time required")
	}
	if state.SnapshotLogEntryID != nil {
		if _, err := NewLogID(state.SnapshotLogEntryID.String()); err != nil {
			return err
		}
	}
	return nil
}

// NewWaitingState validates and constructs waiting metadata.
func NewWaitingState(reason string, since time.Time) (*WaitingState, error) {
	waiting := WaitingState{Reason: strings.TrimSpace(reason), Since: since}
	if err := validateWaiting(waiting); err != nil {
		return nil, err
	}
	return &waiting, nil
}

func validateWaiting(waiting WaitingState) error {
	if waiting.Reason == "" || waiting.Reason != strings.TrimSpace(waiting.Reason) {
		return errkind.Errorf(errkind.InvalidInput, "invalid input: waiting reason required")
	}
	for _, character := range waiting.Reason {
		if unicode.IsControl(character) {
			return errkind.Errorf(
				errkind.InvalidInput,
				"invalid input: waiting reason must be single-line plain text",
			)
		}
	}
	if waiting.Since.IsZero() {
		return errkind.Errorf(errkind.InvalidInput, "invalid input: waiting time required")
	}
	return nil
}

func cloneWaiting(value *WaitingState) *WaitingState {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneActiveClaim(value *ClaimState) *ClaimState {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneRecoveryState(value *RecoveryState) *RecoveryState {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.UpdatedAt = cloneTime(value.UpdatedAt)
	cloned.SnapshotLogEntryID = cloneLogID(value.SnapshotLogEntryID)
	return &cloned
}

func cloneLogID(value *LogID) *LogID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
