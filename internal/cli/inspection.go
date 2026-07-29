package cli

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/issue"
)

type listCommand struct {
	UnderID     string     `name:"under" predictor:"issues" placeholder:"ISSUE" help:"List strict containment descendants of this issue."`
	Statuses    []string   `name:"status" sep:"," enum:"ready,blocked,in_progress,waiting,closed,cancelled" placeholder:"STATUS" help:"Effective issue status to include: ready, blocked, in_progress, waiting, closed, or cancelled. Repeat or separate values with commas to match any status. Defaults to ready, blocked, in_progress, and waiting."`
	Assignee    *string    `name:"assignee" placeholder:"ACTOR" help:"Match active custody by this actor."`
	Type        string     `name:"type" placeholder:"TYPE" help:"Match one issue type: workstream, task, checkpoint, or routine."`
	Labels      labelTerms `name:"label" predictor:"labels" placeholder:"TERM" help:"Label term. No prefix or + requires; - excludes. Repeat for multiple labels."`
	LabelsAny   []string   `name:"label-any" placeholder:"LABEL" help:"Match issues carrying at least one supplied label. Repeat for alternatives."`
	NoAssignee  bool       `name:"no-assignee" help:"Match issues without active custody."`
	TitleRegexp string     `name:"title-regexp" placeholder:"REGEXP" help:"Match titles using Go regular-expression syntax."`
	Sort        string     `name:"sort" placeholder:"FIELD" help:"Sort by priority, created, updated, closed, id, title, or type."`
	Reverse     bool       `name:"reverse" help:"Reverse the selected order."`
	Limit       int        `name:"limit" placeholder:"COUNT" help:"Maximum results. Zero returns every match."`
}

func (c *listCommand) referencedIssueIDs() []string {
	if c.UnderID == "" {
		return nil
	}
	return []string{c.UnderID}
}

func (*listCommand) Help() string {
	return "Filter board issues and render them in deterministic domain order."
}

// ListIssuesOperation selects issue summaries from one board.
type ListIssuesOperation interface {
	// ListIssues returns issue summaries matching the request.
	ListIssues(context.Context, issue.ListRequest) ([]issue.Summary, error)
}

func (c *listCommand) Run(inv *Invocation, operation ListIssuesOperation) error {
	if c.Limit < 0 {
		return UsageErrorf("--limit must not be negative")
	}
	if c.Type != "" && !slices.Contains(
		[]string{"workstream", "task", "checkpoint", "routine"},
		c.Type,
	) {
		return UsageErrorf("unsupported issue type %q", c.Type)
	}
	if c.Sort != "" && !slices.Contains(
		[]string{"priority", "created", "updated", "closed", "id", "title", "type"},
		c.Sort,
	) {
		return UsageErrorf("unsupported sort field %q", c.Sort)
	}
	var titleRegexp *regexp.Regexp
	if c.TitleRegexp != "" {
		var err error
		titleRegexp, err = regexp.Compile(c.TitleRegexp)
		if err != nil {
			return UsageErrorf("invalid --title-regexp %q: %v", c.TitleRegexp, err)
		}
	}
	statuses := slices.Clone(c.Statuses)
	if len(statuses) == 0 {
		for _, status := range issue.NonTerminalStatuses() {
			statuses = append(statuses, status.String())
		}
	}
	result, err := operation.ListIssues(inv.Context, issue.ListRequest{
		UnderID: c.UnderID, Statuses: statuses, Assignee: c.Assignee,
		Type: c.Type, LabelsAll: c.Labels.add, LabelsAny: c.LabelsAny,
		LabelsNone: c.Labels.remove,
		NoAssignee: c.NoAssignee, TitleRegexp: titleRegexp,
		Sort: c.Sort, Reverse: c.Reverse, Limit: c.Limit,
	})
	if err != nil {
		return err
	}
	return renderIssueSummaries(inv.Output, result)
}

type readyCommand struct {
	Limit int `name:"limit" default:"20" placeholder:"COUNT" help:"Maximum results; must be positive. Defaults to 20."`
}

func (*readyCommand) Help() string {
	return "List ready executable issues whose prerequisites are closed."
}

// ListReadyIssuesOperation selects executable issues without blockers.
type ListReadyIssuesOperation interface {
	// ListReadyIssues returns ready issues in claim order.
	ListReadyIssues(context.Context, issue.ListReadyRequest) ([]issue.Summary, error)
}

func (c *readyCommand) Run(inv *Invocation, operation ListReadyIssuesOperation) error {
	if c.Limit <= 0 {
		return UsageErrorf("--limit must be positive")
	}
	result, err := operation.ListReadyIssues(inv.Context, issue.ListReadyRequest{Limit: c.Limit})
	if err != nil {
		return err
	}
	return renderIssueSummaries(inv.Output, result)
}

type blockedCommand struct {
	Limit int `name:"limit" default:"20" placeholder:"COUNT" help:"Maximum results; must be positive. Defaults to 20."`
}

