package cli

import (
	"context"
	"fmt"
	"text/tabwriter"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

// BoardPinRequest identifies one issue by ID or exact board-scoped key.
type BoardPinRequest struct {
	Value string
	Key   bool
}

// BoardPinOperations supplies the ordered board-pin commands.
type BoardPinOperations interface {
	ListBoardPins(context.Context) ([]issue.Reference, error)
	PinBoardIssue(context.Context, board.Invocation, BoardPinRequest) (board.PinMutation, error)
	UnpinBoardIssue(context.Context, board.Invocation, BoardPinRequest) (board.PinMutation, error)
}

type boardPinCommand struct {
	Issue string `arg:"" name:"issue" predictor:"issues" help:"Issue ID or exact producer key with --key."`
	Key   bool   `name:"key" help:"Treat the positional argument as an exact producer key."`
}

func (c *boardPinCommand) referencedIssueIDs() []string {
	if c.Key {
		return nil
	}
	return []string{c.Issue}
}

func (*boardPinCommand) Help() string {
	return "Add one issue to the end of the selected board's pinned issue order."
}

func (c *boardPinCommand) Run(invocation *Invocation, operations BoardPinOperations) error {
	result, err := operations.PinBoardIssue(
		invocation.Context,
		board.NewInvocation(invocation.Actor),
		BoardPinRequest{Value: c.Issue, Key: c.Key},
	)
	if err != nil {
		return err
	}
	return renderBoardPinMutation(invocation.Output, "pin", result)
}

type boardUnpinCommand struct {
	Issue string `arg:"" name:"issue" predictor:"issues" help:"Issue ID or exact producer key with --key."`
	Key   bool   `name:"key" help:"Treat the positional argument as an exact producer key."`
}

func (c *boardUnpinCommand) referencedIssueIDs() []string {
	if c.Key {
		return nil
	}
	return []string{c.Issue}
}

func (*boardUnpinCommand) Help() string {
	return "Remove one issue from the selected board's pinned issue order."
}

func (c *boardUnpinCommand) Run(invocation *Invocation, operations BoardPinOperations) error {
	result, err := operations.UnpinBoardIssue(
		invocation.Context,
		board.NewInvocation(invocation.Actor),
		BoardPinRequest{Value: c.Issue, Key: c.Key},
	)
	if err != nil {
		return err
	}
	return renderBoardPinMutation(invocation.Output, "unpin", result)
}

type boardPinsCommand struct{}

func (*boardPinsCommand) Help() string {
	return "List current issues in the selected board's pinned issue order."
}

func (*boardPinsCommand) Run(invocation *Invocation, operations BoardPinOperations) error {
	values, err := operations.ListBoardPins(invocation.Context)
	if err != nil {
		return err
	}
	if invocation.Output.JSON() {
		return WriteJSONLines(invocation.Output, values)
	}

	writer := tabwriter.NewWriter(invocation.Output.Stdout(), 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "ID\tPRI\tSTATUS\tTYPE\tTITLE"); err != nil {
		return fmt.Errorf("write board pins header: %w", err)
	}
	for _, value := range values {
		if _, err := fmt.Fprintf(
			writer,
			"%s\t%d\t%s\t%s\t%s\n",
			value.ID,
			value.Priority,
			value.Status,
			value.Type,
			singleLine(value.Title),
		); err != nil {
			return fmt.Errorf("write board pins: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush board pins: %w", err)
	}
	return nil
}

type boardPinMutationOutput struct {
	Issue   issue.Reference `json:"issue"`
	Changed bool            `json:"changed"`
}

func renderBoardPinMutation(output *Output, operation string, result board.PinMutation) error {
	if output.JSON() {
		return output.WriteJSON(boardPinMutationOutput{
			Issue: result.Issue, Changed: result.Changed,
		})
	}

	switch {
	case operation == "pin" && result.Changed:
		return output.Noticef("pinned %s: %s", result.Issue.ID, result.Issue.Title)
	case operation == "pin":
		return output.Noticef("%s is already pinned: %s", result.Issue.ID, result.Issue.Title)
	case result.Changed:
		return output.Noticef("unpinned %s: %s", result.Issue.ID, result.Issue.Title)
	default:
		return output.Noticef("%s is not pinned: %s", result.Issue.ID, result.Issue.Title)
	}
}
