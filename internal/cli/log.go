package cli

import (
	"context"
	"fmt"
	"strings"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/record"
)

type logCommand struct {
	Post logPostCommand `cmd:"" help:"Post an immutable issue log entry."`
	Show logShowCommand `cmd:"" help:"Show issue log entries."`
}

// Help distinguishes immutable log entries from mutable recovery state.
func (*logCommand) Help() string {
	return "Post and show immutable attributed Markdown log entries."
}

//go:generate go tool mockgen -destination log_mocks_test.go -package cli -typed -write_package_comment=false . LogEntryWriteOperations,LogEntryReadOperations

// LogEntryWriteOperations appends immutable issue log entries.
type LogEntryWriteOperations interface {
	AddLogEntry(context.Context, issue.Invocation, record.AddLogEntryRequest) (record.AddLogEntryResult, error)
}

var _ LogEntryWriteOperations = (*record.Recorder)(nil)

type logPostCommand struct {
	ID   issueID `arg:"" name:"id" help:"Issue receiving the log entry."`
	Body *string `arg:"" optional:"" name:"body" help:"Markdown body. Use - or omit with piped input to read standard input."`
}

func (c *logPostCommand) referencedIssueIDs() []string { return []string{c.ID.String()} }

// Help describes immutable log input and attribution.
func (*logPostCommand) Help() string {
	return "Post one immutable Markdown log entry attributed to the invocation actor."
}

// Run selects Markdown input and posts one log entry.
func (c *logPostCommand) Run(inv *Invocation, markdown *MarkdownInput, operations LogEntryWriteOperations) error {
	body, provided, err := markdown.Read(c.Body)
	if err != nil {
		return err
	}
	if !provided {
		return UsageErrorf("log body is required as an argument or standard input")
	}
	result, err := operations.AddLogEntry(
		inv.Context,
		issue.NewInvocation(inv.Actor),
		record.AddLogEntryRequest{IssueID: c.ID.String(), Body: body},
	)
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(newLogEntryOutput(result.LogEntry))
	}
	return inv.Output.Noticef("Posted log entry %s to %s.", result.LogEntry.ID, result.LogEntry.IssueID)
}

// LogEntryReadOperations reads issue log entries in durable order.
type LogEntryReadOperations interface {
	ListLogEntries(context.Context, issue.LogListRequest) ([]issue.LogEntry, error)
	GetLogEntry(context.Context, record.GetLogEntryRequest) (issue.LogEntry, error)
}

var _ LogEntryReadOperations = (*record.Recorder)(nil)

// logShowID accepts either namespace used by `card log show`.
type logShowID string

// UnmarshalText rejects selectors outside the issue and Log ID grammars.
func (id *logShowID) UnmarshalText(text []byte) error {
	value := string(text)
	if parsed, err := issue.NewLogID(value); err == nil {
		*id = logShowID(parsed.String())
		return nil
	}
	parsed, err := parseIssueID(value)
	if err != nil {
		return fmt.Errorf("expected an issue ID or stable Log ID: %w", err)
	}
	*id = logShowID(parsed.String())
	return nil
}

func (id logShowID) String() string { return string(id) }

type logShowCommand struct {
	ID          logShowID `arg:"" name:"id" help:"Issue ID to list, or stable Log ID to show exactly."`
	Limit       int       `name:"limit" default:"0" placeholder:"COUNT" help:"Maximum entries after ordering; 0 lists all."`
	OldestFirst bool      `name:"oldest-first" help:"Show entries in chronological order."`
}

func (c *logShowCommand) referencedIssueIDs() []string {
	if _, err := issue.NewLogID(c.ID.String()); err == nil {
		return nil
	}
	return []string{c.ID.String()}
}

// Help describes durable log ordering and limits.
func (*logShowCommand) Help() string {
	return "Show one immutable Log entry by stable ID, or list an issue's Log newest-first."
}

// Run shows an exact Log entry or an issue's ordered Log entries.
func (c *logShowCommand) Run(inv *Invocation, operations LogEntryReadOperations) error {
	if logID, err := issue.NewLogID(c.ID.String()); err == nil {
		if c.Limit != 0 || c.OldestFirst {
			return UsageErrorf("--limit and --oldest-first require an issue ID")
		}
		entry, err := operations.GetLogEntry(
			inv.Context,
			record.GetLogEntryRequest{LogID: logID.String()},
		)
		if err != nil {
			return err
		}
		if inv.Output.JSON() {
			return inv.Output.WriteJSON(newLogEntryOutput(entry))
		}
		return writeLogEntries(inv.Output, []issue.LogEntry{entry})
	}
	entries, err := operations.ListLogEntries(inv.Context, issue.LogListRequest{
		IssueID: c.ID.String(), Reverse: !c.OldestFirst, Limit: c.Limit,
	})
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		output := make([]logEntryOutput, len(entries))
		for i, entry := range entries {
			output[i] = newLogEntryOutput(entry)
		}
		return WriteJSONLines(inv.Output, output)
	}
	return writeLogEntries(inv.Output, entries)
}

func writeLogEntries(output *Output, entries []issue.LogEntry) error {
	var builder strings.Builder
	for i, entry := range entries {
		if i > 0 {
			builder.WriteByte('\n')
		}
		writeLogEntryHeading(&builder, entry)
		writeMarkdown(&builder, entry.Body)
		if entry.NextAction != nil {
			builder.WriteString("\n**Planned next action**\n\n")
			writeMarkdown(&builder, *entry.NextAction)
		}
	}
	return output.WriteString(builder.String())
}

func writeLogEntryHeading(output *strings.Builder, entry issue.LogEntry) {
	switch entry.Kind {
	case issue.LogEntryKindPost.String():
		fmt.Fprintf(output, "Post %s", entry.ID)
	case issue.LogEntryKindStateSnapshot.String():
		fmt.Fprintf(output, "State snapshot %s", entry.ID)
	default:
		fmt.Fprintf(output, "Log entry %s", entry.ID)
	}
	if entry.Author != nil {
		fmt.Fprintf(output, " by %s", *entry.Author)
	}
	if entry.Kind == issue.LogEntryKindStateSnapshot.String() &&
		entry.Committer != nil &&
		(entry.Author == nil || *entry.Committer != *entry.Author) {
		fmt.Fprintf(output, " committed by %s", *entry.Committer)
	}
	output.WriteByte('\n')
}

type logEntryOutput struct {
	ID         issue.LogID `json:"id"`
	IssueID    string      `json:"issue_id"`
	Kind       string      `json:"kind"`
	Author     *string     `json:"author,omitempty"`
	Committer  *string     `json:"committer,omitempty"`
	Body       string      `json:"body"`
	NextAction *string     `json:"next_action,omitempty"`
	Created    *int64      `json:"created,omitempty"`
}

func newLogEntryOutput(entry issue.LogEntry) logEntryOutput {
	return logEntryOutput{
		ID:         entry.ID,
		IssueID:    entry.IssueID,
		Kind:       entry.Kind,
		Author:     entry.Author,
		Committer:  entry.Committer,
		Body:       entry.Body,
		NextAction: entry.NextAction,
		Created:    entry.Created,
	}
}
