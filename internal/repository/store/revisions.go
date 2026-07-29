package store

import (
	"context"
	"errors"
	"math"

	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// RevisionReservation identifies the canonical revision an open Change may
// publish if the retained store head has not changed.
type RevisionReservation struct {
	currentRevision int64
	revision        int64
}

// CurrentRevision returns the canonical revision observed when the logical
// transaction was reserved.
func (r RevisionReservation) CurrentRevision() int64 { return r.currentRevision }

// Revision returns the canonical revision reserved for publication.
func (r RevisionReservation) Revision() int64 { return r.revision }

// RevisionRange reserves one or more ordered canonical revisions for a single
// atomic publication.
type RevisionRange struct {
	currentRevision int64
	firstRevision   int64
	lastRevision    int64
}

// FirstRevision returns the first revision available to persisted projections.
func (r RevisionRange) FirstRevision() int64 { return r.firstRevision }

// LastRevision returns the canonical revision published at commit.
func (r RevisionRange) LastRevision() int64 { return r.lastRevision }

// ReserveRevision reads the scalar store head retained by Change and reserves
// its successor for publication in the same transaction.
func (c *Change) ReserveRevision(ctx context.Context) (RevisionReservation, error) {
	current, err := query.New(c).StoreGetCanonicalRevision(ctx)
	if err != nil {
		return RevisionReservation{}, err
	}
	if current == math.MaxInt64 {
		return RevisionReservation{}, errors.New("canonical revision space exhausted")
	}
	return RevisionReservation{
		currentRevision: current,
		revision:        current + 1,
	}, nil
}

// ReserveRevisions reserves count ordered revisions from the retained head.
func (c *Change) ReserveRevisions(
	ctx context.Context,
	count int64,
) (RevisionRange, error) {
	if count <= 0 {
		return RevisionRange{}, errors.New("revision reservation count must be positive")
	}
	current, err := query.New(c).StoreGetCanonicalRevision(ctx)
	if err != nil {
		return RevisionRange{}, err
	}
	if current > math.MaxInt64-count {
		return RevisionRange{}, errors.New("canonical revision space exhausted")
	}
	return RevisionRange{
		currentRevision: current,
		firstRevision:   current + 1,
		lastRevision:    current + count,
	}, nil
}

// PublishRevision advances the canonical store head to reservation.
func (c *Change) PublishRevision(ctx context.Context, reservation RevisionReservation) error {
	result, err := query.New(c).StorePublishCanonicalRevision(
		ctx,
		query.StorePublishCanonicalRevisionParams{
			Revision:        reservation.revision,
			CurrentRevision: reservation.currentRevision,
		},
	)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errors.New("canonical head changed")
	}
	return nil
}

// PublishRevisions advances the canonical store head to the reserved range's
// final revision.
func (c *Change) PublishRevisions(
	ctx context.Context,
	reservation RevisionRange,
) error {
	result, err := query.New(c).StorePublishCanonicalRevision(
		ctx,
		query.StorePublishCanonicalRevisionParams{
			Revision:        reservation.lastRevision,
			CurrentRevision: reservation.currentRevision,
		},
	)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errors.New("canonical head changed")
	}
	return nil
}

// CanonicalRevision returns the canonical revision retained by View.
func (v *View) CanonicalRevision(ctx context.Context) (int64, error) {
	return query.New(v).StoreGetCanonicalRevision(ctx)
}

// CanonicalRevision returns the canonical revision retained by Change.
func (c *Change) CanonicalRevision(ctx context.Context) (int64, error) {
	return query.New(c).StoreGetCanonicalRevision(ctx)
}

// CanonicalRevision reads the latest committed canonical revision from a new
// Store snapshot.
func (s *Store) CanonicalRevision(ctx context.Context) (revision int64, err error) {
	view, err := s.View(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	return view.CanonicalRevision(ctx)
}

// ReserveIssueNumber returns and advances the store-wide sequential issue
// number within Change.
func (c *Change) ReserveIssueNumber(ctx context.Context) (int64, error) {
	queries := query.New(c)
	next, err := queries.StoreGetNextIssueNumber(ctx)
	if err != nil {
		return 0, err
	}
	if next == math.MaxInt64 {
		return 0, errors.New("sequential issue ID space exhausted")
	}
	if err := queries.StoreSetNextIssueNumber(ctx, next+1); err != nil {
		return 0, err
	}
	return next, nil
}

// AdvanceIssueNumber moves the store-wide sequential allocator past a
// preserved imported identity without decreasing its current position.
func (c *Change) AdvanceIssueNumber(ctx context.Context, next int64) error {
	if next <= 0 {
		return errors.New("next sequential issue number must be positive")
	}
	return query.New(c).StoreAdvanceNextIssueNumber(ctx, next)
}
