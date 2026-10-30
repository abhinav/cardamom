package issue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveStatusPreservesPartialExecutionKnowledge(t *testing.T) {
	for _, test := range []struct {
		name  string
		input StatusInput
		want  Status
	}{
		{"open", StatusInput{Lifecycle: new(LifecycleOpen)}, StatusReady},
		{"closed", StatusInput{Lifecycle: new(LifecycleClosed)}, StatusClosed},
		{"claimed", StatusInput{Lifecycle: new(LifecycleOpen), Claimed: true}, StatusInProgress},
		{"waiting", StatusInput{Lifecycle: new(LifecycleOpen), Waiting: true}, StatusWaiting},
		{"unknown lifecycle", StatusInput{Waiting: true}, StatusUnresolved},
		{"unknown waiting", StatusInput{Lifecycle: new(LifecycleOpen), Conflicts: []ConflictReference{{ID: "waiting", Field: FieldWaiting}}}, StatusUnresolved},
		{"unknown kind", StatusInput{Lifecycle: new(LifecycleOpen), Conflicts: []ConflictReference{{ID: "kind", Field: FieldKind}}}, StatusUnresolved},
		{"unknown dependencies", StatusInput{Lifecycle: new(LifecycleOpen), Conflicts: []ConflictReference{{ID: "dependencies", Field: FieldDependencies}}}, StatusUnresolved},
		{"unknown checkpoint decision", StatusInput{Lifecycle: new(LifecycleOpen), Conflicts: []ConflictReference{{ID: "checkpoint", Field: FieldCheckpoint}}}, StatusUnresolved},
		{"closed with unknown kind", StatusInput{Lifecycle: new(LifecycleClosed), Conflicts: []ConflictReference{{ID: "kind", Field: FieldKind}}}, StatusClosed},
		{"waiting with unknown kind", StatusInput{Lifecycle: new(LifecycleOpen), Waiting: true, Conflicts: []ConflictReference{{ID: "kind", Field: FieldKind}}}, StatusWaiting},
		{"unrelated title", StatusInput{Lifecycle: new(LifecycleOpen), Conflicts: []ConflictReference{{ID: "title", Field: FieldTitle}}}, StatusReady},
	} {
		t.Run(test.name, func(t *testing.T) { assert.Equal(t, test.want, DeriveStatus(test.input)) })
	}
}
