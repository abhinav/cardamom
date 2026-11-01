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
	repositoryhistory "go.abhg.dev/cardamom/internal/repository/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ListConflicts returns one bounded page of compact conflict metadata.
// Alternative bodies are fetched separately from retained history.
func (r *Repository) ListConflicts(ctx context.Context, request boardconflict.ListRequest) (out boardconflict.Page, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	var uid []byte
	if request.IssueID != "" {
		value, err := r.readIssueUID(ctx, view, issue.ID(request.IssueID))
		if err != nil {
			return out, err
		}
		uid = value.Bytes()
	}
	rows, err := query.New(view).BoardListConflicts(ctx, query.BoardListConflictsParams{
		BoardID: r.boardID.String(), AfterID: request.After, IssueUid: uid, PageLimit: int64(request.Limit + 1),
	})
	if err != nil {
		return out, err
	}
	if len(rows) > request.Limit {
		rows = rows[:request.Limit]
		out.Next = rows[len(rows)-1].ID
	}
	presentation, err := r.readIssuePresentation(ctx, view)
	if err != nil {
		return out, err
	}
	out.Conflicts = make([]boardconflict.Summary, 0, len(rows))
	for _, row := range rows {
		unit, err := storedConflictUnit(row.Kind, row.IssueUid, row.RelatedUid, row.MemberKey)
		if err != nil {
			return out, err
		}
		refs, err := r.readConflictIssueReferences(ctx, view, presentation, row.ID)
		if err != nil {
			return out, err
		}
		out.Conflicts = append(out.Conflicts, boardconflict.Summary{
			ID: row.ID, Unit: unit, ConstraintKind: storedConstraintKind(row.Category), Issues: refs, Alternatives: int(row.AlternativeCount),
		})
	}
	return out, nil
}

// ReadConflict returns compact identities for one unresolved field from a
// coherent read snapshot. Candidate bodies remain in the retained journal.
func (r *Repository) ReadConflict(ctx context.Context, id string) (out boardconflict.Inspection, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	out.Conflict, err = r.readConflictEntry(ctx, view, id)
	if err != nil {
		return out, err
	}
	presentation, err := r.readIssuePresentation(ctx, view)
	if err != nil {
		return out, err
	}
	rows, err := query.New(view).BoardReadConflictIssueReferences(ctx, id)
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		uid, err := issue.ParseUID(row.Uid)
		if err != nil {
			return out, err
		}
		reference, err := presentation.Reference(issue.ID(row.ID))
		if err != nil {
			return out, err
		}
		out.Issues = append(out.Issues, boardconflict.IssueReference{UID: uid, Reference: reference})
	}
	return out, nil
}

