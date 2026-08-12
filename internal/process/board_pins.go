package process

import (
	"context"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/issue"
)

type boardPinOperations struct {
	pins   *board.Pins
	issues *issue.Queries
}

var _ cli.BoardPinOperations = (*boardPinOperations)(nil)

func provideBoardPinOperations(
	runtime *namespaceRuntime,
	selected *board.State,
) (cli.BoardPinOperations, error) {
	pins, err := runtime.boardPins(selected.ID())
	if err != nil {
		return nil, err
	}
	issues, err := runtime.issueQueries(selected.ID())
	if err != nil {
		return nil, err
	}
	return &boardPinOperations{pins: pins, issues: issues}, nil
}

func (o *boardPinOperations) ListBoardPins(
	ctx context.Context,
) ([]issue.Reference, error) {
	return o.pins.List(ctx)
}

func (o *boardPinOperations) PinBoardIssue(
	ctx context.Context,
	invocation board.Invocation,
	request cli.BoardPinRequest,
) (board.PinMutation, error) {
	id, err := o.resolveIssueID(ctx, request)
	if err != nil {
		return board.PinMutation{}, err
	}
	return o.pins.Pin(ctx, invocation, id)
}

func (o *boardPinOperations) UnpinBoardIssue(
	ctx context.Context,
	invocation board.Invocation,
	request cli.BoardPinRequest,
) (board.PinMutation, error) {
	id, err := o.resolveIssueID(ctx, request)
	if err != nil {
		return board.PinMutation{}, err
	}
	return o.pins.Unpin(ctx, invocation, id)
}

func (o *boardPinOperations) resolveIssueID(
	ctx context.Context,
	request cli.BoardPinRequest,
) (string, error) {
	if !request.Key {
		return request.Value, nil
	}
	view, err := o.issues.ReadIssue(ctx, issue.ReadRequest{Key: request.Value})
	if err != nil {
		return "", err
	}
	return view.Detail.Issue.ID, nil
}
