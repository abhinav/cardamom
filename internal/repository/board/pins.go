package board

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	domainboard "go.abhg.dev/cardamom/internal/board"
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
	rows, err := query.New(view).BoardListPinReferences(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	values := make([]issue.Reference, 0, len(rows))
	for _, row := range rows {
		values = append(values, pinReference(
			row.ID, row.Title, row.Kind, row.Status, row.Priority,
		))
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
	queries := query.New(mutation.change)
	exists, err := queries.BoardPinExists(ctx, query.BoardPinExistsParams{
		BoardID: r.boardID.String(), IssueID: id.String(),
	})
	if err != nil {
		return out, err
	}
	if exists {
		return domainboard.PinMutation{Issue: reference}, mutation.change.Commit()
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
	if err := queries.BoardInsertPin(ctx, query.BoardInsertPinParams{
		BoardID: r.boardID.String(), IssueID: id.String(),
	}); err != nil {
		return out, err
	}
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
	queries := query.New(mutation.change)
	exists, err := queries.BoardPinExists(ctx, query.BoardPinExistsParams{
		BoardID: r.boardID.String(), IssueID: id.String(),
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
		BoardID: r.boardID.String(), IssueID: id.String(),
	})
	if err != nil {
		return out, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return out, errors.New("pinned issue changed during mutation")
	}
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
	row, err := query.New(mutation.change).BoardGetPinIssueReference(
		ctx,
		query.BoardGetPinIssueReferenceParams{
			BoardID: r.boardID.String(), IssueID: id.String(),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return issue.Reference{}, errkind.Errorf(
			errkind.NotFound, "issue not found: %s", id,
		)
	}
	if err != nil {
		return issue.Reference{}, err
	}
	return pinReference(row.ID, row.Title, row.Kind, row.Status, row.Priority), nil
}

func pinReference(
	id string,
	title string,
	kind string,
	status string,
	priority int64,
) issue.Reference {
	return issue.Reference{
		ID: id, Title: title, Type: kind, Status: status, Priority: int(priority),
	}
}
