package boardconflict

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
)

func TestConstraintConflictPreservesEqualOriginsAndReviewIdentity(t *testing.T) {
	first, second := constraintUID(t, 1), constraintUID(t, 2)
	origins := concurrentOrigins(t, 3)
	edge := &boardhistory.IssueDependency{IssueUID: first, PrerequisiteUID: second, Present: true}
	reverse := &boardhistory.IssueDependency{IssueUID: second, PrerequisiteUID: first, Present: true}
	candidates := []Candidate{{Origins: origins[:2], Value: edge}, {Origins: origins[2:], Value: reverse}}
	grouped, err := NewConstraintConflict("board-test", candidateSequence(candidates))
	require.NoError(t, err)
	split, err := NewConstraintConflict("board-test", candidateSequence([]Candidate{
		candidates[1], {Origins: origins[1:2], Value: edge}, {Origins: origins[:1], Value: edge},
	}))
	require.NoError(t, err)
	assert.Equal(t, grouped.ID(), split.ID())
	assert.Equal(t, grouped.Assignments(), split.Assignments())
	assert.Equal(t, ConstraintRelationships, grouped.Kind())
	assert.Equal(t, board.ID("board-test"), grouped.BoardID())
	assignments := grouped.Assignments()
	count := 0
	for _, assignment := range assignments {
		count += len(assignment.Origins)
	}
	assert.Equal(t, 3, count)
	assignments[0].Origins[0].Sequence++
	assert.Equal(t, grouped.Assignments(), split.Assignments())
	changed, err := NewConstraintConflict("board-test", candidateSequence([]Candidate{{Origins: origins[:1], Value: edge}, candidates[1]}))
	require.NoError(t, err)
	assert.NotEqual(t, grouped.ID(), changed.ID())
	_, err = grouped.ResolveRelations(ResolveRelations{ConflictID: changed.ID(), OccurredAt: time.Unix(10, 0)}, RelationsSnapshot{Assignments: nil, Supported: planning.Relations{}})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
}

