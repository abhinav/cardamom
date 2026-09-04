package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/searchquery"
)

type searchCommand struct {
	Query         string     `arg:"" name:"query" help:"Terms, phrases, operators, and trailing prefix matches."`
	Literal       bool       `name:"literal" help:"Treat the complete query as one literal phrase."`
	Fields        []string   `name:"in" sep:"," enum:"title,summary,details,state,result,log" placeholder:"FIELD" help:"Search only these fields. Repeat or separate values with commas."`
	UnderID       issueID    `name:"under" predictor:"issues" placeholder:"ISSUE" help:"Match strict containment descendants of this issue."`
	Statuses      []string   `name:"status" sep:"," enum:"ready,blocked,in_progress,waiting,closed,cancelled" placeholder:"STATUS" help:"Effective issue status to include. Repeat or separate values with commas."`
	Assignee      *string    `name:"assignee" placeholder:"ACTOR" help:"Match active custody by this actor."`
	Type          string     `name:"type" placeholder:"TYPE" help:"Match one issue type: workstream, task, checkpoint, or routine."`
	Labels        labelTerms `name:"label" predictor:"labels" placeholder:"TERM" help:"Label term. No prefix or + requires; - excludes. Repeat for multiple labels."`
	LabelsAny     []string   `name:"label-any" placeholder:"LABEL" help:"Match issues carrying at least one supplied label. Repeat for alternatives."`
	NoAssignee    bool       `name:"no-assignee" help:"Match issues without active custody."`
	CreatedSince  string     `name:"created-since" placeholder:"TIME" help:"Include issues created at or after this RFC 3339 time."`
	CreatedBefore string     `name:"created-before" placeholder:"TIME" help:"Include issues created before this RFC 3339 time."`
	ClosedSince   string     `name:"closed-since" placeholder:"TIME" help:"Include terminal issues closed or cancelled at or after this RFC 3339 time."`
	ClosedBefore  string     `name:"closed-before" placeholder:"TIME" help:"Include terminal issues closed or cancelled before this RFC 3339 time."`
	Sort          string     `name:"sort" default:"relevance" placeholder:"FIELD" help:"Sort by relevance, priority, created, updated, closed, title, or type."`
	Reverse       bool       `name:"reverse" help:"Reverse a non-relevance sort."`
	Limit         int        `name:"limit" default:"20" placeholder:"COUNT" help:"Maximum issues. Zero returns every match. Defaults to 20."`
}

func (c *searchCommand) referencedIssueIDs() []string {
	if c.UnderID == "" {
		return nil
	}
	return []string{c.UnderID.String()}
}

// Help summarizes the public query language without exposing FTS5 syntax.
func (*searchCommand) Help() string {
	return "Search issue records with phrases, implicit AND, OR, infix NOT, grouping, and trailing *."
}

//go:generate go tool mockgen -destination search_mocks_test.go -package cli -typed -write_package_comment=false . SearchIssuesOperation

// SearchIssuesOperation searches one selected board's issue records.
type SearchIssuesOperation interface {
	// SearchIssues returns ranked and filtered issue matches.
	SearchIssues(context.Context, issue.SearchRequest) (issue.SearchResult, error)
}

func (c *searchCommand) Run(inv *Invocation, operation SearchIssuesOperation) error {
	request, err := c.request()
	if err != nil {
		return err
	}
	result, err := operation.SearchIssues(inv.Context, request)
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(newSearchOutput(result))
	}
	return writeSearchResult(inv.Output, result)
}

