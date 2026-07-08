package changeconnect

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/must"
)

const defaultChangePollInterval = 500 * time.Millisecond

// CanonicalRevisionReader reads the committed head of one physical store.
type CanonicalRevisionReader interface {
	// CanonicalRevision returns the latest committed canonical revision.
	CanonicalRevision(context.Context) (int64, error)
}

// ChangeBoardLister reads the committed board catalog used by aggregate
// subscriptions.
type ChangeBoardLister interface {
	// ListAllBoards returns the current committed board catalog.
	ListAllBoards(context.Context) ([]*board.State, error)
}

// PollingConfig supplies the committed revision reader and poll
// cadence for a PollingSource.
type PollingConfig struct {
	// Revisions reads committed canonical heads.
	Revisions CanonicalRevisionReader // required

	// Boards refreshes aggregate board scope.
	Boards ChangeBoardLister // required

	// Interval is the delay between committed-head reads.
	// Zero uses 500 milliseconds.
	Interval time.Duration
}

// PollingSource turns committed canonical-head reads into board revision
// notifications.
//
// The source may coalesce multiple unseen revisions into the latest observed
// head. The Connect boundary converts every notification into a conservative
// invalidation of all browser resources.
type PollingSource struct {
	// revisions supplies committed canonical heads.
	revisions CanonicalRevisionReader

	// boards refreshes aggregate board scope at each observed head.
	boards ChangeBoardLister

	// interval controls the delay between committed-head reads.
	interval time.Duration
}

// NewPollingSource constructs a production change source over a narrow
// committed-revision reader.
func NewPollingSource(cfg PollingConfig) *PollingSource {
	must.NotBeNilf(cfg.Revisions, "changeconnect: canonical revision reader is required")
	must.NotBeNilf(cfg.Boards, "changeconnect: change board lister is required")
	interval := cfg.Interval
	if interval == 0 {
		interval = defaultChangePollInterval
	}
	if interval < 0 {
		panic("changeconnect: change poll interval must not be negative")
	}
	return &PollingSource{
		revisions: cfg.Revisions,
		boards:    cfg.Boards,
		interval:  interval,
	}
}

// Subscribe captures the current committed head for reconnect catch-up and
// returns an independent polling subscription.
func (s *PollingSource) Subscribe(
	ctx context.Context,
	request WatchRequest,
) (Subscription, error) {
	revision, boardIDs, err := s.snapshot(ctx, request)
	if err != nil {
		return nil, err
	}
	return &pollingSubscription{
		source:  s,
		request: cloneWatchRequest(request),
		current: revision,
		pending: broadChanges(boardIDs, revision),
	}, nil
}

// snapshot brackets board selection with equal committed-head reads.
// A concurrent commit retries the complete read so aggregate scope cannot
// retain a catalog from an older head.
func (s *PollingSource) snapshot(
	ctx context.Context,
	request WatchRequest,
) (uint64, []board.ID, error) {
	for {
		before, err := s.canonicalRevision(ctx)
		if err != nil {
			return 0, nil, err
		}
		boardIDs, err := s.changeWatchBoardIDs(ctx, request)
		if err != nil {
			return 0, nil, err
		}
		after, err := s.canonicalRevision(ctx)
		if err != nil {
			return 0, nil, err
		}
		if after < before {
			return 0, nil, fmt.Errorf(
				"canonical revision moved from %d to %d",
				before,
				after,
			)
		}
		if after == before {
			return after, boardIDs, nil
		}
	}
}

func (s *PollingSource) canonicalRevision(
	ctx context.Context,
) (uint64, error) {
	revision, err := s.revisions.CanonicalRevision(ctx)
	if err != nil {
		return 0, fmt.Errorf("read canonical revision: %w", err)
	}
	if revision < 0 {
		return 0, fmt.Errorf("canonical revision %d is negative", revision)
	}
	return uint64(revision), nil
}

func (s *PollingSource) changeWatchBoardIDs(
	ctx context.Context,
	request WatchRequest,
) ([]board.ID, error) {
	if request.AllBoards {
		boards, err := s.boards.ListAllBoards(ctx)
		if err != nil {
			return nil, fmt.Errorf("list change watch boards: %w", err)
		}
		boardIDs := make([]board.ID, 0, len(boards))
		for _, board := range boards {
			boardIDs = append(boardIDs, board.ID())
		}
		return validateChangeWatchBoardIDs(boardIDs)
	}
	if len(request.BoardIDs) == 0 && !request.AllBoards {
		return nil, errors.New("change watch requires a board")
	}
	return validateChangeWatchBoardIDs(request.BoardIDs)
}

func validateChangeWatchBoardIDs(
	values []board.ID,
) ([]board.ID, error) {
	boardIDs := slices.Clone(values)
	seen := make(map[board.ID]struct{}, len(boardIDs))
	for _, boardID := range boardIDs {
		validated, err := board.NewID(boardID.String())
		if err != nil {
			return nil, fmt.Errorf("change watch board ID: %w", err)
		}
		if _, duplicate := seen[validated]; duplicate {
			return nil, fmt.Errorf("duplicate change watch board %q", validated)
		}
		seen[validated] = struct{}{}
	}
	return boardIDs, nil
}

func cloneWatchRequest(request WatchRequest) WatchRequest {
	request.BoardIDs = slices.Clone(request.BoardIDs)
	return request
}

// pollingSubscription owns one reconnect cursor and lazily created
// ticker. Receive is called serially by one Connect stream.
type pollingSubscription struct {
	// source supplies stabilized committed snapshots and poll cadence.
	source *PollingSource

	// request is the immutable scope selected for this connection.
	request WatchRequest

	// current is the latest committed head represented by pending or delivered
	// invalidations.
	current uint64

	// pending contains remaining per-board invalidations for current.
	pending []CommittedChange

	// ticker exists only after catch-up delivery needs live polling.
	ticker *time.Ticker
}

// Receive returns pending catch-up invalidations before polling for a newer
// committed head.
func (s *pollingSubscription) Receive(
	ctx context.Context,
) (CommittedChange, error) {
	if err := ctx.Err(); err != nil {
		if s.ticker != nil {
			s.ticker.Stop()
		}
		return CommittedChange{}, err
	}
	if len(s.pending) > 0 {
		return s.popPending(), nil
	}
	if s.ticker == nil {
		s.ticker = time.NewTicker(s.source.interval)
	}
	for {
		select {
		case <-ctx.Done():
			s.ticker.Stop()
			return CommittedChange{}, ctx.Err()
		case <-s.ticker.C:
			revision, boardIDs, err := s.source.snapshot(ctx, s.request)
			if err != nil {
				s.ticker.Stop()
				return CommittedChange{}, err
			}
			if revision < s.current {
				s.ticker.Stop()
				return CommittedChange{}, fmt.Errorf(
					"canonical revision moved from %d to %d",
					s.current,
					revision,
				)
			}
			if revision == s.current {
				continue
			}
			s.current = revision
			s.pending = broadChanges(boardIDs, s.current)
			if len(s.pending) > 0 {
				return s.popPending(), nil
			}
		}
	}
}

func (s *pollingSubscription) popPending() CommittedChange {
	change := s.pending[0]
	s.pending = s.pending[1:]
	return change
}

func broadChanges(
	boardIDs []board.ID,
	revision uint64,
) []CommittedChange {
	if revision == 0 {
		return nil
	}
	changes := make([]CommittedChange, 0, len(boardIDs))
	for _, boardID := range boardIDs {
		changes = append(changes, CommittedChange{
			BoardID:  boardID,
			Revision: revision,
		})
	}
	return changes
}
