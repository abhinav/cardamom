package issue

import (
	"regexp"
	"strings"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/must"
)

const idGrammar = `[A-Za-z0-9][A-Za-z0-9-]*`

var idPattern = regexp.MustCompile(`^` + idGrammar + `$`)

// ID is a stable store-global issue identity.
type ID string

// NewID parses an issue identity matching [A-Za-z0-9][A-Za-z0-9-]*.
func NewID(value string) (ID, error) {
	if !idPattern.MatchString(value) {
		return "", errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: issue ID %q must match %s",
			value,
			idGrammar,
		)
	}
	return ID(value), nil
}

// MustID returns an issue identity that Cardamom code guarantees is valid.
func MustID(value string) ID {
	id, err := NewID(value)
	must.NotErrorf(err, "code-owned issue identity %q must be valid", value)
	return id
}

// String returns the textual representation of issue id.
func (id ID) String() string { return string(id) }

// Kind selects an issue's workflow policy.
type Kind uint8

const (
	_ Kind = iota
	// KindWorkstream identifies a persistent, nestable deliverable.
	KindWorkstream
	// KindTask identifies bounded executable work.
	KindTask
	// KindCheckpoint identifies an explicit workflow gate.
	KindCheckpoint
	// KindRoutine identifies a persistent, directly claimed workstream.
	KindRoutine
)

var kindNames = [...]string{"", "workstream", "task", "checkpoint", "routine"}

// NewKind validates and constructs issue kind.
func NewKind(value string) (Kind, error) {
	switch value {
	case "workstream":
		return KindWorkstream, nil
	case "task":
		return KindTask, nil
	case "checkpoint":
		return KindCheckpoint, nil
	case "routine":
		return KindRoutine, nil
	default:
		return 0, errkind.Errorf(errkind.InvalidInput, "invalid input: invalid type %q (valid: workstream, task, checkpoint, routine)", value)
	}
}

// ValidKinds returns every supported issue kind.
func ValidKinds() []Kind {
	return []Kind{KindWorkstream, KindTask, KindCheckpoint, KindRoutine}
}

// String returns the textual representation of issue kind.
func (k Kind) String() string {
	if int(k) >= len(kindNames) {
		return ""
	}
	return kindNames[k]
}

// Executable reports whether the issue kind can hold execution custody.
func (k Kind) Executable() bool {
	return k == KindWorkstream || k == KindTask || k == KindRoutine
}

// Status is the caller-visible execution state derived from lifecycle,
// custody, waiting metadata, and dependencies.
type Status uint8

const (
	statusUnknown Status = iota
	// StatusReady identifies unfinished work without custody or blockers.
	StatusReady
	// StatusBlocked identifies unfinished work with an open prerequisite.
	StatusBlocked
	// StatusInProgress identifies an issue with active execution custody.
	StatusInProgress
	// StatusWaiting identifies open work excluded from automatic selection.
	StatusWaiting
	// StatusClosed identifies a successfully completed issue.
	StatusClosed
	// StatusCancelled identifies an issue terminated without completion.
	StatusCancelled
)

var statusNames = [...]string{
	"", "ready", "blocked", "in_progress", "waiting", "closed", "cancelled",
}

// NewStatus validates and constructs issue status.
func NewStatus(value string) (Status, error) {
	switch value {
	case "ready":
		return StatusReady, nil
	case "blocked":
		return StatusBlocked, nil
	case "in_progress":
		return StatusInProgress, nil
	case "waiting":
		return StatusWaiting, nil
	case "closed":
		return StatusClosed, nil
	case "cancelled":
		return StatusCancelled, nil
	default:
		return 0, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: invalid status %q (valid: ready, blocked, in_progress, waiting, closed, cancelled)",
			value,
		)
	}
}

// ValidStatuses returns every supported issue status.
func ValidStatuses() []Status {
	return []Status{
		StatusReady, StatusBlocked, StatusInProgress,
		StatusWaiting, StatusClosed, StatusCancelled,
	}
}

// String returns the textual representation of issue status.
func (s Status) String() string {
	if int(s) >= len(statusNames) {
		return ""
	}
	return statusNames[s]
}

