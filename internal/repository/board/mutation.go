package board

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// mutation retains one immediate writer snapshot from state loading through
// canonical publication.
type mutation struct {
	// repository supplies selected-board identity and publication policy.
	repository *Repository

	// change retains the immediate transaction for the operation lifetime.
	change *store.Change

	// reservedIssueIDs prevents provisional random identities from repeating
	// before one mutation materializes its issue rows.
	reservedIssueIDs map[issue.ID]struct{}

	// current is the selected board revision observed before this operation.
	current domainboard.Revision

	// occurredAt is shared by domain transitions and persisted projections.
	occurredAt time.Time

	// reservation identifies the store revision reserved after policy accepts a
	// change.
	reservation store.RevisionReservation
}

func (r *Repository) beginMutation(
	ctx context.Context,
) (*mutation, error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return nil, err
	}
	current, err := query.New(change).BoardGetRevision(ctx, r.boardID.String())
	if err != nil {
		return nil, errors.Join(err, change.Done())
	}
	return &mutation{
		repository: r, change: change, reservedIssueIDs: make(map[issue.ID]struct{}),
		current: domainboard.Revision(current), occurredAt: r.clock.Now(),
	}, nil
}

func (m *mutation) reserve(ctx context.Context) error {
	var err error
	m.reservation, err = m.change.ReserveRevision(ctx)
	return err
}

// commit publishes scalar issue, board, and store revisions with every
// projection written through this mutation.
func (m *mutation) commit(
	ctx context.Context,
	affectedIssueIDs ...issue.ID,
) error {
	queries := query.New(m.change)
	seen := make(map[issue.ID]struct{}, len(affectedIssueIDs))
	for _, id := range affectedIssueIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result, err := queries.BoardPublishIssueRevision(
			ctx,
			query.BoardPublishIssueRevisionParams{
				Revision: m.reservation.Revision(),
				BoardID:  m.repository.boardID.String(),
				IssueID:  id.String(),
			},
		)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return fmt.Errorf("advance issue %q revision: row not found", id)
		}
	}

	result, err := queries.BoardPublishRevision(
		ctx,
		query.BoardPublishRevisionParams{
			Revision:         m.reservation.Revision(),
			ID:               m.repository.boardID.String(),
			PreviousRevision: int64(m.current),
		},
	)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errors.New("board revision changed")
	}
	if err := m.change.PublishRevision(ctx, m.reservation); err != nil {
		return fmt.Errorf("publish canonical revision: %w", err)
	}
	return m.change.Commit()
}