func (*blockedCommand) Help() string {
	return "List open, unclaimed, non-routine issues with unresolved prerequisites."
}

// ListBlockedIssuesOperation selects issues with unresolved prerequisites.
type ListBlockedIssuesOperation interface {
	// ListBlockedIssues returns blocked issues in domain order.
	ListBlockedIssues(context.Context, issue.ListBlockedRequest) ([]issue.Summary, error)
}

func (c *blockedCommand) Run(inv *Invocation, operation ListBlockedIssuesOperation) error {
	if c.Limit <= 0 {
		return UsageErrorf("--limit must be positive")
	}
	result, err := operation.ListBlockedIssues(inv.Context, issue.ListBlockedRequest{Limit: c.Limit})
	if err != nil {
		return err
	}
	return renderIssueSummaries(inv.Output, result)
}

func renderIssueSummaries(output *Output, summaries []issue.Summary) error {
	if output.JSON() {
		records := make([]issueSummaryOutput, len(summaries))
		for index, summary := range summaries {
			records[index] = newIssueSummaryOutput(summary)
		}
		return WriteJSONLines(output, records)
	}

	writer := tabwriter.NewWriter(output.Stdout(), 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "ID\tPRI\tSTATUS\tTYPE\tTITLE"); err != nil {
		return fmt.Errorf("write issue list header: %w", err)
	}
	for _, summary := range summaries {
		if _, err := fmt.Fprintf(
			writer, "%s\t%d\t%s\t%s\t%s\n",
			summary.Issue.ID, summary.Issue.Priority, summary.Issue.Status,
			summary.Issue.Type, singleLine(summary.Issue.Title),
		); err != nil {
			return fmt.Errorf("write issue list: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush issue list: %w", err)
	}
	return nil
}

func singleLine(value string) string {
	return strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(value)
}

type showCommand struct {
	ID           string `arg:"" name:"id" predictor:"issues" help:"Issue ID."`
	Context      bool   `name:"context" help:"Include board, ancestor, and direct-dependency context."`
	ContextDepth int    `name:"context-depth" placeholder:"DEPTH" help:"Maximum ancestor count. Zero includes every ancestor."`
}

func (c *showCommand) referencedIssueIDs() []string { return []string{c.ID} }

func (*showCommand) Help() string {
	return "Show one issue's current metadata, relationships, records, and optional inherited context."
}

// ReadIssueOperation reconstructs one issue and its optional inherited context.
type ReadIssueOperation interface {
	// ReadIssue returns the requested issue view.
	ReadIssue(context.Context, issue.ReadRequest) (attachment.IssueView, error)
}

func (c *showCommand) Run(inv *Invocation, operation ReadIssueOperation) error {
	if c.ContextDepth < 0 {
		return UsageErrorf("--context-depth must not be negative")
	}
	if c.ContextDepth != 0 && !c.Context {
		return UsageErrorf("--context-depth requires --context")
	}
	var depth *int
	if c.Context {
		depth = &c.ContextDepth
	}
	result, err := operation.ReadIssue(inv.Context, issue.ReadRequest{
		IssueID: c.ID, ContextDepth: depth,
	})
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(newIssueInspectionOutput(result).jsonValue())
	}
	if result.Issue.Context != nil {
		if err := writeIssueContext(inv.Output, *result.Issue.Context); err != nil {
			return err
		}
	}
	return writeIssueDetail(inv.Output, result.Issue.Detail, result.Attachments)
}

func writeIssueContext(output *Output, current issue.Context) error {
	if current.Board.Description != nil {
		if err := output.WriteString("Board context:\n" + *current.Board.Description + "\n\n"); err != nil {
			return err
		}
	}
	if len(current.Ancestors) > 0 {
		if err := output.WriteString("Ancestors:\n"); err != nil {
			return err
		}
		for _, ancestor := range current.Ancestors {
			if err := output.WriteString(fmt.Sprintf(
				"- %s: %s (%d log entries)\n",
				ancestor.Issue.ID, ancestor.Issue.Title, ancestor.LogSummary.Count,
			)); err != nil {
				return err
			}
			if ancestor.DetailsBytes > 0 {
				if err := output.WriteString(fmt.Sprintf(
					"  %d bytes of details available: %s\n",
					ancestor.DetailsBytes, ancestor.Issue.ID,
				)); err != nil {
					return err
				}
			}
		}
		if err := output.WriteString("\n"); err != nil {
			return err
		}
	}
	if len(current.DependencyResults) > 0 {
		if err := output.WriteString("Dependency results:\n"); err != nil {
			return err
		}
		for _, result := range current.DependencyResults {
			if err := output.WriteString("- " + result.Issue.ID + ": " + result.Issue.Title + "\n" + result.Body + "\n"); err != nil {
				return err
			}
		}
		if err := output.WriteString("\n"); err != nil {
			return err
		}
	}
	return nil
}