// Lifecycle returns the persisted lifecycle represented by the status.
func (s Status) Lifecycle() Lifecycle {
	switch s {
	case StatusReady, StatusBlocked, StatusInProgress, StatusWaiting:
		return LifecycleOpen
	case StatusClosed:
		return LifecycleClosed
	case StatusCancelled:
		return LifecycleCancelled
	default:
		return lifecycleUnknown
	}
}

// Terminal reports whether the value represents a terminal state.
func (s Status) Terminal() bool { return s == StatusClosed || s == StatusCancelled }

// Lifecycle is the durable completion state of an issue. Execution
// custody is represented separately by ActiveClaim.
type Lifecycle uint8

const (
	lifecycleUnknown Lifecycle = iota
	// LifecycleOpen identifies a non-terminal persisted state.
	LifecycleOpen
	// LifecycleClosed identifies successful persisted completion.
	LifecycleClosed
	// LifecycleCancelled identifies persisted termination without completion.
	LifecycleCancelled
)

var lifecycleNames = [...]string{"", "open", "closed", "cancelled"}

// NewLifecycle validates and constructs issue lifecycle.
func NewLifecycle(value string) (Lifecycle, error) {
	switch value {
	case "open":
		return LifecycleOpen, nil
	case "closed":
		return LifecycleClosed, nil
	case "cancelled":
		return LifecycleCancelled, nil
	default:
		return 0, errkind.Errorf(errkind.InvalidInput, "invalid input: invalid lifecycle %q (valid: open, closed, cancelled)", value)
	}
}

// ValidLifecycles returns every supported issue lifecycle.
func ValidLifecycles() []Lifecycle {
	return []Lifecycle{LifecycleOpen, LifecycleClosed, LifecycleCancelled}
}

// String returns the textual representation of issue lifecycle.
func (s Lifecycle) String() string {
	if int(s) >= len(lifecycleNames) {
		return ""
	}
	return lifecycleNames[s]
}

// Terminal reports whether the value represents a terminal state.
func (s Lifecycle) Terminal() bool {
	return s == LifecycleClosed || s == LifecycleCancelled
}

// Priority orders issues from highest urgency to lowest urgency.
type Priority int

const (
	// PriorityHighest is the most urgent priority.
	PriorityHighest Priority = iota
	// PriorityHigh is more urgent than the normal priority.
	PriorityHigh
	// PriorityNormal is the default priority.
	PriorityNormal
	// PriorityLow is less urgent than the normal priority.
	PriorityLow
	// PriorityLowest is the least urgent priority.
	PriorityLowest
)

// NewPriority parses a numeric priority from 0 through 4.
func NewPriority(value int) (Priority, error) {
	if value < int(PriorityHighest) || value > int(PriorityLowest) {
		return 0, errkind.Errorf(errkind.InvalidInput, "invalid input: invalid priority %d (must be 0..4)", value)
	}
	return Priority(value), nil
}

// Int returns the numeric priority value.
func (p Priority) Int() int { return int(p) }

// Actor is a normalized execution or attribution identity.
type Actor string

// NewActor normalizes an execution or attribution identity.
func NewActor(value string) Actor { return Actor(strings.TrimSpace(value)) }

// String returns the textual representation of actor.
func (a Actor) String() string { return string(a) }

// Label is a normalized issue classification value that does not begin with
// `+` or `-`.
type Label string

// NewLabel trims surrounding whitespace and constructs a label.
// It rejects empty values and labels beginning with `+` or `-`.
func NewLabel(value string) (Label, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errkind.Errorf(errkind.InvalidInput, "invalid input: label cannot be empty")
	}
	if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return "", errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: label cannot start with + or -",
		)
	}
	return Label(value), nil
}

// MustLabel returns a label that Cardamom code guarantees is valid.
func MustLabel(value string) Label {
	label, err := NewLabel(value)
	must.NotErrorf(err, "code-owned label %q must be valid", value)
	return label
}

// String returns the textual representation of label.
func (l Label) String() string { return string(l) }