func (r *Repository) readConflictEntry(ctx context.Context, scope queryScope, id string) (boardconflict.Entry, error) {
	queries := query.New(scope)
	row, err := queries.BoardGetConflict(ctx, query.BoardGetConflictParams{BoardID: r.boardID.String(), ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errkind.Errorf(errkind.Conflict, "conflict %s changed or was resolved; inspect current conflicts", id)
	}
	if err != nil {
		return nil, err
	}
	values, err := queries.BoardReadConflictAlternatives(ctx, id)
	if err != nil {
		return nil, err
	}
	origins, err := queries.BoardReadConflictOrigins(ctx, id)
	if err != nil {
		return nil, err
	}
	byAlternative := make(map[string][]boardhistory.EventID)
	for _, origin := range origins {
		writer, err := domainboard.ParseWriterUID(origin.WriterUid)
		if err != nil {
			return nil, err
		}
		byAlternative[origin.AlternativeID] = append(byAlternative[origin.AlternativeID], boardhistory.EventID{
			Writer: writer, Sequence: uint64(origin.Sequence), Ordinal: uint64(origin.Ordinal),
		})
	}
	if kind := storedConstraintKind(row.Category); kind != 0 {
		assignments := make([]boardconflict.AssignmentReference, 0, len(values))
		for _, value := range values {
			unit, err := storedConflictUnit(value.Kind, value.IssueUid, value.RelatedUid, value.MemberKey)
			if err != nil {
				return nil, err
			}
			assignments = append(assignments, boardconflict.AssignmentReference{Unit: unit, AlternativeID: boardconflict.AssignmentID(value.AlternativeID), Origins: byAlternative[value.AlternativeID], WaitingPresent: value.WaitingPresent})
		}
		return boardconflict.LoadConstraint(boardconflict.ConstraintSnapshot{ID: id, BoardID: r.boardID, Kind: kind, Assignments: assignments}), nil
	}
	unit, err := storedConflictUnit(row.Kind, row.IssueUid, row.RelatedUid, row.MemberKey)
	if err != nil {
		return nil, err
	}
	alternatives := make([]boardconflict.Alternative, 0, len(values))
	for _, value := range values {
		alternatives = append(alternatives, boardconflict.Alternative{ID: value.AlternativeID, Origins: byAlternative[value.AlternativeID]})
	}
	affectedRows, err := queries.BoardReadConflictAffectedIssues(ctx, id)
	if err != nil {
		return nil, err
	}
	var affected []boardconflict.AffectedIssue
	for _, affectedRow := range affectedRows {
		uid, err := issue.ParseUID(affectedRow.IssueUid)
		if err != nil {
			return nil, err
		}
		field, err := issue.ParseField(affectedRow.Field)
		if err != nil {
			return nil, err
		}
		affected = append(affected, boardconflict.AffectedIssue{UID: uid, Field: field})
	}
	return boardconflict.Load(boardconflict.Snapshot{ID: id, BoardID: r.boardID, Unit: unit, Alternatives: alternatives, Affected: affected}), nil
}

func storedConflictUnit(kind string, uid, related []byte, key string) (boardconflict.Unit, error) {
	unit := boardconflict.Unit{Kind: boardhistory.EventKind(kind), Key: key}
	var err error
	if uid != nil {
		unit.IssueUID, err = issue.ParseUID(uid)
		if err != nil {
			return unit, err
		}
	}
	if related != nil {
		unit.RelatedUID, err = issue.ParseUID(related)
		if err != nil {
			return unit, err
		}
	}
	return unit, nil
}

func (r *Repository) readConflictIssueReferences(ctx context.Context, scope queryScope, presentation *issuePresentation, id string) ([]issue.Reference, error) {
	rows, err := query.New(scope).BoardReadConflictIssueReferences(ctx, id)
	if err != nil {
		return nil, err
	}
	refs := make([]issue.Reference, 0, len(rows))
	for _, row := range rows {
		reference, err := presentation.Reference(issue.ID(row.ID))
		if err != nil {
			return nil, err
		}
		refs = append(refs, reference)
	}
	return refs, nil
}

// ReadAlternative reads one body without materializing other contenders.
func (r *Repository) ReadAlternative(ctx context.Context, conflictID, alternativeID string) (out boardconflict.Value, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	out.Payload, err = r.readAlternative(ctx, view, conflictID, alternativeID)
	if err != nil {
		return out, err
	}
	presentation, err := r.readIssuePresentation(ctx, view)
	if err != nil {
		return out, err
	}
	for _, uid := range assignmentIssueUIDs(out.Payload) {
		id, err := query.New(view).BoardGetIssueIDByUID(ctx, query.BoardGetIssueIDByUIDParams{BoardID: r.boardID.String(), IssueUid: uid.Bytes()})
		if err != nil {
			return out, err
		}
		reference, err := presentation.Reference(issue.ID(id))
		if err != nil {
			return out, err
		}
		out.Issues = append(out.Issues, boardconflict.IssueReference{UID: uid, Reference: reference})
	}
	return out, nil
}

func (r *Repository) readAlternative(ctx context.Context, scope queryScope, conflictID, alternativeID string) (boardhistory.Payload, error) {
	row, err := query.New(scope).BoardReadConflictAlternativeOrigin(ctx, query.BoardReadConflictAlternativeOriginParams{
		BoardID: r.boardID.String(), ConflictID: conflictID, AlternativeID: alternativeID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errkind.Errorf(errkind.Conflict, "conflict or alternative changed; inspect current conflicts")
	}
	if err != nil {
		return nil, err
	}
	writer, err := domainboard.ParseWriterUID(row.WriterUid)
	if err != nil {
		return nil, err
	}
	unit, err := storedConflictUnit(row.Kind, row.IssueUid, row.RelatedUid, row.MemberKey)
	if err != nil {
		return nil, err
	}
	event, err := repositoryhistory.ReadEvent(ctx, query.New(scope), r.boardID, boardhistory.EventID{
		Writer: writer, Sequence: uint64(row.Sequence), Ordinal: uint64(row.Ordinal),
	})
	if err != nil {
		return nil, err
	}
	return boardconflict.ProjectAssignment(event.Payload, unit)
}

func storedConstraintKind(category string) boardconflict.ConstraintKind {
	switch category {
	case "relationships":
		return boardconflict.ConstraintRelationships
	case "execution":
		return boardconflict.ConstraintExecution
	default:
		return 0
	}
}

func assignmentIssueUIDs(value boardhistory.Payload) []issue.UID {
	unit, _ := boardconflict.UnitOf(value)
	var result []issue.UID
	if unit.IssueUID != (issue.UID{}) {
		result = append(result, unit.IssueUID)
	}
	switch value := value.(type) {
	case *boardhistory.IssueDependency:
		result = append(result, value.PrerequisiteUID)
	case *boardhistory.IssueContainment:
		result = append(result, value.ParentUID)
	case *boardhistory.ExternalKey:
		result = append(result, value.IssueUID)
	}
	return result
}
