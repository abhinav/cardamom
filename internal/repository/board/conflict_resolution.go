package board

import (
	"context"
	"database/sql"
	"errors"

	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
	repositoryhistory "go.abhg.dev/cardamom/internal/repository/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ResolveConflict applies a reviewed scalar choice in the same transaction that
// checks its identity. The chosen retained body is the only alternative loaded.
func (r *Repository) ResolveConflict(ctx context.Context, request boardconflict.ResolveRequest) (out boardconflict.Resolution, err error) {
	snapshot, err := r.beginConflictResolution(ctx, request.ConflictID)
	if err != nil {
		return out, err
	}
	m := snapshot.Mutation
	defer func() { err = errors.Join(err, m.change.Done()) }()
	conflict, ok := snapshot.Conflict.(*boardconflict.Conflict)
	if !ok {
		return out, errkind.Errorf(errkind.InvalidInput, "constraint conflicts require a relationship or execution repair")
	}
	var selected boardhistory.Payload
	if choice, ok := request.Choice.(boardconflict.TakeAlternative); ok {
		selected, err = r.readAlternative(ctx, m.change, request.ConflictID, choice.ID)
		if err != nil {
			return out, err
		}
	}
	event, err := conflict.Resolve(request, selected, m.occurredAt)
	if err != nil {
		return out, err
	}
	return r.commitConflictResolution(ctx, m, conflict, boardconflict.ConstraintRepair{Author: []boardhistory.Event{event}})
}

// RepairRelations applies only reviewed edge removals and parent choices, then
// publishes the graph validated by the shared containment/dependency policy.
func (r *Repository) RepairRelations(ctx context.Context, request boardconflict.RelationsRequest) (out boardconflict.Resolution, err error) {
	snapshot, err := r.beginConflictResolution(ctx, request.ConflictID)
	if err != nil {
		return out, err
	}
	m := snapshot.Mutation
	defer func() { err = errors.Join(err, m.change.Done()) }()
	conflict, ok := snapshot.Conflict.(*boardconflict.ConstraintConflict)
	if !ok {
		return out, errkind.Errorf(errkind.InvalidInput, "field conflicts require a field choice")
	}
	var retained []boardconflict.RetainedAssignment
	for _, assignment := range conflict.Assignments() {
		value, err := r.readAlternative(ctx, m.change, request.ConflictID, string(assignment.AlternativeID))
		if err != nil {
			return out, err
		}
		retained = append(retained, boardconflict.RetainedAssignment{AlternativeID: assignment.AlternativeID, Value: value})
	}
	graph, err := r.readRelations(ctx, m.change)
	if err != nil {
		return out, err
	}
	command := boardconflict.ResolveRelations{ConflictID: request.ConflictID, Remove: request.Remove, OccurredAt: m.occurredAt}
	for _, selection := range request.Parents {
		child, err := r.readIssueUID(ctx, m.change, selection.ChildID)
		if err != nil {
			return out, err
		}
		parent := boardconflict.ParentChoice{ChildUID: child}
		if selection.ParentID != nil {
			uid, err := r.readIssueUID(ctx, m.change, *selection.ParentID)
			if err != nil {
				return out, err
			}
			parent.ParentUID = &uid
		}
		command.Parents = append(command.Parents, parent)
	}
	repair, err := conflict.ResolveRelations(command, boardconflict.RelationsSnapshot{Assignments: retained, Supported: graph})
	if err != nil {
		return out, err
	}
	return r.commitConflictResolution(ctx, m, conflict, repair)
}

// RepairExecution selects a coherent kind/lifecycle/waiting tuple. Waiting
// reasons remain in history unless the chosen outcome needs one of them.
func (r *Repository) RepairExecution(ctx context.Context, request boardconflict.ExecutionRequest) (out boardconflict.Resolution, err error) {
	snapshot, err := r.beginConflictResolution(ctx, request.ConflictID)
	if err != nil {
		return out, err
	}
	m := snapshot.Mutation
	defer func() { err = errors.Join(err, m.change.Done()) }()
	conflict, ok := snapshot.Conflict.(*boardconflict.ConstraintConflict)
	if !ok {
		return out, errkind.Errorf(errkind.InvalidInput, "field conflicts require a field choice")
	}
	graph, err := r.readRelations(ctx, m.change)
	if err != nil {
		return out, err
	}
	var current planning.RelationIssue
	for _, value := range graph.Issues {
		if value.UID == conflict.Assignments()[0].Unit.IssueUID {
			current = value
			break
		}
	}
	repair, err := conflict.ResolveExecution(ctx, boardconflict.ResolveExecution{
		ConflictID: request.ConflictID, Waiting: request.Waiting, WaitingAssignment: request.WaitingAssignment,
		Kind: request.Kind, Lifecycle: request.Lifecycle, OccurredAt: m.occurredAt,
	}, boardconflict.ExecutionSnapshot{
		Kind: current.Kind, Lifecycle: current.Lifecycle,
		Reader: &executionAssignmentReader{Repository: r, Scope: m.change, ConflictID: request.ConflictID},
	})
	if err != nil {
		return out, err
	}
	return r.commitConflictResolution(ctx, m, conflict, repair)
}

// conflictResolution retains the reviewed entry and its writer transaction.
// Archive resolution is admitted through this boundary even while archive state
// is unknown; other writes retain the ordinary board lifecycle prerequisite.
type conflictResolution struct {
	Mutation *mutation
	Conflict boardconflict.Entry
}

func (r *Repository) beginConflictResolution(ctx context.Context, id string) (out conflictResolution, err error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return out, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, change.Done())
		}
	}()
	entry, err := r.readConflictEntry(ctx, change, id)
	if err != nil {
		return out, err
	}
	queries := query.New(change)
	row, err := queries.BoardGetRevision(ctx, r.boardID.String())
	if err != nil {
		return out, err
	}
	field, scalar := entry.(*boardconflict.Conflict)
	archiveResolution := scalar && field.Unit().Kind == boardhistory.EventBoardArchiveSet
	if !archiveResolution {
		unresolved, err := queries.BoardReadArchiveConflict(ctx, r.boardID.String())
		if err == nil {
			return out, errkind.Errorf(errkind.Conflict, "board archive state is unresolved in conflict %s", unresolved)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		if row.ArchivedAt != nil {
			return out, domainboard.ErrArchived
		}
	}
	out.Conflict = entry
	out.Mutation = &mutation{
		repository: r, change: change, current: domainboard.Revision(row.Revision),
		occurredAt: r.clock.Now(), history: repositoryhistory.NewAppender(ctx, change, r.boardID),
	}
	return out, nil
}

