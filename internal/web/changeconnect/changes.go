// Package changeconnect streams committed domain changes through Connect.
package changeconnect

import (
	"context"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/web"
	"go.abhg.dev/cardamom/internal/web/boardscope"
)

// Config supplies the collaborators required by ChangeService.
type Config struct {
	// Scope resolves protocol scopes to current board identities.
	Scope *boardscope.Resolver // required

	// Changes opens independent committed-change subscriptions.
	Changes Source // required
}

// Service adapts committed revisions to generated ChangeService streams.
type Service struct {
	privatev1connect.UnimplementedChangeServiceHandler
	scope   *boardscope.Resolver
	changes Source
}

var _ privatev1connect.ChangeServiceHandler = (*Service)(nil)

// New constructs a ChangeService handler from scope and change-source policy.
func New(cfg Config) *Service {
	must.NotBeNilf(cfg.Scope, "changeconnect: board scope resolver is required")
	must.NotBeNilf(cfg.Changes, "changeconnect: change source is required")
	return &Service{scope: cfg.Scope, changes: cfg.Changes}
}

// CommittedChange identifies one board at a published canonical revision.
type CommittedChange struct {
	// BoardID identifies the board affected by the revision.
	BoardID board.ID

	// Revision is the positive canonical revision assigned after commit.
	Revision uint64
}

// WatchRequest identifies the boards visible to one stream subscription.
type WatchRequest struct {
	// BoardIDs is the resolved board set at subscription time.
	BoardIDs []board.ID

	// AllBoards requires the source to refresh board selection as committed
	// heads advance.
	AllBoards bool
}

// Subscription delivers committed revisions in increasing order for
// each board in one WatchChanges connection.
type Subscription interface {
	// Receive waits for the next committed change or context cancellation.
	// Implementations may coalesce revisions when each delivered invalidation
	// is sufficient to recover current state.
	Receive(context.Context) (CommittedChange, error)
}

// Source opens independent committed-revision subscriptions.
//
// A new subscription must first deliver a committed catch-up state
// for a reconnecting browser,
// then deliver later committed state according to its source policy.
// It must never expose a reserved or rolled-back revision.
type Source interface {
	// Subscribe opens one connection-owned committed-change subscription.
	Subscribe(context.Context, WatchRequest) (Subscription, error)
}

// WatchChanges streams protobuf invalidations for committed canonical
// revisions in the requested board scope.
func (s *Service) WatchChanges(
	ctx context.Context,
	request *connect.Request[privatev1.WatchChangesRequest],
	stream *connect.ServerStream[privatev1.WatchChangesResponse],
) error {
	boards, err := s.scope.Boards(ctx, request.Msg.GetScope())
	if err != nil {
		return web.FromError(err)
	}
	watch := WatchRequest{
		BoardIDs:  make([]board.ID, 0, len(boards)),
		AllBoards: isAllBoardsScope(request.Msg.GetScope()),
	}
	for _, board := range boards {
		watch.BoardIDs = append(watch.BoardIDs, board.ID())
	}
	subscription, err := s.changes.Subscribe(ctx, watch)
	if err != nil {
		return web.FromError(err)
	}
	if subscription == nil {
		return web.FromError(errors.New("change source returned a nil subscription"))
	}

	lastRevisions := make(map[board.ID]uint64, len(boards))
	for {
		change, err := subscription.Receive(ctx)
		if err != nil {
			return web.FromError(err)
		}
		lastRevision := lastRevisions[change.BoardID]
		if change.Revision <= lastRevision {
			return web.FromError(fmt.Errorf(
				"change revision %d does not follow %d",
				change.Revision,
				lastRevision,
			))
		}
		response, err := watchChangesResponse(change)
		if err != nil {
			return web.FromError(err)
		}
		if err := stream.Send(response); err != nil {
			return web.FromError(err)
		}
		lastRevisions[change.BoardID] = change.Revision
	}
}

func isAllBoardsScope(scope *privatev1.BoardScope) bool {
	if scope == nil {
		return false
	}
	_, ok := scope.Selection.(*privatev1.BoardScope_AllBoards)
	return ok
}

// watchChangesResponse validates source-owned revision data and maps it to the
// generated coarse-invalidation protocol.
func watchChangesResponse(change CommittedChange) (*privatev1.WatchChangesResponse, error) {
	boardID, err := board.NewID(change.BoardID.String())
	if err != nil {
		return nil, fmt.Errorf("change board ID: %w", err)
	}
	if change.Revision == 0 {
		return nil, errors.New("change revision must be positive")
	}
	return &privatev1.WatchChangesResponse{
		BoardId:  boardID.String(),
		Revision: change.Revision,
		Resources: []privatev1.WatchResource{
			privatev1.WatchResource_WATCH_RESOURCE_BOARD_CATALOG,
			privatev1.WatchResource_WATCH_RESOURCE_BOARD,
			privatev1.WatchResource_WATCH_RESOURCE_ISSUES,
			privatev1.WatchResource_WATCH_RESOURCE_LOG,
			privatev1.WatchResource_WATCH_RESOURCE_APPROVALS,
		},
	}, nil
}
