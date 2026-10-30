package cli

import (
	"fmt"
	"strings"

	"go.abhg.dev/cardamom/internal/boardconflict"
)

//go:generate go tool mockgen -destination conflicts_mocks_test.go -package cli -typed -write_package_comment=false go.abhg.dev/cardamom/internal/boardconflict Repository,ConfigurationResolver

// conflictsCommand separates compact collection reads from selected body reads.
type conflictsCommand struct {
	List conflictsListCommand `cmd:"" default:"withargs" help:"List unresolved fields and constraints."`
	Show conflictsShowCommand `cmd:"" help:"Inspect reviewed alternatives or one retained value."`
}

type conflictsListCommand struct {
	Issue issueID `arg:"" optional:"" help:"Limit conflicts to one issue."`
	After string  `help:"Continue after the conflict ID returned by the previous page."`
	Limit int     `default:"50" help:"Maximum conflicts in this page (1–100)."`
}

func (c *conflictsListCommand) referencedIssueIDs() []string {
	if c.Issue == "" {
		return nil
	}
	return []string{c.Issue.String()}
}

func (c *conflictsListCommand) Run(inv *Invocation, service *boardconflict.Service) error {
	page, err := service.List(inv.Context, boardconflict.ListRequest{IssueID: c.Issue.String(), After: c.After, Limit: c.Limit})
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(conflictPageJSON(page))
	}
	var text strings.Builder
	if len(page.Conflicts) == 0 {
		text.WriteString("No unresolved conflicts.\n")
	}
	for _, value := range page.Conflicts {
		field := string(value.Unit.Kind)
		if value.ConstraintKind != 0 {
			field = value.ConstraintKind.String()
		}
		fmt.Fprintf(&text, "%s  %s  (%d values)", value.ID, field, value.Alternatives)
		for _, ref := range value.Issues {
			fmt.Fprintf(&text, "  %s", ref.ID)
		}
		text.WriteByte('\n')
	}
	if page.Next != "" {
		fmt.Fprintf(&text, "Continue with: card conflicts list --after %s\n", page.Next)
	}
	if len(page.Conflicts) > 0 {
		text.WriteString("Inspect: card conflicts show CONFLICT\nResolve a field: card resolve CONFLICT --take ALTERNATIVE\nSee card resolve --help for text and constraint repairs.\n")
	}
	return inv.Output.WriteString(text.String())
}

type conflictsShowCommand struct {
	ID          string `arg:"" name:"conflict" help:"Reviewed conflict identity."`
	Alternative string `help:"Read only this retained alternative body."`
}

func (c *conflictsShowCommand) Run(inv *Invocation, service *boardconflict.Service) error {
	if c.Alternative != "" {
		value, err := service.ReadAlternative(inv.Context, c.ID, c.Alternative)
		if err != nil {
			return err
		}
		if inv.Output.JSON() {
			output, err := conflictValueJSON(value)
			if err != nil {
				return err
			}
			return inv.Output.WriteJSON(output)
		}
		return inv.Output.WriteString(formatConflictValue(value) + "\n")
	}
	value, err := service.Read(inv.Context, c.ID)
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(conflictInspectionJSON(value))
	}
	var text strings.Builder
	// Entry is sealed; its concrete forms expose metadata, never retained bodies.
	switch conflict := value.Conflict.(type) {
	case *boardconflict.Conflict:
		fmt.Fprintf(&text, "%s\nField: %s\n", conflict.ID(), conflict.Unit().Kind)
		for _, alternative := range conflict.Alternatives() {
			fmt.Fprintf(&text, "  %s (%d originating assignments)\n", alternative.ID, len(alternative.Origins))
		}
		fmt.Fprintf(&text, "Resolve: card resolve %s --take ALTERNATIVE\n", c.ID)
	case *boardconflict.ConstraintConflict:
		fmt.Fprintf(&text, "%s\nConstraint: %s\n", conflict.ID(), conflict.Kind())
		for _, assignment := range conflict.Assignments() {
			fmt.Fprintf(&text, "  %s  %s\n", assignment.AlternativeID, assignment.Unit.Kind)
		}
		text.WriteString("Repair relationships with --remove-assignment, --parent CHILD:PARENT, or --unparent CHILD.\nRepair execution with --waiting keep|clear and optional --kind / --lifecycle.\n")
	}
	for _, ref := range value.Issues {
		fmt.Fprintf(&text, "Issue: %s\n", ref.Reference.ID)
	}
	fmt.Fprintf(&text, "Read one value: card conflicts show %s --alternative ALTERNATIVE\n", c.ID)
	return inv.Output.WriteString(text.String())
}