func TestConstraintConflictRepairsCycleWithoutReauthoringRetainedEdge(t *testing.T) {
	first, second, third := constraintUID(t, 1), constraintUID(t, 2), constraintUID(t, 3)
	origins := concurrentOrigins(t, 2)
	candidates := []Candidate{
		{Origins: origins[:1], Value: &boardhistory.IssueDependency{IssueUID: first, PrerequisiteUID: second, Present: true}},
		{Origins: origins[1:], Value: &boardhistory.IssueDependency{IssueUID: second, PrerequisiteUID: first, Present: true}},
	}
	conflict, err := NewConstraintConflict("board-test", candidateSequence(candidates))
	require.NoError(t, err)
	snapshot := RelationsSnapshot{
		Assignments: retainedConstraintAssignments(t, conflict, candidates),
		Supported:   planning.Relations{Issues: []planning.RelationIssue{{UID: first}, {UID: second}, {UID: third}}, Dependencies: []planning.RelationDependency{{IssueUID: third, PrerequisiteUID: first}}},
	}
	request := ResolveRelations{ConflictID: conflict.ID(), OccurredAt: time.Unix(10, 0)}
	_, err = conflict.ResolveRelations(request, snapshot)
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	removed := soleConstraintAssignmentID(t, conflict, Unit{Kind: boardhistory.EventIssueDependency, IssueUID: second, RelatedUID: first})
	request.Remove = []AssignmentID{removed}
	repaired, err := conflict.ResolveRelations(request, snapshot)
	require.NoError(t, err)
	require.Len(t, repaired.Author, 1)
	assert.Equal(t, &boardhistory.IssueDependency{IssueUID: second, PrerequisiteUID: first}, repaired.Author[0].Payload)
	require.Len(t, repaired.Restore, 1)
	assert.Equal(t, candidates[0].Value, repaired.Restore[0].Value)
	assert.Equal(t, soleConstraintAssignmentID(t, conflict, Unit{Kind: boardhistory.EventIssueDependency, IssueUID: first, RelatedUID: second}), repaired.Restore[0].AlternativeID)
	assert.Len(t, snapshot.Supported.Dependencies, 1)
	assert.True(t, candidates[1].Value.(*boardhistory.IssueDependency).Present)
	// The proposed edge must also remain compatible with unrelated supported edges.
	snapshot.Supported.Dependencies = append(snapshot.Supported.Dependencies, planning.RelationDependency{IssueUID: second, PrerequisiteUID: third})
	_, err = conflict.ResolveRelations(request, snapshot)
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestConstraintConflictSupportsCompoundParentRepairAndUnparent(t *testing.T) {
	first, second, outside := constraintUID(t, 1), constraintUID(t, 2), constraintUID(t, 3)
	origins := concurrentOrigins(t, 2)
	candidates := []Candidate{
		{Origins: origins[:1], Value: &boardhistory.IssueContainment{ChildUID: first, ParentUID: second, Present: true}},
		{Origins: origins[1:], Value: &boardhistory.IssueContainment{ChildUID: second, ParentUID: first, Present: true}},
	}
	conflict, err := NewConstraintConflict("board-test", candidateSequence(candidates))
	require.NoError(t, err)
	repaired, err := conflict.ResolveRelations(ResolveRelations{
		ConflictID: conflict.ID(), OccurredAt: time.Unix(10, 0),
		Parents: []ParentChoice{{ChildUID: first, ParentUID: &outside}, {ChildUID: second}},
	}, RelationsSnapshot{Assignments: retainedConstraintAssignments(t, conflict, candidates), Supported: planning.Relations{Issues: []planning.RelationIssue{{UID: first}, {UID: second}, {UID: outside}}}})
	require.NoError(t, err)
	assert.Empty(t, repaired.Restore)
	payloads := make([]boardhistory.Payload, len(repaired.Author))
	for index, event := range repaired.Author {
		payloads[index] = event.Payload
	}
	assert.ElementsMatch(t, []boardhistory.Payload{&boardhistory.IssueContainment{ChildUID: first, ParentUID: outside, Present: true}, &boardhistory.IssueContainment{ChildUID: second, ParentUID: first}}, payloads)
}

func TestConstraintConflictRestoresEdgeAfterKnownParentKindChanges(t *testing.T) {
	child, parent := constraintUID(t, 1), constraintUID(t, 2)
	candidates := []Candidate{{Origins: concurrentOrigins(t, 1), Value: &boardhistory.IssueContainment{ChildUID: child, ParentUID: parent, Present: true}}}
	conflict, err := NewConstraintConflict("board-test", candidateSequence(candidates))
	require.NoError(t, err)
	snapshot := RelationsSnapshot{Assignments: retainedConstraintAssignments(t, conflict, candidates), Supported: planning.Relations{Issues: []planning.RelationIssue{
		{UID: child, Lifecycle: new(issue.LifecycleOpen)}, {UID: parent, Kind: new(issue.KindTask), Lifecycle: new(issue.LifecycleOpen)},
	}}}
	request := ResolveRelations{ConflictID: conflict.ID(), OccurredAt: time.Unix(10, 0)}
	_, err = conflict.ResolveRelations(request, snapshot)
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	snapshot.Supported.Issues[1].Kind = new(issue.KindWorkstream)
	repaired, err := conflict.ResolveRelations(request, snapshot)
	require.NoError(t, err)
	assert.Empty(t, repaired.Author)
	assert.Equal(t, snapshot.Assignments, repaired.Restore)
}

func TestConstraintConflictExecutionRepairPreservesClosureOrWaiting(t *testing.T) {
	uid := constraintUID(t, 1)
	origins := concurrentOrigins(t, 2)
	closedAt := time.Unix(4, 0).UTC()
	waiting := &issue.WaitingState{Reason: "External review", Since: time.Unix(3, 0).UTC()}
	candidates := []Candidate{
		{Origins: origins[:1], Value: &boardhistory.IssueLifecycleSet{IssueUID: uid, Lifecycle: "closed", ClosedAt: &closedAt}},
		{Origins: origins[1:], Value: &boardhistory.IssueWaitingSet{IssueUID: uid, Waiting: waiting}},
	}
	conflict, err := NewConstraintConflict("board-test", candidateSequence(candidates))
	require.NoError(t, err)
	snapshot := ExecutionSnapshot{Reader: newExecutionAssignmentReader(t, conflict, candidates), Kind: new(issue.KindTask)}
	closed, err := conflict.ResolveExecution(t.Context(), ResolveExecution{ConflictID: conflict.ID(), Waiting: ClearWaiting, OccurredAt: time.Unix(10, 0)}, snapshot)
	require.NoError(t, err)
	require.Len(t, closed.Author, 1)
	assert.Equal(t, &boardhistory.IssueWaitingSet{IssueUID: uid}, closed.Author[0].Payload)
	require.Len(t, closed.Restore, 1)
	assert.Equal(t, candidates[0].Value, closed.Restore[0].Value)
	_, err = conflict.ResolveExecution(t.Context(), ResolveExecution{ConflictID: conflict.ID(), Waiting: KeepWaiting, OccurredAt: time.Unix(10, 0)}, snapshot)
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	reopened, err := conflict.ResolveExecution(t.Context(), ResolveExecution{ConflictID: conflict.ID(), Waiting: KeepWaiting, Lifecycle: new(issue.LifecycleOpen), OccurredAt: time.Unix(10, 0)}, snapshot)
	require.NoError(t, err)
	require.Len(t, reopened.Author, 1)
	assert.Equal(t, &boardhistory.IssueLifecycleSet{IssueUID: uid, Lifecycle: "open"}, reopened.Author[0].Payload)
	require.Len(t, reopened.Restore, 1)
	assert.Equal(t, candidates[1].Value, reopened.Restore[0].Value)
}

func TestConstraintConflictExecutionRepairRequiresChosenExecutableKind(t *testing.T) {
	uid := constraintUID(t, 1)
	origins := concurrentOrigins(t, 2)
	candidates := []Candidate{
		{Origins: origins[:1], Value: &boardhistory.IssueKindSet{IssueUID: uid, Kind: "checkpoint"}},
		{Origins: origins[1:], Value: &boardhistory.IssueWaitingSet{IssueUID: uid, Waiting: &issue.WaitingState{Reason: "External review", Since: time.Unix(3, 0).UTC()}}},
	}
	conflict, err := NewConstraintConflict("board-test", candidateSequence(candidates))
	require.NoError(t, err)
	snapshot := ExecutionSnapshot{Reader: newExecutionAssignmentReader(t, conflict, candidates), Lifecycle: new(issue.LifecycleOpen)}
	request := ResolveExecution{ConflictID: conflict.ID(), Waiting: KeepWaiting, OccurredAt: time.Unix(10, 0)}
	_, err = conflict.ResolveExecution(t.Context(), request, snapshot)
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	request.Kind = new(issue.KindRoutine)
	repaired, err := conflict.ResolveExecution(t.Context(), request, snapshot)
	require.NoError(t, err)
	require.Len(t, repaired.Author, 1)
	assert.Equal(t, &boardhistory.IssueKindSet{IssueUID: uid, Kind: "routine"}, repaired.Author[0].Payload)
	assert.Len(t, repaired.Restore, 1)
	// Unrelated scalar uncertainty remains unknown; this repair cannot choose it.
	snapshot.Lifecycle = nil
	request.Lifecycle = new(issue.LifecycleOpen)
	_, err = conflict.ResolveExecution(t.Context(), request, snapshot)
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	request.Lifecycle = nil
	repaired, err = conflict.ResolveExecution(t.Context(), request, snapshot)
	require.NoError(t, err)
	require.Len(t, repaired.Author, 1)
	assert.Equal(t, &boardhistory.IssueKindSet{IssueUID: uid, Kind: "routine"}, repaired.Author[0].Payload)
	assert.Nil(t, snapshot.Lifecycle)
}

func TestConstraintConflictRepairsKindContendersWithWaiting(t *testing.T) {
	uid := constraintUID(t, 1)
	origins := concurrentOrigins(t, 3)
	waiting := &issue.WaitingState{Reason: "External review", Since: time.Unix(3, 0).UTC()}
	candidates := []Candidate{
		{Origins: origins[:1], Value: &boardhistory.IssueKindSet{IssueUID: uid, Kind: "checkpoint"}},
		{Origins: origins[1:2], Value: &boardhistory.IssueKindSet{IssueUID: uid, Kind: "workstream"}},
		{Origins: origins[2:], Value: &boardhistory.IssueWaitingSet{IssueUID: uid, Waiting: waiting}},
	}
	conflict, err := NewConstraintConflict("board-test", candidateSequence(candidates))
	require.NoError(t, err)
	require.Len(t, conflict.Assignments(), 3)
	snapshot := ExecutionSnapshot{Reader: newExecutionAssignmentReader(t, conflict, candidates), Lifecycle: new(issue.LifecycleOpen)}
	_, err = conflict.ResolveExecution(t.Context(), ResolveExecution{ConflictID: conflict.ID(), Waiting: ClearWaiting, OccurredAt: time.Unix(10, 0)}, snapshot)
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	workstream, err := conflict.ResolveExecution(t.Context(), ResolveExecution{ConflictID: conflict.ID(), Waiting: KeepWaiting, Kind: new(issue.KindWorkstream), OccurredAt: time.Unix(10, 0)}, snapshot)
	require.NoError(t, err)
	require.Len(t, workstream.Author, 1)
	assert.Equal(t, &boardhistory.IssueKindSet{IssueUID: uid, Kind: "workstream"}, workstream.Author[0].Payload)
	require.Len(t, workstream.Restore, 1)
	assert.Equal(t, candidates[2].Value, workstream.Restore[0].Value)
	checkpoint, err := conflict.ResolveExecution(t.Context(), ResolveExecution{ConflictID: conflict.ID(), Waiting: ClearWaiting, Kind: new(issue.KindCheckpoint), OccurredAt: time.Unix(10, 0)}, snapshot)
	require.NoError(t, err)
	require.Len(t, checkpoint.Author, 2)
	assert.Equal(t, &boardhistory.IssueKindSet{IssueUID: uid, Kind: "checkpoint"}, checkpoint.Author[0].Payload)
	assert.Equal(t, &boardhistory.IssueWaitingSet{IssueUID: uid}, checkpoint.Author[1].Payload)
	assert.Empty(t, checkpoint.Restore)
}

func TestConstraintConflictRequiresReviewedWaitingChoice(t *testing.T) {
	uid := constraintUID(t, 1)
	origins := concurrentOrigins(t, 4)
	candidates := []Candidate{
		{Origins: origins[:1], Value: &boardhistory.IssueKindSet{IssueUID: uid, Kind: "checkpoint"}},
		{Origins: origins[1:2], Value: &boardhistory.IssueWaitingSet{IssueUID: uid, Waiting: &issue.WaitingState{Reason: "First reviewer", Since: time.Unix(3, 0).UTC()}}},
		{Origins: origins[2:3], Value: &boardhistory.IssueWaitingSet{IssueUID: uid, Waiting: &issue.WaitingState{Reason: "Second reviewer", Since: time.Unix(4, 0).UTC()}}},
		{Origins: origins[3:], Value: &boardhistory.IssueWaitingSet{IssueUID: uid}},
	}
	conflict, err := NewConstraintConflict("board-test", candidateSequence(candidates))
	require.NoError(t, err)
	snapshot := ExecutionSnapshot{Reader: newExecutionAssignmentReader(t, conflict, candidates), Lifecycle: new(issue.LifecycleOpen)}
	request := ResolveExecution{ConflictID: conflict.ID(), Waiting: KeepWaiting, Kind: new(issue.KindTask), OccurredAt: time.Unix(10, 0)}
	_, err = conflict.ResolveExecution(t.Context(), request, snapshot)
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	for _, assignment := range conflict.Assignments() {
		if slices.Contains(assignment.Origins, origins[2]) {
			request.WaitingAssignment = assignment.AlternativeID
		}
	}
	repaired, err := conflict.ResolveExecution(t.Context(), request, snapshot)
	require.NoError(t, err)
	require.Len(t, repaired.Author, 2)
	assert.Equal(t, candidates[2].Value, repaired.Author[1].Payload)
	assert.Empty(t, repaired.Restore)
}

func TestConstraintConflictParentChoiceAuthorsOneAssignmentPerChild(t *testing.T) {
	child, first, second := constraintUID(t, 1), constraintUID(t, 2), constraintUID(t, 3)
	origins := concurrentOrigins(t, 3)
	candidates := []Candidate{
		{Origins: origins[:1], Value: &boardhistory.IssueContainment{ChildUID: child, ParentUID: first, Present: true}},
		{Origins: origins[1:2], Value: &boardhistory.IssueContainment{ChildUID: child, ParentUID: second, Present: true}},
		{Origins: origins[2:], Value: &boardhistory.IssueContainment{ChildUID: second, ParentUID: child, Present: true}},
	}
	conflict, err := NewConstraintConflict("board-test", candidateSequence(candidates))
	require.NoError(t, err)
	repaired, err := conflict.ResolveRelations(ResolveRelations{ConflictID: conflict.ID(), Parents: []ParentChoice{{ChildUID: child, ParentUID: &first}, {ChildUID: second}}, OccurredAt: time.Unix(10, 0)}, RelationsSnapshot{
		Assignments: retainedConstraintAssignments(t, conflict, candidates),
		Supported:   planning.Relations{Issues: []planning.RelationIssue{{UID: child}, {UID: first}, {UID: second}}},
	})
	require.NoError(t, err)
	require.Len(t, repaired.Author, 2)
	assert.Empty(t, repaired.Restore)
	payloads := make([]boardhistory.Payload, len(repaired.Author))
	for index, event := range repaired.Author {
		payloads[index] = event.Payload
	}
	assert.ElementsMatch(t, []boardhistory.Payload{&boardhistory.IssueContainment{ChildUID: child, ParentUID: first, Present: true}, &boardhistory.IssueContainment{ChildUID: second, ParentUID: child}}, payloads)
}

func TestConstraintConflictKeepingSolePositiveWaitingResolvesAbsence(t *testing.T) {
	uid := constraintUID(t, 1)
	origins := concurrentOrigins(t, 3)
	waiting := &issue.WaitingState{Reason: "External review", Since: time.Unix(3, 0).UTC()}
	candidates := []Candidate{
		{Origins: origins[:1], Value: &boardhistory.IssueKindSet{IssueUID: uid, Kind: "checkpoint"}},
		{Origins: origins[1:2], Value: &boardhistory.IssueWaitingSet{IssueUID: uid, Waiting: waiting}},
		{Origins: origins[2:], Value: &boardhistory.IssueWaitingSet{IssueUID: uid}},
	}
	conflict, err := NewConstraintConflict("board-test", candidateSequence(candidates))
	require.NoError(t, err)
	reader := newExecutionAssignmentReader(t, conflict, candidates)
	snapshot := ExecutionSnapshot{Reader: reader, Lifecycle: new(issue.LifecycleOpen)}
	request := ResolveExecution{ConflictID: conflict.ID(), Waiting: KeepWaiting, Kind: new(issue.KindWorkstream), OccurredAt: time.Unix(10, 0)}
	repaired, err := conflict.ResolveExecution(t.Context(), request, snapshot)
	require.NoError(t, err)
	require.Len(t, repaired.Author, 2)
	assert.Equal(t, candidates[1].Value, repaired.Author[1].Payload)
	assert.Empty(t, repaired.Restore)
	// The repair reads the singleton kind and sole positive waiting value;
	// the absent waiting contender needs no body read.
	require.Len(t, reader.read, 2)
	for _, assignment := range conflict.Assignments() {
		if assignment.Unit.Kind == boardhistory.EventIssueWaitingSet {
			assert.Equal(t, assignment.WaitingPresent, slices.Contains(reader.read, assignment.AlternativeID))
		}
	}
	reader.read = nil
	request.ConflictID = "superseded-conflict"
	_, err = conflict.ResolveExecution(t.Context(), request, snapshot)
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.Empty(t, reader.read)
}

func TestConstraintExecutionLoadsOnlySelectedWaitingBody(t *testing.T) {
	uid := constraintUID(t, 1)
	origins := concurrentOrigins(t, 5)
	candidates := []Candidate{
		{Origins: origins[:1], Value: &boardhistory.IssueKindSet{IssueUID: uid, Kind: "checkpoint"}},
		{Origins: origins[1:2], Value: &boardhistory.IssueKindSet{IssueUID: uid, Kind: "workstream"}},
		{Origins: origins[2:3], Value: &boardhistory.IssueWaitingSet{IssueUID: uid, Waiting: &issue.WaitingState{Reason: "First reviewer", Since: time.Unix(3, 0).UTC()}}},
		{Origins: origins[3:4], Value: &boardhistory.IssueWaitingSet{IssueUID: uid, Waiting: &issue.WaitingState{Reason: "Second reviewer", Since: time.Unix(4, 0).UTC()}}},
		{Origins: origins[4:], Value: &boardhistory.IssueWaitingSet{IssueUID: uid}},
	}
	conflict, err := NewConstraintConflict("board-test", candidateSequence(candidates))
	require.NoError(t, err)
	reader := newExecutionAssignmentReader(t, conflict, candidates)
	snapshot := ExecutionSnapshot{Reader: reader, Lifecycle: new(issue.LifecycleOpen)}
	request := ResolveExecution{ConflictID: conflict.ID(), Waiting: ClearWaiting, Kind: new(issue.KindCheckpoint), OccurredAt: time.Unix(10, 0)}
	repair, err := conflict.ResolveExecution(t.Context(), request, snapshot)
	require.NoError(t, err)
	require.Len(t, repair.Author, 2)
	assert.Empty(t, reader.read)
	request.Waiting, request.Kind = KeepWaiting, new(issue.KindWorkstream)
	_, err = conflict.ResolveExecution(t.Context(), request, snapshot)
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	assert.Empty(t, reader.read)
	for _, assignment := range conflict.Assignments() {
		if slices.Contains(assignment.Origins, origins[3]) {
			assert.True(t, assignment.WaitingPresent)
			request.WaitingAssignment = assignment.AlternativeID
		}
		if slices.Contains(assignment.Origins, origins[4]) {
			assert.False(t, assignment.WaitingPresent)
		}
	}
	repair, err = conflict.ResolveExecution(t.Context(), request, snapshot)
	require.NoError(t, err)
	assert.Equal(t, []AssignmentID{request.WaitingAssignment}, reader.read)
	require.Len(t, repair.Author, 2)
	assert.Equal(t, candidates[3].Value, repair.Author[1].Payload)
	reader.err = errors.New("retained event unavailable")
	repair, err = conflict.ResolveExecution(t.Context(), request, snapshot)
	assert.ErrorIs(t, err, reader.err)
	assert.Empty(t, repair.Author)
	assert.Empty(t, repair.Restore)
	reader.read = nil
	request.Waiting, request.WaitingAssignment = ClearWaiting, ""
	_, err = conflict.ResolveExecution(t.Context(), request, snapshot)
	require.NoError(t, err)
	assert.Empty(t, reader.read)
}

type executionAssignmentReader struct {
	values map[AssignmentID]boardhistory.Payload
	read   []AssignmentID
	err    error
}

func newExecutionAssignmentReader(t *testing.T, conflict *ConstraintConflict, candidates []Candidate) *executionAssignmentReader {
	t.Helper()
	reader := &executionAssignmentReader{values: make(map[AssignmentID]boardhistory.Payload)}
	for _, assignment := range retainedConstraintAssignments(t, conflict, candidates) {
		reader.values[assignment.AlternativeID] = assignment.Value
	}
	return reader
}

func (r *executionAssignmentReader) ReadExecutionAssignment(_ context.Context, assignment AssignmentReference) (boardhistory.Payload, error) {
	r.read = append(r.read, assignment.AlternativeID)
	if r.err != nil {
		return nil, r.err
	}
	return r.values[assignment.AlternativeID], nil
}

func constraintUID(t *testing.T, value byte) issue.UID {
	t.Helper()
	uid, err := issue.GenerateUID(bytes.NewReader(bytes.Repeat([]byte{value}, 16)))
	require.NoError(t, err)
	return uid
}

// retainedConstraintAssignments models separately loading the reviewed journal bodies.
func retainedConstraintAssignments(t *testing.T, conflict *ConstraintConflict, candidates []Candidate) []RetainedAssignment {
	t.Helper()
	result := make([]RetainedAssignment, 0, len(candidates))
	for _, assignment := range conflict.Assignments() {
		for _, candidate := range candidates {
			unit, err := UnitOf(candidate.Value)
			require.NoError(t, err)
			if unit == assignment.Unit && slices.Contains(candidate.Origins, assignment.Origins[0]) {
				result = append(result, RetainedAssignment{AlternativeID: assignment.AlternativeID, Value: candidate.Value})
				break
			}
		}
	}
	require.Len(t, result, len(conflict.Assignments()))
	return result
}

// soleConstraintAssignmentID selects a unit only when it has one reviewed value.
func soleConstraintAssignmentID(t *testing.T, conflict *ConstraintConflict, unit Unit) AssignmentID {
	t.Helper()
	var selected AssignmentID
	for _, assignment := range conflict.Assignments() {
		if assignment.Unit != unit {
			continue
		}
		require.Empty(t, selected, "unit has several values; select the intended origin")
		selected = assignment.AlternativeID
	}
	require.NotEmpty(t, selected, "constraint has no assignment for requested unit")
	return selected
}
