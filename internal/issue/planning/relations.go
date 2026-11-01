package planning

import (
	"encoding/hex"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

// Relations describes the supported edges and current issue values in one
// proposed board graph. Unresolved edges are omitted; they have no current
// assignment. Repair callers include the contender edges they intend to retain.
type Relations struct {
	// Issues contains each endpoint once, identified by private identity.
	Issues []RelationIssue
	// Dependencies contains directed readiness requirements.
	Dependencies []RelationDependency
	// Parents contains the proposed containment assignments.
	Parents []RelationParent
}

// RelationIssue contains the issue values consumed by containment policy.
type RelationIssue struct {
	// UID identifies the issue independently of its public reference.
	UID issue.UID // required
	// Kind is nil when the issue's role is unresolved.
	Kind *issue.Kind
	// Lifecycle is nil when the issue's lifecycle is unresolved.
	Lifecycle *issue.Lifecycle
}

// RelationDependency requires one issue to finish before another may proceed.
type RelationDependency struct {
	// IssueUID identifies the issue with the readiness requirement.
	IssueUID issue.UID // required
	// PrerequisiteUID identifies the required issue.
	PrerequisiteUID issue.UID // required
}

// RelationParent assigns one containment parent to a child.
type RelationParent struct {
	// ChildUID identifies the contained issue.
	ChildUID issue.UID // required
	// ParentUID identifies its container.
	ParentUID issue.UID // required
}

// ValidateRelations checks cycles and containment constraints that the supplied
// values establish. Unknown kind or lifecycle values do not become defaults,
// and do not prevent repairs to unrelated resolved relationships.
// Every edge endpoint must occur in Issues.
func ValidateRelations(relations Relations) error {
	nodes := make([]relationNode, len(relations.Issues))
	indices := make(map[issue.UID]int, len(nodes))
	for index, current := range relations.Issues {
		if _, exists := indices[current.UID]; exists {
			return ErrIncompleteSnapshot
		}
		indices[current.UID] = index
		nodes[index] = relationNode{
			name: hex.EncodeToString(current.UID.Bytes()),
			kind: current.Kind, lifecycle: current.Lifecycle, parent: -1,
		}
	}
	for _, edge := range relations.Dependencies {
		dependent, haveDependent := indices[edge.IssueUID]
		prerequisite, havePrerequisite := indices[edge.PrerequisiteUID]
		if !haveDependent || !havePrerequisite {
			return ErrIncompleteSnapshot
		}
		nodes[dependent].dependencies = append(nodes[dependent].dependencies, prerequisite)
	}
	for _, edge := range relations.Parents {
		child, haveChild := indices[edge.ChildUID]
		parent, haveParent := indices[edge.ParentUID]
		if !haveChild || !haveParent {
			return ErrIncompleteSnapshot
		}
		if current := nodes[child].parent; current >= 0 && current != parent {
			return errkind.Errorf(errkind.InvalidInput, "issue cannot have multiple parents")
		}
		nodes[child].parent = parent
	}
	return validateRelationGraph(nodes)
}

// relationNode uses collection indices so apply dry runs can validate new
// issues before allocating their durable identities. The public validator maps
// persisted UIDs into the same representation.
type relationNode struct {
	name         string
	kind         *issue.Kind
	lifecycle    *issue.Lifecycle
	dependencies []int
	parent       int
}

func validateRelationGraph(nodes []relationNode) error {
	if dependencyCycle(nodes) {
		return errkind.Errorf(errkind.InvalidInput, "dependency graph must remain acyclic")
	}
	if containmentCycle(nodes) {
		return errkind.Errorf(errkind.InvalidInput, "containment would create a cycle")
	}
	children := make([]int, len(nodes))
	for _, node := range nodes {
		if node.parent < 0 {
			continue
		}
		parent := nodes[node.parent]
		children[node.parent]++
		if node.lifecycle != nil && *node.lifecycle == issue.LifecycleOpen &&
			(parent.lifecycle != nil && *parent.lifecycle != issue.LifecycleOpen ||
				parent.kind != nil && *parent.kind != issue.KindWorkstream && *parent.kind != issue.KindRoutine) {
			return errkind.Errorf(
				errkind.InvalidInput,
				"invalid input: open issue requires an open workstream or routine parent",
			)
		}
	}
	for index, childCount := range children {
		if childCount > 0 && nodes[index].kind != nil && *nodes[index].kind == issue.KindTask {
			return errkind.Errorf(
				errkind.InvalidInput,
				"invalid input: task issue %q cannot contain children",
				nodes[index].name,
			)
		}
	}
	return nil
}

func dependencyCycle(nodes []relationNode) bool {
	state := make([]uint8, len(nodes))
	var visit func(int) bool
	visit = func(node int) bool {
		state[node] = 1
		for _, dependency := range nodes[node].dependencies {
			if state[dependency] == 1 || state[dependency] == 0 && visit(dependency) {
				return true
			}
		}
		state[node] = 2
		return false
	}
	for node := range nodes {
		if state[node] == 0 && visit(node) {
			return true
		}
	}
	return false
}

func containmentCycle(nodes []relationNode) bool {
	for start := range nodes {
		seen := make(map[int]struct{})
		for node := start; node >= 0; node = nodes[node].parent {
			if _, ok := seen[node]; ok {
				return true
			}
			seen[node] = struct{}{}
		}
	}
	return false
}