func (c *searchCommand) request() (issue.SearchRequest, error) {
	if c.Limit < 0 {
		return issue.SearchRequest{}, UsageErrorf("--limit must not be negative")
	}
	if c.Type != "" && !slices.Contains(
		[]string{"workstream", "task", "checkpoint", "routine"},
		c.Type,
	) {
		return issue.SearchRequest{}, UsageErrorf("unsupported issue type %q", c.Type)
	}
	if !slices.Contains(
		[]string{"relevance", "priority", "created", "updated", "closed", "title", "type"},
		c.Sort,
	) {
		return issue.SearchRequest{}, UsageErrorf("unsupported sort field %q", c.Sort)
	}
	if c.Sort == "relevance" && c.Reverse {
		return issue.SearchRequest{}, UsageErrorf("--reverse cannot be used with relevance order")
	}
	if c.Assignee != nil && c.NoAssignee {
		return issue.SearchRequest{}, UsageErrorf("--assignee and --no-assignee cannot be combined")
	}

	query, err := searchquery.Parse(c.Query)
	if c.Literal {
		query, err = searchquery.Literal(c.Query)
	}
	if err != nil {
		return issue.SearchRequest{}, UsageErrorf("%v", err)
	}
	fields := make([]issue.SearchField, len(c.Fields))
	seenFields := make(map[issue.SearchField]struct{}, len(c.Fields))
	for index, value := range c.Fields {
		fields[index], err = issue.NewSearchField(value)
		if err != nil {
			return issue.SearchRequest{}, UsageErrorf("%v", err)
		}
		if _, ok := seenFields[fields[index]]; ok {
			return issue.SearchRequest{}, UsageErrorf("duplicate --in field %q", value)
		}
		seenFields[fields[index]] = struct{}{}
	}
	createdSince, err := parseSearchTime("--created-since", c.CreatedSince)
	if err != nil {
		return issue.SearchRequest{}, err
	}
	createdBefore, err := parseSearchTime("--created-before", c.CreatedBefore)
	if err != nil {
		return issue.SearchRequest{}, err
	}
	closedSince, err := parseSearchTime("--closed-since", c.ClosedSince)
	if err != nil {
		return issue.SearchRequest{}, err
	}
	closedBefore, err := parseSearchTime("--closed-before", c.ClosedBefore)
	if err != nil {
		return issue.SearchRequest{}, err
	}
	if invalidSearchTimeRange(createdSince, createdBefore) {
		return issue.SearchRequest{}, UsageErrorf(
			"--created-since must be before --created-before",
		)
	}
	if invalidSearchTimeRange(closedSince, closedBefore) {
		return issue.SearchRequest{}, UsageErrorf(
			"--closed-since must be before --closed-before",
		)
	}
	return issue.SearchRequest{
		Query: query, Fields: fields, UnderID: c.UnderID.String(),
		Statuses: c.Statuses, Assignee: c.Assignee, Type: c.Type,
		LabelsAll: c.Labels.add, LabelsAny: c.LabelsAny,
		LabelsNone: c.Labels.remove, NoAssignee: c.NoAssignee,
		CreatedSince: createdSince, CreatedBefore: createdBefore,
		ClosedSince: closedSince, ClosedBefore: closedBefore,
		Sort: c.Sort, Reverse: c.Reverse, Limit: c.Limit,
	}, nil
}

func invalidSearchTimeRange(since, before *time.Time) bool {
	return since != nil && before != nil && !since.Before(*before)
}

func parseSearchTime(flag, value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, UsageErrorf("%s must be an RFC 3339 time: %v", flag, err)
	}
	return &parsed, nil
}

type searchOutput struct {
	Total   int                 `json:"total"`
	Matches []searchMatchOutput `json:"matches"`
}

type searchMatchOutput struct {
	issueSummaryOutput
	MatchedIn []string            `json:"matched_in"`
	Excerpt   searchExcerptOutput `json:"excerpt"`
}

type searchExcerptOutput struct {
	Field    string       `json:"field"`
	RecordID *issue.LogID `json:"record_id,omitempty"`
	Text     string       `json:"text"`
}

func newSearchOutput(result issue.SearchResult) searchOutput {
	matches := make([]searchMatchOutput, len(result.Matches))
	for index, match := range result.Matches {
		fields := make([]string, len(match.MatchedFields))
		for fieldIndex, field := range match.MatchedFields {
			fields[fieldIndex] = field.String()
		}
		matches[index] = searchMatchOutput{
			issueSummaryOutput: newIssueSummaryOutput(match.Summary),
			MatchedIn:          fields,
			Excerpt: searchExcerptOutput{
				Field:    match.Excerpt.Field.String(),
				RecordID: match.Excerpt.RecordID,
				Text:     match.Excerpt.Text,
			},
		}
	}
	return searchOutput{Total: result.Total, Matches: matches}
}

func writeSearchResult(output *Output, result issue.SearchResult) error {
	shown := len(result.Matches)
	if result.Total == shown {
		if err := output.WriteString(fmt.Sprintf(
			"%d %s\n",
			result.Total,
			plural(result.Total, "match", "matches"),
		)); err != nil {
			return err
		}
	} else if err := output.WriteString(fmt.Sprintf(
		"%d matches (showing %d)\n",
		result.Total,
		shown,
	)); err != nil {
		return err
	}
	if shown == 0 {
		return nil
	}

	writer := tabwriter.NewWriter(output.Stdout(), 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "ID\tSTATUS\tMATCH\tTITLE"); err != nil {
		return fmt.Errorf("write search header: %w", err)
	}
	for _, match := range result.Matches {
		fields := make([]string, len(match.MatchedFields))
		for index, field := range match.MatchedFields {
			fields[index] = field.String()
		}
		if _, err := fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\n",
			match.Summary.Issue.ID,
			match.Summary.Issue.Status,
			strings.Join(fields, ","),
			singleLine(match.Summary.Issue.Title),
		); err != nil {
			return fmt.Errorf("write search match: %w", err)
		}
		source := match.Excerpt.Field.String()
		if match.Excerpt.RecordID != nil {
			source += " " + match.Excerpt.RecordID.String()
		}
		if _, err := fmt.Fprintf(
			writer,
			"  %s:\t%s\n",
			source,
			singleLine(match.Excerpt.Text),
		); err != nil {
			return fmt.Errorf("write search excerpt: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush search result: %w", err)
	}
	return nil
}
