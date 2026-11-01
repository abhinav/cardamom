package boardconflict

import (
	"fmt"
	"slices"
	"time"

	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
)

// ResolveRelations describes changes to the reviewed contributing edges.
// Edges without a requested change are retained with their original authorship.
type ResolveRelations struct {
	// ConflictID must identify the assignment set the caller reviewed.
	ConflictID string // required

	// Remove excludes selected contributing assignments. Removing every
	// contender of one relationship assigns its absence.
	Remove []AssignmentID

	// Parents chooses a parent or explicit absence for implicated children.
	// Several children may change in one repair.
	Parents []ParentChoice

	// OccurredAt attributes any newly authored assignments.
	OccurredAt time.Time // required
}

// ParentChoice replaces one implicated child's containment assignment.
type ParentChoice struct {
	// ChildUID identifies a child whose parent contributes to the constraint.
	ChildUID issue.UID // required

	// ParentUID is the desired parent; nil explicitly removes the current parent.
	ParentUID *issue.UID
}

// RelationsSnapshot is one coherent view used to admit a graph repair.
// Supported contains ordinary current graph values, excluding obscured edges.
type RelationsSnapshot struct {
	// Assignments contains the separately loaded contributing edge assignments.
	Assignments []RetainedAssignment // required

	// Supported contains all endpoint identities, current kind/lifecycle values,
	// and unrelated supported edges. Unknown scalar values remain nil.
	Supported planning.Relations // required
}

// ResolveRelations validates a compound repair against the whole supported graph.
// It returns no partial repair if the review token changed or the proposed graph
// remains invalid. Known kind/lifecycle values are consumed without being hidden
// or reauthored. The snapshot and its retained values remain unchanged.
func (c *ConstraintConflict) ResolveRelations(request ResolveRelations, snapshot RelationsSnapshot) (ConstraintRepair, error) {
	if err := c.requireReview(request.ConflictID, ConstraintRelationships); err != nil {
		return ConstraintRepair{}, err
	}
	groups, err := c.retainedUnits(snapshot.Assignments)
	if err != nil {
		return ConstraintRepair{}, err
	}
	removals := make(map[AssignmentID]struct{}, len(request.Remove))
	for _, id := range request.Remove {
		if !slices.ContainsFunc(c.assignments, func(value AssignmentReference) bool { return value.AlternativeID == id }) {
			return ConstraintRepair{}, errkind.Errorf(errkind.Conflict, "assignment %q is not in the reviewed constraint", id)
		}
		removals[id] = struct{}{}
	}
	parents := make(map[issue.UID]*issue.UID, len(request.Parents))
	for _, choice := range request.Parents {
		if _, exists := parents[choice.ChildUID]; exists {
			return ConstraintRepair{}, errkind.Errorf(errkind.InvalidInput, "child has more than one parent choice")
		}
		index := slices.IndexFunc(c.assignments, func(value AssignmentReference) bool {
			return value.Unit.Kind == boardhistory.EventIssueContainment && value.Unit.IssueUID == choice.ChildUID
		})
		if index < 0 {
			return ConstraintRepair{}, errkind.Errorf(errkind.InvalidInput, "parent choice must address an implicated child")
		}
		for _, assignment := range c.assignments {
			if assignment.Unit.Kind != boardhistory.EventIssueContainment || assignment.Unit.IssueUID != choice.ChildUID {
				continue
			}
			if _, removed := removals[assignment.AlternativeID]; removed {
				return ConstraintRepair{}, errkind.Errorf(errkind.InvalidInput, "choose a parent or remove its assignments, not both")
			}
		}
		parents[choice.ChildUID] = choice.ParentUID
	}
	graph := snapshot.Supported
	graph.Dependencies = slices.Clone(graph.Dependencies)
	graph.Parents = slices.Clone(graph.Parents)
	var repair ConstraintRepair
	for _, group := range groups {
		var value boardhistory.Payload
		parent, chosen := parents[group.unit.IssueUID]
		if chosen && group.unit.Kind == boardhistory.EventIssueContainment {
			previous := group.values[0].Value.(*boardhistory.IssueContainment)
			replacement := &boardhistory.IssueContainment{ChildUID: previous.ChildUID, ParentUID: previous.ParentUID}
			if parent != nil {
				replacement.ParentUID, replacement.Present = *parent, true
			}
			value = replacement
		} else {
			remaining := make([]RetainedAssignment, 0, len(group.values))
			for _, assignment := range group.values {
				if _, removed := removals[assignment.AlternativeID]; !removed {
					remaining = append(remaining, assignment)
				}
			}
			switch len(remaining) {
			case 0:
				switch previous := group.values[0].Value.(type) {
				case *boardhistory.IssueDependency:
					value = &boardhistory.IssueDependency{IssueUID: previous.IssueUID, PrerequisiteUID: previous.PrerequisiteUID}
				case *boardhistory.IssueContainment:
					value = &boardhistory.IssueContainment{ChildUID: previous.ChildUID, ParentUID: previous.ParentUID}
				}
			case 1:
				value = remaining[0].Value
			default:
				return ConstraintRepair{}, errkind.Errorf(errkind.InvalidInput, "choose one assignment for each disputed relationship")
			}
		}
		switch edge := value.(type) {
		case *boardhistory.IssueDependency:
			if edge.Present {
				graph.Dependencies = append(graph.Dependencies, planning.RelationDependency{IssueUID: edge.IssueUID, PrerequisiteUID: edge.PrerequisiteUID})
			}
		case *boardhistory.IssueContainment:
			if edge.Present {
				graph.Parents = append(graph.Parents, planning.RelationParent{ChildUID: edge.ChildUID, ParentUID: edge.ParentUID})
			}
		}
		if len(group.values) > 1 || !sameConstraintRelation(group.values[0].Value, value) {
			event, err := constraintEvent(value, request.OccurredAt)
			if err != nil {
				return ConstraintRepair{}, err
			}
			repair.Author = append(repair.Author, event)
		} else {
			repair.Restore = append(repair.Restore, group.values[0])
		}
	}
	// The retained component cannot be validated in isolation: an otherwise
	// unrelated supported edge may close a cycle through a restored contributor.
	if err := planning.ValidateRelations(graph); err != nil {
		return ConstraintRepair{}, fmt.Errorf("repaired relationships: %w", err)
	}
	return repair, nil
}

// sameConstraintRelation compares the assigned relation, rather than provenance.
// An absent parent has no target, even though its removal event names an old edge.
func sameConstraintRelation(left, right boardhistory.Payload) bool {
	switch left := left.(type) {
	case *boardhistory.IssueDependency:
		return *left == *right.(*boardhistory.IssueDependency)
	case *boardhistory.IssueContainment:
		other := right.(*boardhistory.IssueContainment)
		return left.Present == other.Present && (!left.Present || left.ParentUID == other.ParentUID)
	default:
		return false
	}
}
