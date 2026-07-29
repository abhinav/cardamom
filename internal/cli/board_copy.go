package cli

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/boardcopy"
)

// BoardCopyRequest supplies destination selections from the command boundary.
type BoardCopyRequest struct {
	DestinationStore   string
	DestinationProject string
	Name               *string
}

// BoardCopyOperation creates one non-destructive destination board.
type BoardCopyOperation interface {
	Copy(context.Context, BoardCopyRequest) (boardcopy.CopyOutcome, error)
}

type boardCopyCommand struct {
	ToStore   string  `name:"to-store" required:"" placeholder:"DESTINATION" help:"Existing destination Cardamom store."`
	ToProject string  `name:"to-project" placeholder:"PROJECT" help:"Destination project ID or exact name. When omitted, use the destination store's sole project."`
	Name      *string `name:"name" placeholder:"NAME" help:"Destination board name. Defaults to the source board name."`
}

// Help describes the bounded, non-destructive transfer contract.
func (*boardCopyCommand) Help() string {
	return `Create one new board in an existing destination store.

The root --store and --board flags select the source. The source remains
unchanged. The command does not merge, move, refresh, or synchronize boards.`
}

// Run copies the selected source board and renders its durable receipt.
func (c *boardCopyCommand) Run(
	invocation *Invocation,
	operation BoardCopyOperation,
) error {
	outcome, err := operation.Copy(invocation.Context, BoardCopyRequest{
		DestinationStore:   c.ToStore,
		DestinationProject: c.ToProject,
		Name:               c.Name,
	})
	if err != nil {
		return err
	}
	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(outcome)
	}

	action := "copied"
	if outcome.AlreadyCompleted {
		action = "previously copied"
	}
	return invocation.Output.WriteString(fmt.Sprintf(
		"%s board %q as %s in project %s: %d %s, %d %s, %d %s, %d %s\n",
		action,
		outcome.DestinationName,
		outcome.DestinationBoardID,
		outcome.DestinationProjectID,
		outcome.Counts.Issues,
		plural(outcome.Counts.Issues, "issue", "issues"),
		outcome.Counts.LogEntries,
		plural(outcome.Counts.LogEntries, "Log entry", "Log entries"),
		outcome.Counts.Attachments,
		plural(outcome.Counts.Attachments, "attachment", "attachments"),
		outcome.Counts.Blobs,
		plural(outcome.Counts.Blobs, "blob", "blobs"),
	))
}
