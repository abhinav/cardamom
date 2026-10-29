package cli

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/boardclone"
)

// BoardCloneOperation resolves local stores and publishes a new board replica.
type BoardCloneOperation interface {
	Clone(context.Context, BoardCloneRequest) (boardclone.Result, error)
}

// BoardCloneRequest selects the destination without changing shared board state.
type BoardCloneRequest struct {
	// DestinationStore is an existing local store directory.
	DestinationStore string // required

	// DestinationProject is an ID or name; omission selects the sole project.
	DestinationProject string
}

type boardCloneCommand struct {
	ToStore   string `name:"to-store" required:"" placeholder:"DESTINATION" help:"Existing destination Cardamom store."`
	ToProject string `name:"to-project" placeholder:"PROJECT" help:"Destination project ID or name. Defaults to the destination store's sole project."`
}

// Help describes identity preservation and the local-only transfer boundary.
func (*boardCloneCommand) Help() string {
	return `Create a replica of the selected board in another local store.

The root --store and --board flags select the source. Clone preserves the
board name, identities, shared state, mutation history, and attachment content.
Claims, assignees, leases, mail, and unfinished uploads stay local.

Both replicas must use random issue IDs. The destination must not already
contain the board. Clone does not update, merge, or synchronize existing boards.`
}

// Run publishes the selected board and renders the committed clone result.
func (c *boardCloneCommand) Run(invocation *Invocation, operation BoardCloneOperation) error {
	out, err := operation.Clone(invocation.Context, BoardCloneRequest{
		DestinationStore: c.ToStore, DestinationProject: c.ToProject,
	})
	if err != nil {
		return err
	}
	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(out)
	}
	return invocation.Output.WriteString(fmt.Sprintf(
		"cloned board %q as %s in project %s: %d %s, %d %s, %d %s\n",
		out.Name, out.BoardID, out.ProjectID,
		out.Issues, plural(out.Issues, "issue", "issues"),
		out.Mutations, plural(out.Mutations, "mutation", "mutations"),
		out.Blobs, plural(out.Blobs, "blob", "blobs"),
	))
}
