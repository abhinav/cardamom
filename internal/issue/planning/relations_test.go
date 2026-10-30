package planning

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestValidateRelations(t *testing.T) {
	parent := planningTestUID(t, 1)
	child := planningTestUID(t, 2)
	for _, tt := range []struct {
		name         string
		parentKind   *issue.Kind
		parentLife   *issue.Lifecycle
		childLife    *issue.Lifecycle
		dependencies []RelationDependency
		parents      []RelationParent
		wantError    string
	}{
		{
			name:       "OpenWorkstream",
			parentKind: new(issue.KindWorkstream),
			parentLife: new(issue.LifecycleOpen),
			childLife:  new(issue.LifecycleOpen),
			parents:    []RelationParent{{ChildUID: child, ParentUID: parent}},
		},
		{
			name:       "UnknownParentKind",
			parentLife: new(issue.LifecycleOpen),
			childLife:  new(issue.LifecycleOpen),
			parents:    []RelationParent{{ChildUID: child, ParentUID: parent}},
		},
		{
			name:       "UnknownParentLifecycle",
			parentKind: new(issue.KindWorkstream),
			childLife:  new(issue.LifecycleOpen),
			parents:    []RelationParent{{ChildUID: child, ParentUID: parent}},
		},
		{
			name:       "ClosedParent",
			parentKind: new(issue.KindWorkstream),
			parentLife: new(issue.LifecycleClosed),
			childLife:  new(issue.LifecycleOpen),
			parents:    []RelationParent{{ChildUID: child, ParentUID: parent}},
			wantError:  "open issue requires an open workstream or routine parent",
		},
		{
			name:       "CheckpointParent",
			parentKind: new(issue.KindCheckpoint),
			parentLife: new(issue.LifecycleOpen),
			childLife:  new(issue.LifecycleOpen),
			parents:    []RelationParent{{ChildUID: child, ParentUID: parent}},
			wantError:  "open issue requires an open workstream or routine parent",
		},
		{
			name:       "TaskWithTerminalChild",
			parentKind: new(issue.KindTask),
			parentLife: new(issue.LifecycleOpen),
			childLife:  new(issue.LifecycleClosed),
			parents:    []RelationParent{{ChildUID: child, ParentUID: parent}},
			wantError:  "cannot contain children",
		},
		{
			name:         "DependencyCycle",
			dependencies: []RelationDependency{{IssueUID: child, PrerequisiteUID: parent}, {IssueUID: parent, PrerequisiteUID: child}},
			wantError:    "dependency graph must remain acyclic",
		},
		{
			name:      "ContainmentCycle",
			parents:   []RelationParent{{ChildUID: child, ParentUID: parent}, {ChildUID: parent, ParentUID: child}},
			wantError: "containment would create a cycle",
		},
		{
			name:      "MultipleParents",
			parents:   []RelationParent{{ChildUID: child, ParentUID: parent}, {ChildUID: child, ParentUID: child}},
			wantError: "multiple parents",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRelations(Relations{
				Issues: []RelationIssue{
					{UID: parent, Kind: tt.parentKind, Lifecycle: tt.parentLife},
					{UID: child, Kind: new(issue.KindTask), Lifecycle: tt.childLife},
				},
				Dependencies: tt.dependencies,
				Parents:      tt.parents,
			})
			if tt.wantError != "" {
				assert.ErrorContains(t, err, tt.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateRelationsAllowsUnknownUnrelatedIssue(t *testing.T) {
	parent := planningTestUID(t, 1)
	child := planningTestUID(t, 2)
	unrelated := planningTestUID(t, 3)
	assert.NoError(t, ValidateRelations(Relations{
		Issues: []RelationIssue{
			{UID: parent, Kind: new(issue.KindWorkstream), Lifecycle: new(issue.LifecycleOpen)},
			{UID: child, Kind: new(issue.KindTask), Lifecycle: new(issue.LifecycleOpen)},
			{UID: unrelated},
		},
		Parents: []RelationParent{{ChildUID: child, ParentUID: parent}},
	}))
}