// commitConflictResolution separates authoring from restoring previously
// obscured values. Only chosen semantic changes enter the local journal.
func (r *Repository) commitConflictResolution(ctx context.Context, m *mutation, entry boardconflict.Entry, repair boardconflict.ConstraintRepair) (boardconflict.Resolution, error) {
	var out boardconflict.Resolution
	if err := m.reserve(ctx); err != nil {
		return out, err
	}
	queries := query.New(m.change)
	affected, err := queries.BoardReadConflictIssueReferences(ctx, entry.ID())
	if err != nil {
		return out, err
	}
	if err := queries.BoardDeleteConflict(ctx, query.BoardDeleteConflictParams{BoardID: r.boardID.String(), ID: entry.ID()}); err != nil {
		return out, err
	}
	for _, assignment := range repair.Restore {
		unit, err := boardconflict.UnitOf(assignment.Value)
		if err != nil {
			return out, err
		}
		if err := r.writeConflictProjection(ctx, queries, unit, assignment.Value, nil, m.reservation.Revision()); err != nil {
			return out, err
		}
	}
	for _, event := range repair.Author {
		unit, err := boardconflict.UnitOf(event.Payload)
		if err != nil {
			return out, err
		}
		if err := r.writeConflictProjection(ctx, queries, unit, event.Payload, &event.OccurredAt, m.reservation.Revision()); err != nil {
			return out, err
		}
		m.record(event)
	}
	if resolutionNeedsRelations(entry) {
		graph, err := r.readRelations(ctx, m.change)
		if err != nil {
			return out, err
		}
		if err := planning.ValidateRelations(graph); err != nil {
			return out, err
		}
	}
	ids := make([]issue.ID, 0, len(affected))
	for _, row := range affected {
		ids = append(ids, issue.ID(row.ID))
	}
	if m.hasHistory() {
		err = m.commit(ctx, ids...)
	} else {
		err = m.commitProjection(ctx, ids...)
	}
	if err != nil {
		return out, err
	}
	return boardconflict.Resolution{ConflictID: entry.ID(), Revision: m.reservation.Revision()}, nil
}

type executionAssignmentReader struct {
	Repository *Repository
	Scope      queryScope
	ConflictID string
}

func (r *executionAssignmentReader) ReadExecutionAssignment(ctx context.Context, assignment boardconflict.AssignmentReference) (boardhistory.Payload, error) {
	return r.Repository.readAlternative(ctx, r.Scope, r.ConflictID, string(assignment.AlternativeID))
}

// Relationship repairs already validate their proposed graph in domain policy.
// Other resolutions consume graph policy only when they change its inputs.
func resolutionNeedsRelations(entry boardconflict.Entry) bool {
	switch conflict := entry.(type) {
	case *boardconflict.ConstraintConflict:
		return conflict.Kind() == boardconflict.ConstraintExecution
	case *boardconflict.Conflict:
		switch conflict.Unit().Kind {
		case boardhistory.EventIssueKindSet, boardhistory.EventIssueLifecycleSet, boardhistory.EventIssueDependency, boardhistory.EventIssueContainment:
			return true
		}
	}
	return false
}
