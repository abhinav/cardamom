// Package issueconnect exposes board-scoped issue reads through Connect.
package issueconnect

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/web/boardscope"
	"go.abhg.dev/cardamom/internal/web/issueview"
)

//go:generate go tool mockgen -destination mocks_test.go -package issueconnect -typed -write_package_comment=false . BoardReaderFactory,BoardPinsFactory

// BoardReader supplies the issue reads exposed by IssueService.
type BoardReader interface {
	// ReadIssue returns one issue detail and its requested inherited context.
	ReadIssue(context.Context, issue.ReadRequest) (issue.View, error)

	// ListIssues returns issue summaries matching one board-scoped query.
	ListIssues(context.Context, issue.ListRequest) ([]issue.Summary, error)

	// ListIssuesSnapshot reads issue summaries and the board revision from one view.
	ListIssuesSnapshot(context.Context, issue.ListRequest) (issue.ListSnapshot, error)
}

// BoardReaderFactory opens issue reads for one explicitly resolved board.
type BoardReaderFactory interface {
	// Reader returns issue reads constrained to boardID.
	Reader(board.ID) (BoardReader, error)
}

// BoardPins supplies the pin operations exposed by IssueService.
type BoardPins interface {
	// List returns current issue references in insertion order.
	List(context.Context) ([]issue.Reference, error)

	// Pin adds one issue to the ordered collection when absent.
	Pin(context.Context, board.Invocation, string) (board.PinMutation, error)

	// Unpin removes one issue from the ordered collection when present.
	Unpin(context.Context, board.Invocation, string) (board.PinMutation, error)
}

// BoardPinsFactory opens pin operations for one explicitly resolved board.
type BoardPinsFactory interface {
	// Pins returns pin operations constrained to boardID.
	Pins(board.ID) (BoardPins, error)
}

// Config supplies the collaborators required by IssueService.
type Config struct {
	// Scope resolves protocol scopes and store-global issue ownership.
	Scope *boardscope.Resolver // required

	// Readers opens board-scoped issue reads.
	Readers BoardReaderFactory // required

	// Pins opens board-scoped pin operations.
	Pins BoardPinsFactory // required

	// Views converts issue-domain records to generated protocol messages.
	Views *issueview.Encoder // required
}

// Service adapts issue reads to generated IssueService RPCs.
type Service struct {
	privatev1connect.UnimplementedIssueServiceHandler
	scope   *boardscope.Resolver
	readers BoardReaderFactory
	pins    BoardPinsFactory
	views   *issueview.Encoder
}

var _ privatev1connect.IssueServiceHandler = (*Service)(nil)

// New constructs an IssueService handler from board-scoped domain operations.
func New(cfg Config) *Service {
	must.NotBeNilf(cfg.Scope, "issueconnect: board scope resolver is required")
	must.NotBeNilf(cfg.Readers, "issueconnect: board reader factory is required")
	must.NotBeNilf(cfg.Pins, "issueconnect: board pins factory is required")
	must.NotBeNilf(cfg.Views, "issueconnect: issue view encoder is required")
	return &Service{
		scope: cfg.Scope, readers: cfg.Readers, pins: cfg.Pins, views: cfg.Views,
	}
}

// scopedBoardReader pairs one resolved board with reads constrained to it.
type scopedBoardReader struct {
	board  *board.State
	reader BoardReader
}

func (s *Service) scopedReaders(
	ctx context.Context,
	scopeValue *privatev1.BoardScope,
) ([]scopedBoardReader, error) {
	boards, err := s.scope.Boards(ctx, scopeValue)
	if err != nil {
		return nil, err
	}
	return s.readersForBoards(boards)
}

func (s *Service) readersForBoards(boards []*board.State) ([]scopedBoardReader, error) {
	readers := make([]scopedBoardReader, 0, len(boards))
	for _, board := range boards {
		reader, err := s.readers.Reader(board.ID())
		if err != nil {
			return nil, fmt.Errorf("open board %q: %w", board.ID(), err)
		}
		readers = append(readers, scopedBoardReader{board: board, reader: reader})
	}
	return readers, nil
}

func (s *Service) readerForIssue(
	ctx context.Context,
	issueID string,
) (scopedBoardReader, error) {
	state, err := s.scope.BoardForIssue(ctx, issueID)
	if err != nil {
		return scopedBoardReader{}, err
	}
	readers, err := s.readersForBoards([]*board.State{state})
	if err != nil {
		return scopedBoardReader{}, err
	}
	return readers[0], nil
}
