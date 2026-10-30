package board

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ListPins returns current issue references in board insertion order.
func (r *Repository) ListPins(
	ctx context.Context,
) (out []issue.Reference, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	ids, err := query.New(view).BoardListPinIDs(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	presentation, err := r.readIssuePresentation(ctx, view)
	if err != nil {
		return nil, err
	}
	values := make([]issue.Reference, 0, len(ids))
	for _, id := range ids {
		reference, err := presentation.Reference(issue.ID(id))
		if err != nil {
			return nil, err
		}
		values = append(values, reference)
	}
	return values, nil
}

// PinIssue adds one same-board issue at the end of the ordered collection.
// An existing pin succeeds unchanged before the current limit is considered.
func (r *Repository) PinIssue(
	ctx context.Context,
	id issue.ID,
	limit domainboard.PinLimit,
) (out domainboard.PinMutation, err error) {
	mutation, err := r.beginMutation(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	reference, err := r.pinIssueReference(ctx, mutation, id)
	if err != nil {
		return out, err
	}
	uid, err := r.readIssueUID(ctx, mutation.change, id)
	if err != nil {
		return out, err
	}
	queries := query.New(mutation.change)
	exists, err := queries.BoardPinExists(ctx, query.BoardPinExistsParams{
		BoardID: r.boardID.String(), IssueUid: uid.Bytes(),
	})
	if err != nil {
		return out, err
	}
	if exists {
		return domainboard.PinMutation{Issue: reference}, mutation.change.Commit()
	}
	// A new pin consumes the complete collection's count and final position.
	// Existing-pin no-ops and unpin do not need those unresolved assignments.
	conflictID, err := queries.BoardReadPinConflict(ctx, r.boardID.String())
	if err == nil {
		return out, errkind.Errorf(errkind.Conflict, "pin collection is unresolved in conflict %s", conflictID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	count, err := queries.BoardCountPins(ctx, r.boardID.String())
	if err != nil {
		return out, err
	}
	if uint64(count) >= limit.Uint64() {
		return out, fmt.Errorf("%w: maximum %d", domainboard.ErrPinLimit, limit.Uint64())
	}
	if err := mutation.reserve(ctx); err != nil {
		return out, err
	}
	position, err := queries.BoardInsertPin(ctx, query.BoardInsertPinParams{
		BoardID: r.boardID.String(), IssueUid: uid.Bytes(),
	})
	if err != nil {
		return out, err
	}
	mutation.record(boardhistory.Event{
		OccurredAt: mutation.occurredAt,
		Payload: &boardhistory.BoardPin{
			IssueUID: uid, Present: true,
			Position: position,
		},
	})
	if err := mutation.commit(ctx); err != nil {
		return out, err
	}
	return domainboard.PinMutation{Issue: reference, Changed: true}, nil
}

// UnpinIssue removes one same-board issue from the ordered collection.
func (r *Repository) UnpinIssue(
	ctx context.Context,
	id issue.ID,
) (out domainboard.PinMutation, err error) {
	mutation, err := r.beginMutation(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	reference, err := r.pinIssueReference(ctx, mutation, id)
	if err != nil {
		return out, err
	}
	uid, err := r.readIssueUID(ctx, mutation.change, id)
	if err != nil {
		return out, err
	}
	queries := query.New(mutation.change)
	exists, err := queries.BoardPinExists(ctx, query.BoardPinExistsParams{
		BoardID: r.boardID.String(), IssueUid: uid.Bytes(),
	})
	if err != nil {
		return out, err
	}
	if !exists {
		return domainboard.PinMutation{Issue: reference}, mutation.change.Commit()
	}
	if err := mutation.reserve(ctx); err != nil {
		return out, err
	}
	result, err := queries.BoardDeletePin(ctx, query.BoardDeletePinParams{
		BoardID: r.boardID.String(), IssueUid: uid.Bytes(),
	})
	if err != nil {
		return out, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return out, errors.New("pinned issue changed during mutation")
	}
	mutation.record(boardhistory.Event{
		OccurredAt: mutation.occurredAt,
		Payload: &boardhistory.BoardPin{
			IssueUID: uid,
		},
	})
	if err := mutation.commit(ctx); err != nil {
		return out, err
	}
	return domainboard.PinMutation{Issue: reference, Changed: true}, nil
}

func (r *Repository) pinIssueReference(
	ctx context.Context,
	mutation *mutation,
	id issue.ID,
) (issue.Reference, error) {
	presentation, err := r.readIssuePresentation(ctx, mutation.change)
	if err != nil {
		return issue.Reference{}, err
	}
	reference, err := presentation.Reference(id)
	if err != nil {
		return issue.Reference{}, err
	}
	for _, conflict := range reference.Conflicts {
		if conflict.Field == issue.FieldPins {
			return issue.Reference{}, errkind.Errorf(errkind.Conflict, "pin membership is unresolved in conflict %s", conflict.ID)
		}
	}
	return reference, nil
}
