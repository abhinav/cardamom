package board

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
	repositoryhistory "go.abhg.dev/cardamom/internal/repository/boardhistory"
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

	// reservedIssueUIDs prevents provisional private identities from repeating
	// before one mutation materializes its issue rows.
	reservedIssueUIDs map[issue.UID]struct{}

	// current is the selected board revision observed before this operation.
	current domainboard.Revision

	// occurredAt is shared by domain transitions and persisted projections.
	occurredAt time.Time

	// reservation identifies the store revision reserved after policy accepts a
	// change.
	reservation store.RevisionReservation

	// history incrementally retains semantic events in the same transaction as
	// their projections. The appender reads the frontier only after the first
	// shared event, so local-only and no-op mutations do no history work.
	history *repositoryhistory.Appender

	// historyErr preserves the first append failure until the caller reaches a
	// commit boundary. Projection helpers cannot return history errors directly,
	// but the transaction cannot publish after one occurs.
	historyErr error
}

// beginMutation opens the common issue writer boundary. Reading lifecycle and
// revision through that immediate transaction ensures an archive transition
// cannot interleave with an accepted issue mutation.
func (r *Repository) beginMutation(
	ctx context.Context,
) (*mutation, error) {
	change, err := r.store.Change(ctx)
	if err != nil {
		return nil, err
	}
	row, err := query.New(change).BoardGetRevision(ctx, r.boardID.String())
	if err != nil {
		return nil, errors.Join(err, change.Done())
	}
	// Explicit reads use separate paths and remain available for archived boards;
	// only the writer boundary enforces lifecycle policy.
	if row.ArchivedAt != nil {
		return nil, errors.Join(domainboard.ErrArchived, change.Done())
	}
	return &mutation{
		repository: r, change: change,
		reservedIssueIDs:  make(map[issue.ID]struct{}),
		reservedIssueUIDs: make(map[issue.UID]struct{}),
		current:           domainboard.Revision(row.Revision),
		occurredAt:        r.clock.Now(),
		history:           repositoryhistory.NewAppender(ctx, change, r.boardID),
	}, nil
}

func (m *mutation) reserve(ctx context.Context) error {
	var err error
	m.reservation, err = m.change.ReserveRevision(ctx)
	return err
}

// record streams semantic events into the transaction and preserves the first
// append failure for the operation's commit boundary.
func (m *mutation) record(events ...boardhistory.Event) {
	for _, event := range events {
		if m.historyErr != nil {
			return
		}
		m.historyErr = m.history.Append(event)
	}
}

func (m *mutation) hasHistory() bool { return m.history.HasEvents() }

// commit publishes scalar issue, board, and store revisions with every
// projection written through this mutation.
func (m *mutation) commit(
	ctx context.Context,
	affectedIssueIDs ...issue.ID,
) error {
	if m.historyErr != nil {
		return m.historyErr
	}
	if !m.hasHistory() {
		return errors.New("shared board mutation has no semantic history")
	}
	if err := m.history.Finish(); err != nil {
		return err
	}
	return m.commitProjection(ctx, affectedIssueIDs...)
}

// commitLocal publishes a mutation whose effects are replica-local and must
// not enter shared history, such as active execution custody.
func (m *mutation) commitLocal(
	ctx context.Context,
	affectedIssueIDs ...issue.ID,
) error {
	if m.historyErr != nil {
		return m.historyErr
	}
	if m.hasHistory() {
		return errors.New("local board mutation contains shared semantic history")
	}
	return m.commitProjection(ctx, affectedIssueIDs...)
}

func (m *mutation) commitProjection(
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
		uid, err := m.repository.readIssueUID(ctx, m.change, id)
		if err != nil {
			return err
		}
		result, err := queries.BoardPublishIssueRevision(
			ctx,
			query.BoardPublishIssueRevisionParams{
				Revision: m.reservation.Revision(),
				BoardID:  m.repository.boardID.String(),
				IssueUid: uid.Bytes(),
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
