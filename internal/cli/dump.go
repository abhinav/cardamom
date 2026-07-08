package cli

import (
	"context"
	"slices"

	"go.abhg.dev/cardamom/internal/dump"
)

type dumpCommand struct {
	Directory     string   `arg:"" name:"directory" type:"path" help:"Directory to create or update."`
	Issues        []string `name:"issue" predictor:"issues" placeholder:"ISSUE" help:"Issue root to publish. Repeat for multiple roots."`
	NoDescendants bool     `name:"no-descendants" help:"Publish only named issues, without containment descendants."`
	Force         bool     `name:"force" help:"Replace recognized generated files modified after publication."`
}

func (c *dumpCommand) referencedIssueIDs() []string { return slices.Clone(c.Issues) }

func (*dumpCommand) Help() string {
	return "Publish a deterministic Markdown view of a board or selected containment subtrees."
}

// DumpOperation publishes one board selection to a Markdown directory.
type DumpOperation interface {
	// Execute publishes the requested selection and reports changed files.
	Execute(context.Context, dump.Request) (dump.ExecutionResult, error)
}

func (c *dumpCommand) Run(inv *Invocation, operation DumpOperation) error {
	if c.NoDescendants && len(c.Issues) == 0 {
		return UsageErrorf("--no-descendants requires --issue")
	}
	selection := dump.WholeBoard()
	if len(c.Issues) > 0 {
		selection = dump.SelectedIssues(c.Issues...)
		if c.NoDescendants {
			selection = dump.NamedIssuesOnly(c.Issues...)
		}
	}
	force := dump.PreserveGenerated
	if c.Force {
		force = dump.ForceGenerated
	}
	result, err := operation.Execute(inv.Context, dump.Request{
		Destination: c.Directory, Selection: selection, Force: force,
	})
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		issueIDs := result.Selection.IssueIDs
		if issueIDs == nil {
			issueIDs = []string{}
		}
		return inv.Output.WriteJSON(dumpResultOutput{
			Destination: result.Destination, BoardID: result.BoardID,
			Revision: result.Revision,
			Selection: dumpSelectionOutput{
				Mode: result.Selection.Mode.String(), IssueIDs: issueIDs,
				IncludeDescendants: result.Selection.Descendants == dump.IncludeDescendants,
			},
			Issues: result.Issues, Written: result.Written,
			Unchanged: result.Unchanged, Removed: result.Removed,
		})
	}
	return inv.Output.Noticef(
		"published %d issues to %s (%d written, %d unchanged, %d removed)",
		result.Issues, result.Destination, result.Written, result.Unchanged, result.Removed,
	)
}

type dumpSelectionOutput struct {
	Mode               string   `json:"mode"`
	IssueIDs           []string `json:"issue_ids"`
	IncludeDescendants bool     `json:"include_descendants"`
}

type dumpResultOutput struct {
	Destination string              `json:"destination"`
	BoardID     string              `json:"board_id"`
	Revision    int64               `json:"revision"`
	Selection   dumpSelectionOutput `json:"selection"`
	Issues      int                 `json:"issues"`
	Written     int                 `json:"written"`
	Unchanged   int                 `json:"unchanged"`
	Removed     int                 `json:"removed"`
}
