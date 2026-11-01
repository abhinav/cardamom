package boardconflict

import (
	"bytes"
	"errors"
	"fmt"
	"iter"
	"slices"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

// ConstraintConflict identifies assignments whose combination cannot be supported.
// Unlike a scalar Conflict, its assignments are contributors to one invalid
// combination, not alternative complete graphs or execution states.
// It retains metadata only; repair callers load the finite implicated values.
type ConstraintConflict struct {
	id          string
	boardID     board.ID
	kind        ConstraintKind
	assignments []AssignmentReference
}

// NewConstraintConflict groups equal assignments and binds their review identity.
// The caller establishes causal maximality and the violated constraint from a
// coherent board snapshot. This constructor validates the finite assignment
// families and their metadata; it does not rediscover a whole-board violation.
// Execution constraints must include a retained positive waiting assignment.
// A coupled constraint retains every contender of an implicated unit;
// it never expands their combinations into alternative complete states.
func NewConstraintConflict(boardID board.ID, candidates iter.Seq2[Candidate, error]) (*ConstraintConflict, error) {
	conflict := &ConstraintConflict{boardID: boardID}
	type valueKey struct {
		unit     Unit
		semantic [32]byte
	}
	type originKey struct {
		unit  Unit
		event boardhistory.EventID
	}
	groups := make(map[valueKey]int)
	origins := make(map[originKey][32]byte)
	hasWaiting := false
	var executionIssue issue.UID
	for candidate, err := range candidates {
		if err != nil {
			return nil, fmt.Errorf("read constraint assignment: %w", err)
		}
		encoded, err := encodeValue(candidate.Value)
		if err != nil {
			return nil, fmt.Errorf("constraint assignment: %w", err)
		}
		unit, err := UnitOf(candidate.Value)
		if err != nil {
			return nil, err
		}
		kind, err := constraintKind(unit.Kind)
		if err != nil {
			return nil, err
		}
		if conflict.kind == 0 {
			conflict.kind = kind
		}
		if conflict.kind != kind {
			return nil, errors.New("constraint assignments address different constraint families")
		}
		if kind == ConstraintExecution {
			if executionIssue == (issue.UID{}) {
				executionIssue = unit.IssueUID
			}
			if executionIssue != unit.IssueUID {
				return nil, errors.New("execution constraint must address one issue")
			}
		}
		if len(candidate.Origins) == 0 {
			return nil, errors.New("constraint assignment requires originating events")
		}
		key := semanticKey(candidate.Value, encoded)
		group := valueKey{unit: unit, semantic: key}
		index, exists := groups[group]
		if !exists {
			index = len(conflict.assignments)
			groups[group] = index
			conflict.assignments = append(conflict.assignments, AssignmentReference{Unit: unit, AlternativeID: "", Origins: nil})
		}
		if waiting, ok := candidate.Value.(*boardhistory.IssueWaitingSet); ok && waiting.Waiting != nil {
			hasWaiting = true
			conflict.assignments[index].WaitingPresent = true
		}
		for _, origin := range candidate.Origins {
			if origin.Writer == (board.WriterUID{}) || origin.Sequence == 0 {
				return nil, errors.New("constraint assignment requires a writer and sequence")
			}
			source := originKey{unit: unit, event: origin}
			if previous, exists := origins[source]; exists && previous != key {
				return nil, errors.New("one originating event assigns incompatible values to one unit")
			}
			origins[source] = key
			conflict.assignments[index].Origins = append(conflict.assignments[index].Origins, origin)
		}
	}
	if len(conflict.assignments) == 0 {
		return nil, errors.New("constraint requires contributing assignments")
	}
	if conflict.kind == ConstraintExecution && !hasWaiting {
		return nil, errors.New("execution constraint requires a retained waiting state")
	}
	for index := range conflict.assignments {
		assignment := &conflict.assignments[index]
		slices.SortFunc(assignment.Origins, compareEventID)
		assignment.Origins = slices.Compact(assignment.Origins)
		// An origin event can assign several fields. Unit identity distinguishes
		// those contributions even when their originating event sets are equal.
		identity := append(assignment.Unit.identityBytes(), eventIdentityBytes(assignment.Origins)...)
		assignment.AlternativeID = AssignmentID(digestID("alt_", identity))
	}
	slices.SortFunc(conflict.assignments, func(left, right AssignmentReference) int {
		return bytes.Compare([]byte(left.AlternativeID), []byte(right.AlternativeID))
	})
	identity := []byte(boardID.String() + "\x00" + conflict.kind.String() + "\x00")
	for _, assignment := range conflict.assignments {
		identity = append(identity, []byte(assignment.AlternativeID)...)
	}
	conflict.id = digestID("conflict_", identity)
	return conflict, nil
}

// ConstraintSnapshot is validated metadata persisted by a supported writer.
// Assignment bodies remain in the immutable mutation journal.
type ConstraintSnapshot struct {
	// ID binds the displayed assignment set.
	ID string // required

	// BoardID identifies the logical board.
	BoardID board.ID // required

	// Kind identifies the violated policy family.
	Kind ConstraintKind // required

	// Assignments identifies the contributing units and their originating events.
	Assignments []AssignmentReference // required
}

// LoadConstraint restores supported metadata without revalidating retained bodies.
// It owns copies of the assignment and origin slices.
func LoadConstraint(snapshot ConstraintSnapshot) *ConstraintConflict {
	return &ConstraintConflict{id: snapshot.ID, boardID: snapshot.BoardID, kind: snapshot.Kind, assignments: cloneConstraintAssignments(snapshot.Assignments)}
}

// ID is the review token for the displayed assignment set.
func (c *ConstraintConflict) ID() string { return c.id }

// BoardID identifies the logical board containing the constraint.
func (c *ConstraintConflict) BoardID() board.ID { return c.boardID }

// Kind distinguishes relationship constraints from execution compatibility.
func (c *ConstraintConflict) Kind() ConstraintKind { return c.kind }

// Assignments returns stable metadata order and independently owned origin slices.
func (c *ConstraintConflict) Assignments() []AssignmentReference {
	return cloneConstraintAssignments(c.assignments)
}

// AffectedIssues identifies only the finite units obscured by this constraint.
// A known parent kind used to validate an edge remains operative; it is not
// made unresolved merely because an edge cannot be supported with that kind.
func (c *ConstraintConflict) AffectedIssues() []AffectedIssue {
	affected := make([]AffectedIssue, 0, len(c.assignments))
	for _, assignment := range c.assignments {
		var field issue.Field
		switch assignment.Unit.Kind {
		case boardhistory.EventIssueDependency:
			field = issue.FieldDependencies
		case boardhistory.EventIssueContainment:
			field = issue.FieldParent
		case boardhistory.EventIssueKindSet:
			field = issue.FieldKind
		case boardhistory.EventIssueLifecycleSet:
			field = issue.FieldLifecycle
		case boardhistory.EventIssueWaitingSet:
			field = issue.FieldWaiting
		}
		current := AffectedIssue{UID: assignment.Unit.IssueUID, Field: field}
		if !slices.Contains(affected, current) {
			affected = append(affected, current)
		}
	}
	return affected
}

// retainedAssignments binds separately loaded values to the current metadata.
// Journal immutability establishes the value of each originating event; checking
// its unit prevents an adapter from supplying another field of an origin event.
func (c *ConstraintConflict) retainedAssignments(values []RetainedAssignment) ([]RetainedAssignment, error) {
	supplied := make(map[AssignmentID]boardhistory.Payload, len(values))
	for _, value := range values {
		if _, exists := supplied[value.AlternativeID]; exists {
			return nil, errors.New("duplicate retained constraint assignment")
		}
		supplied[value.AlternativeID] = value.Value
	}
	if len(supplied) != len(c.assignments) {
		return nil, errors.New("retained constraint assignments are incomplete")
	}
	out := make([]RetainedAssignment, 0, len(c.assignments))
	for _, assignment := range c.assignments {
		value, exists := supplied[assignment.AlternativeID]
		if !exists {
			return nil, fmt.Errorf("retained assignment %q is unavailable", assignment.AlternativeID)
		}
		unit, err := UnitOf(value)
		if err != nil {
			return nil, err
		}
		if unit != assignment.Unit {
			return nil, errors.New("retained constraint assignment addresses another unit")
		}
		out = append(out, RetainedAssignment{AlternativeID: assignment.AlternativeID, Value: value})
	}
	return out, nil
}

// retainedUnits keeps per-field contenders together for a compound repair.
// Multiple values of one unit require a choice even when their union is acyclic.
func (c *ConstraintConflict) retainedUnits(values []RetainedAssignment) ([]constraintUnitAssignments, error) {
	assignments, err := c.retainedAssignments(values)
	if err != nil {
		return nil, err
	}
	indices := make(map[Unit]int)
	var groups []constraintUnitAssignments
	for index, assignment := range assignments {
		unit := c.assignments[index].Unit
		group, exists := indices[unit]
		if !exists {
			group = len(groups)
			indices[unit] = group
			groups = append(groups, constraintUnitAssignments{unit: unit})
		}
		groups[group].values = append(groups[group].values, assignment)
	}
	return groups, nil
}

func (c *ConstraintConflict) requireReview(id string, kind ConstraintKind) error {
	if id != c.id {
		return errkind.Errorf(errkind.Conflict, "constraint changed; inspect the current assignments")
	}
	if c.kind != kind {
		return errkind.Errorf(errkind.InvalidInput, "constraint %q requires a %s repair", c.id, c.kind.String())
	}
	return nil
}

// AssignmentID selects one contributing assignment in a reviewed constraint.
// It binds the unit and every equal causal origin, not a preferred writer.
type AssignmentID string

// AssignmentReference identifies a retained semantic value without its body.
// Origins are sorted and include every equal concurrent assignment.
type AssignmentReference struct {
	// Unit identifies the independent field or relationship member.
	Unit Unit // required

	// AlternativeID selects this contribution within a constraint repair.
	AlternativeID AssignmentID // required

	// Origins identifies retained events supporting the value.
	Origins []boardhistory.EventID // required

	// WaitingPresent distinguishes a positive waiting assignment from absence.
	// It is false for other units and lets repair choose a body without loading
	// every retained waiting reason.
	WaitingPresent bool
}

// RetainedAssignment supplies one separately loaded relationship or execution
// payload. Execution repair retains at most one chosen waiting body.
type RetainedAssignment struct {
	// AlternativeID identifies the reviewed contribution whose body was loaded.
	AlternativeID AssignmentID // required

	// Value is the ordinary semantic assignment projected from its retained origin.
	Value boardhistory.Payload // required
}

// ConstraintRepair separates new choices from unchanged obscured values.
// The repository journals Author under the current observed frontier, restores
// both collections into projections, and retires the constraint atomically.
// Restore must never be appended as a new causal assignment.
type ConstraintRepair struct {
	// Author contains only values changed by the requested repair.
	Author []boardhistory.Event

	// Restore contains unchanged retained assignments with their existing identities.
	Restore []RetainedAssignment
}

// constraintEvent uses the journal's maintained validation and ownership boundary
// for a newly authored repair value. Retained values never pass through this path.
func constraintEvent(value boardhistory.Payload, occurredAt time.Time) (boardhistory.Event, error) {
	encoded, err := boardhistory.MarshalEvent(boardhistory.Event{OccurredAt: occurredAt, Payload: value})
	if err != nil {
		return boardhistory.Event{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	return boardhistory.UnmarshalEvent(encoded)
}

// ConstraintKind identifies the finite policy violated by contributing assignments.
type ConstraintKind uint8

const (
	_ ConstraintKind = iota
	// ConstraintRelationships identifies invalid dependency or containment edges.
	ConstraintRelationships
	// ConstraintExecution identifies incompatible kind, lifecycle, and waiting values.
	ConstraintExecution
)

// String is the stable spelling used by constraint inspection and persistence.
func (k ConstraintKind) String() string {
	switch k {
	case ConstraintRelationships:
		return "relationships"
	case ConstraintExecution:
		return "execution"
	default:
		return ""
	}
}

func constraintKind(kind boardhistory.EventKind) (ConstraintKind, error) {
	switch kind {
	case boardhistory.EventIssueDependency, boardhistory.EventIssueContainment:
		return ConstraintRelationships, nil
	case boardhistory.EventIssueKindSet, boardhistory.EventIssueLifecycleSet, boardhistory.EventIssueWaitingSet:
		return ConstraintExecution, nil
	default:
		return 0, fmt.Errorf("assignment %q cannot contribute to a constraint conflict", kind)
	}
}

func cloneConstraintAssignments(assignments []AssignmentReference) []AssignmentReference {
	out := slices.Clone(assignments)
	for index := range out {
		out[index].Origins = slices.Clone(out[index].Origins)
	}
	return out
}

// constraintUnitAssignments keeps mutually exclusive values of one unit separate
// from independent units that contribute to the same constraint.
type constraintUnitAssignments struct {
	unit   Unit
	values []RetainedAssignment
}
