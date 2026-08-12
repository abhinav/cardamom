package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/issue/record"
)

// Generate typed mocks for command operation contracts whose tests only
// configure calls and results.
//
//go:generate go tool mockgen -destination mocks_test.go -package cli -typed -write_package_comment=false . BackupOperation,RestoreOperation,DumpOperation,LeaseOperations,InitOperation,InfoOperation,WebOperation,ListIssuesOperation,ListReadyIssuesOperation,ListBlockedIssuesOperation,ReadIssueOperation,CreateIssueOperation,ApplyDocumentOperation,EditIssueOperation,ClaimOperations,ReleaseOperations,CloseOperations,CancelOperations,ReopenOperations,CheckpointOperations,LogEntryWriteOperations,LogEntryReadOperations,StateWriteOperations,StateReadOperations,StateCommitOperations,ResultWriteOperations,ResultReadOperations

// ClaimOperations supplies the two domain-owned claim modes.
type ClaimOperations interface {
	ClaimIssue(context.Context, issue.Invocation, execution.ClaimIssueRequest) (execution.ClaimIssueResult, error)
	ClaimNext(context.Context, issue.Invocation, execution.ClaimNextRequest) (execution.ClaimIssueResult, error)
}

var _ ClaimOperations = (*execution.Executor)(nil)

type claimCommand struct {
	ID        string     `arg:"" optional:"" name:"id" predictor:"issues" help:"Issue to claim directly. Routines may be claimed only by ID."`
	Under     string     `name:"under" predictor:"issues" placeholder:"ISSUE" help:"Limit automatic selection to strict descendants of this issue."`
	Labels    labelTerms `name:"label" short:"l" predictor:"labels" placeholder:"TERM" help:"Label term for automatic selection. No prefix or + requires; - excludes. Repeat for multiple labels."`
	LabelsAny []string   `name:"label-any" predictor:"labels" placeholder:"LABEL" help:"Alternative label during automatic selection. Repeat to require at least one."`
	Context   bool       `name:"context" help:"Include shared and inherited current context."`
	Watch     bool       `name:"watch" help:"Wait until matching ready work is available."`
}

func (c *claimCommand) referencedIssueIDs() []string {
	if c.ID != "" {
		return []string{c.ID}
	}
	if c.Under == "" {
		return nil
	}
	return []string{c.Under}
}

// Help explains direct and automatic claim selection.
func (*claimCommand) Help() string {
	return `Claim one issue for the invocation actor.

With an ID, claim that issue directly, including a routine. Without an ID,
automatic selection uses ready priority order and applies --under and every
positive --label filter, excludes every negative --label filter, and requires
one --label-any match when supplied. --watch waits until matching work is
available or the invocation is cancelled.`
}

// Run selects the claim mode and renders the committed issue view.
func (c *claimCommand) Run(inv *Invocation, operations ClaimOperations) error {
	if c.ID != "" && (c.Under != "" || len(c.Labels.add) > 0 || len(c.Labels.remove) > 0 || len(c.LabelsAny) > 0 || c.Watch) {
		return UsageErrorf("--under, --label, --label-any, and --watch require automatic claim selection without an ID")
	}

	var contextDepth *int
	if c.Context {
		depth := 0
		contextDepth = &depth
	}

	domainInvocation := issue.NewInvocation(inv.Actor)
	var result execution.ClaimIssueResult
	var err error
	if c.ID != "" {
		result, err = operations.ClaimIssue(inv.Context, domainInvocation, execution.ClaimIssueRequest{
			ID: c.ID, Assignee: inv.Actor, ContextDepth: contextDepth,
		})
	} else {
		result, err = operations.ClaimNext(inv.Context, domainInvocation, execution.ClaimNextRequest{
			UnderID: c.Under, Assignee: inv.Actor,
			LabelsAll: c.Labels.add, LabelsAny: c.LabelsAny,
			LabelsNone: c.Labels.remove,
			Watch:      c.Watch, ContextDepth: contextDepth,
		})
	}
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(newIssueViewOutput(result.Issue).jsonValue())
	}
	if err := inv.Output.Noticef("Claimed %s as %s.", result.Issue.Detail.Issue.ID, inv.Actor); err != nil {
		return err
	}
	return inv.Output.WriteString(formatIssueView(result.Issue))
}

// ReleaseOperations relinquishes domain-owned claim custody.
type ReleaseOperations interface {
	ReleaseIssue(context.Context, issue.Invocation, execution.ReleaseIssueRequest) (execution.ReleaseIssueResult, error)
}

var _ ReleaseOperations = (*execution.Executor)(nil)

type releaseCommand struct {
	ID      string  `arg:"" name:"id" predictor:"issues" help:"Issue whose active claim will be released."`
	Waiting *string `name:"waiting" placeholder:"REASON" help:"Release into waiting status with this required plain-text reason."`
}

func (c *releaseCommand) referencedIssueIDs() []string { return []string{c.ID} }

// Help describes actor-owned custody release.
func (*releaseCommand) Help() string {
	return "Release the invocation actor's active claim into ready or waiting status."
}

// Run releases one claim through the domain operation.
func (c *releaseCommand) Run(inv *Invocation, operations ReleaseOperations) error {
	result, err := operations.ReleaseIssue(
		inv.Context,
		issue.NewInvocation(inv.Actor),
		execution.ReleaseIssueRequest{ID: c.ID, WaitingReason: c.Waiting},
	)
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(newIssueDetailOutput(result.Issue))
	}
	if result.Issue.Issue.Waiting != nil {
		return inv.Output.Noticef("Released %s as waiting.", result.Issue.Issue.ID)
	}
	return inv.Output.Noticef("Released %s as ready.", result.Issue.Issue.ID)
}

// CloseOperations completes requested issues under domain lifecycle policy.
type CloseOperations interface {
	CloseIssues(context.Context, issue.Invocation, execution.CloseIssuesRequest) (execution.CloseIssuesResult, error)
}

var _ CloseOperations = (*execution.Executor)(nil)

type closeCommand struct {
	IDs []string `arg:"" name:"id" help:"Issues to close in the requested order."`
}

func (c *closeCommand) referencedIssueIDs() []string { return slices.Clone(c.IDs) }

// Help describes lifecycle constraints not carried by the command summary.
func (*closeCommand) Help() string {
	return `Checkpoints require approve or deny.
Workstreams and routines require every direct child to be terminal.`
}

// Run closes the requested issues and reports parent readiness notices.
func (c *closeCommand) Run(inv *Invocation, operations CloseOperations) error {
	result, err := operations.CloseIssues(
		inv.Context,
		issue.NewInvocation(inv.Actor),
		execution.CloseIssuesRequest{IDs: c.IDs},
	)
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(newCloseIssuesOutput(result))
	}
	for _, closed := range result.Issues {
		if err := inv.Output.Noticef("Closed %s.", closed.Issue.ID); err != nil {
			return err
		}
	}
	return writeParentNotices(inv.Output, result.ParentsWithoutOpenChildren)
}

// CancelOperations cancels requested roots and their dependent closure.
type CancelOperations interface {
	CancelIssues(context.Context, issue.Invocation, execution.CancelIssuesRequest) (execution.CancelIssuesResult, error)
}

var _ CancelOperations = (*execution.Executor)(nil)

type cancelCommand struct {
	IDs []string `arg:"" name:"id" help:"Root issues to cancel."`
}

func (c *cancelCommand) referencedIssueIDs() []string { return slices.Clone(c.IDs) }

// Help describes cancellation propagation.
func (*cancelCommand) Help() string {
	return "Cancel requested issues and every non-terminal transitive dependent."
}

// Run cancels the requested issue roots and renders the committed closure.
func (c *cancelCommand) Run(inv *Invocation, operations CancelOperations) error {
	result, err := operations.CancelIssues(
		inv.Context,
		issue.NewInvocation(inv.Actor),
		execution.CancelIssuesRequest{Roots: c.IDs},
	)
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(cancelIssuesOutput{
			Issues: result.Issues, Requested: result.Requested, Dependents: result.Dependents,
			ParentsWithoutOpenChildren: result.ParentsWithoutOpenChildren,
		})
	}
	if err := inv.Output.Noticef(
		"Cancelled %d requested %s and %d dependent %s.",
		result.Requested,
		plural(result.Requested, "issue", "issues"),
		result.Dependents,
		plural(result.Dependents, "issue", "issues"),
	); err != nil {
		return err
	}
	return writeParentNotices(inv.Output, result.ParentsWithoutOpenChildren)
}

// ReopenOperations restores terminal issues to open lifecycle.
type ReopenOperations interface {
	ReopenIssues(context.Context, issue.Invocation, execution.ReopenIssuesRequest) (execution.ReopenIssuesResult, error)
}

var _ ReopenOperations = (*execution.Executor)(nil)

type reopenCommand struct {
	IDs []string `arg:"" name:"id" help:"Terminal issues to reopen in the requested order."`
}

func (c *reopenCommand) referencedIssueIDs() []string { return slices.Clone(c.IDs) }

// Help describes reopening without changing custody or prerequisites.
func (*reopenCommand) Help() string {
	return "Reopen terminal issues without claiming them or reopening prerequisites."
}

// Run reopens the requested issues and reports unresolved prerequisites.
func (c *reopenCommand) Run(inv *Invocation, operations ReopenOperations) error {
	result, err := operations.ReopenIssues(
		inv.Context,
		issue.NewInvocation(inv.Actor),
		execution.ReopenIssuesRequest{IDs: c.IDs},
	)
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(newReopenIssuesOutput(result))
	}
	for _, reopened := range result.Issues {
		if err := inv.Output.Noticef("Reopened %s.", reopened.Issue.Issue.ID); err != nil {
			return err
		}
		for _, prerequisite := range reopened.UnresolvedPrerequisites {
			if err := inv.Output.Noticef(
				"%s remains blocked by %s (%s).",
				reopened.Issue.Issue.ID,
				prerequisite.ID,
				prerequisite.Status,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

type checkpointCommand struct {
	Approve checkpointApproveCommand `cmd:"" help:"Approve an actionable checkpoint."`
	Deny    checkpointDenyCommand    `cmd:"" help:"Deny a checkpoint and cancel its dependents."`
}

// Help describes checkpoint resolution without approver assignment.
func (*checkpointCommand) Help() string {
	return `Resolve human-oriented workflow gates.

Approve closes an actionable checkpoint. Deny cancels the checkpoint and its
non-terminal dependent closure. An optional Markdown reason is stored on the
immutable decision.`
}

// CheckpointOperations resolves checkpoints through domain decisions.
type CheckpointOperations interface {
	ApproveCheckpoint(context.Context, issue.Invocation, execution.CheckpointRequest) (execution.ResolveCheckpointResult, error)
	DenyCheckpoint(context.Context, issue.Invocation, execution.CheckpointRequest) (execution.ResolveCheckpointResult, error)
}

var _ CheckpointOperations = (*execution.Executor)(nil)

type checkpointApproveCommand struct {
	ID     string  `arg:"" name:"id" help:"Checkpoint to approve."`
	Reason *string `name:"reason" placeholder:"MARKDOWN" help:"Optional Markdown reason. Use - to read standard input."`
}

func (c *checkpointApproveCommand) referencedIssueIDs() []string { return []string{c.ID} }

// Help describes successful checkpoint resolution.
func (*checkpointApproveCommand) Help() string {
	return "Approve an actionable checkpoint and atomically record an optional reason."
}

// Run maps public approval to the domain decision operation.
func (c *checkpointApproveCommand) Run(inv *Invocation, markdown *MarkdownInput, operations CheckpointOperations) error {
	reason, _, err := markdown.Read(c.Reason)
	if err != nil {
		return err
	}
	result, err := operations.ApproveCheckpoint(
		inv.Context,
		issue.NewInvocation(inv.Actor),
		execution.CheckpointRequest{IssueID: c.ID, Reason: reason},
	)
	return renderCheckpointResult(inv.Output, c.ID, true, result, err)
}

type checkpointDenyCommand struct {
	ID     string  `arg:"" name:"id" help:"Checkpoint to deny."`
	Reason *string `name:"reason" placeholder:"MARKDOWN" help:"Optional Markdown reason. Use - to read standard input."`
}

func (c *checkpointDenyCommand) referencedIssueIDs() []string { return []string{c.ID} }

// Help describes failed checkpoint resolution.
func (*checkpointDenyCommand) Help() string {
	return "Deny an actionable checkpoint, cancel its dependents, and atomically record an optional reason."
}

// Run maps public denial to the domain decision operation.
func (c *checkpointDenyCommand) Run(inv *Invocation, markdown *MarkdownInput, operations CheckpointOperations) error {
	reason, _, err := markdown.Read(c.Reason)
	if err != nil {
		return err
	}
	result, err := operations.DenyCheckpoint(
		inv.Context,
		issue.NewInvocation(inv.Actor),
		execution.CheckpointRequest{IssueID: c.ID, Reason: reason},
	)
	return renderCheckpointResult(inv.Output, c.ID, false, result, err)
}

func renderCheckpointResult(output *Output, id string, approved bool, result execution.ResolveCheckpointResult, err error) error {
	if err != nil {
		return err
	}
	if output.JSON() {
		return output.WriteJSON(checkpointOutput{
			Decision: result.Decision, Issue: result.Issue, Cancelled: result.Cancelled,
			ParentsWithoutOpenChildren: result.ParentsWithoutOpenChildren,
		})
	}
	if approved {
		if err := output.Noticef("Approved %s.", id); err != nil {
			return err
		}
	} else {
		dependents := len(result.Cancelled)
		if dependents > 0 {
			dependents--
		}
		if err := output.Noticef(
			"Denied %s; cancelled %d dependent %s.",
			id,
			dependents,
			plural(dependents, "issue", "issues"),
		); err != nil {
			return err
		}
	}
	return writeParentNotices(output, result.ParentsWithoutOpenChildren)
}

type logCommand struct {
	Post logPostCommand `cmd:"" help:"Post an immutable issue log entry."`
	Show logShowCommand `cmd:"" help:"Show issue log entries."`
}

// Help distinguishes immutable log entries from mutable recovery state.
func (*logCommand) Help() string {
	return "Post and show immutable attributed Markdown log entries."
}

// LogEntryWriteOperations appends immutable issue log entries.
type LogEntryWriteOperations interface {
	AddLogEntry(context.Context, issue.Invocation, record.AddLogEntryRequest) (record.AddLogEntryResult, error)
}

var _ LogEntryWriteOperations = (*record.Recorder)(nil)

type logPostCommand struct {
	ID   string  `arg:"" name:"id" help:"Issue receiving the log entry."`
	Body *string `arg:"" optional:"" name:"body" help:"Markdown body. Use - or omit with piped input to read standard input."`
}

func (c *logPostCommand) referencedIssueIDs() []string { return []string{c.ID} }

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
		record.AddLogEntryRequest{IssueID: c.ID, Body: body},
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
}

var _ LogEntryReadOperations = (*record.Recorder)(nil)

type logShowCommand struct {
	ID          string `arg:"" name:"id" help:"Issue whose log entries will be shown."`
	Limit       int    `name:"limit" default:"0" placeholder:"COUNT" help:"Maximum entries after ordering; 0 lists all."`
	OldestFirst bool   `name:"oldest-first" help:"Show entries in chronological order."`
}

func (c *logShowCommand) referencedIssueIDs() []string { return []string{c.ID} }

// Help describes durable log ordering and limits.
func (*logShowCommand) Help() string {
	return "Show immutable log entries. Newest entries are shown first; --limit applies after ordering."
}

// Run shows log entries as human records or JSON Lines.
func (c *logShowCommand) Run(inv *Invocation, operations LogEntryReadOperations) error {
	entries, err := operations.ListLogEntries(inv.Context, issue.LogListRequest{
		IssueID: c.ID, Reverse: !c.OldestFirst, Limit: c.Limit,
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
	var output strings.Builder
	for i, entry := range entries {
		if i > 0 {
			output.WriteByte('\n')
		}
		writeLogEntryHeading(&output, entry)
		writeMarkdown(&output, entry.Body)
		if entry.NextAction != nil {
			output.WriteString("\n**Planned next action**\n\n")
			writeMarkdown(&output, *entry.NextAction)
		}
	}
	return inv.Output.WriteString(output.String())
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

type stateCommand struct {
	Show   stateShowCommand   `cmd:"" help:"Show the issue recovery State."`
	Set    stateSetCommand    `cmd:"" help:"Set or clear the issue recovery State."`
	Append stateAppendCommand `cmd:"" help:"Append to the issue recovery State."`
	Commit stateCommitCommand `cmd:"" help:"Commit the issue recovery State to the Log."`
}

// Help describes the single mutable recovery record.
func (*stateCommand) Help() string {
	return "Show, set, append, or commit one mutable Markdown recovery State."
}

// StateWriteOperations changes one issue's mutable recovery state.
type StateWriteOperations interface {
	SetState(context.Context, issue.Invocation, record.SetStateRequest) (record.StateResult, error)
	AppendState(context.Context, issue.Invocation, record.SetStateRequest) (record.StateResult, error)
}

var _ StateWriteOperations = (*record.Recorder)(nil)

type stateSetCommand struct {
	ID   string  `arg:"" name:"id" help:"Issue whose State will be set or cleared."`
	Text *string `arg:"" optional:"" name:"text" help:"State body Markdown. Use - or omit with piped input to read standard input."`
	Next *string `name:"next" placeholder:"ACTION" help:"Optional next-action Markdown. Use - to read standard input."`
}

func (c *stateSetCommand) referencedIssueIDs() []string { return []string{c.ID} }

// Help describes State setting and explicit removal.
func (*stateSetCommand) Help() string {
	return "Set the complete mutable recovery State. An explicitly empty body clears it."
}

// Run sets or clears one issue State from selected Markdown input.
func (c *stateSetCommand) Run(inv *Invocation, markdown *MarkdownInput, operations StateWriteOperations) error {
	inputs := []*string{c.Text}
	if c.Next != nil {
		inputs = append(inputs, c.Next)
	}
	if err := markdown.ValidateSingleStdinConsumer(inputs...); err != nil {
		return err
	}
	text, provided, err := markdown.Read(c.Text)
	if err != nil {
		return err
	}
	if !provided {
		return UsageErrorf("state text is required as an argument or standard input")
	}
	if text == "" && c.Next != nil {
		return UsageErrorf("--next requires non-empty state text")
	}
	if text != "" && strings.TrimSpace(text) == "" {
		return UsageErrorf("state body must not be blank")
	}
	nextAction := ""
	if c.Next != nil {
		nextAction, _, err = markdown.Read(c.Next)
		if err != nil {
			return err
		}
		if strings.TrimSpace(nextAction) == "" {
			return UsageErrorf("next action must not be blank")
		}
	}
	result, err := operations.SetState(
		inv.Context,
		issue.NewInvocation(inv.Actor),
		record.SetStateRequest{
			IssueID: c.ID, Text: text, NextAction: nextAction,
		},
	)
	return renderStateMutation(inv.Output, "Set state on", result, err)
}

type stateAppendCommand struct {
	ID   string  `arg:"" name:"id" help:"Issue whose state will be extended."`
	Text *string `arg:"" optional:"" name:"text" help:"Markdown to append. Use - or omit with piped input to read standard input."`
}

func (c *stateAppendCommand) referencedIssueIDs() []string { return []string{c.ID} }

// Help describes state append input and separation.
func (*stateAppendCommand) Help() string {
	return "Append Markdown to mutable recovery state, separated by one newline."
}

// Run appends selected Markdown input to one issue state.
func (c *stateAppendCommand) Run(inv *Invocation, markdown *MarkdownInput, operations StateWriteOperations) error {
	text, provided, err := markdown.Read(c.Text)
	if err != nil {
		return err
	}
	if !provided {
		return UsageErrorf("state text is required as an argument or standard input")
	}
	if strings.TrimSpace(text) == "" {
		return UsageErrorf("state body must not be blank")
	}
	result, err := operations.AppendState(
		inv.Context,
		issue.NewInvocation(inv.Actor),
		record.SetStateRequest{IssueID: c.ID, Text: text},
	)
	return renderStateMutation(inv.Output, "Appended state on", result, err)
}

func renderStateMutation(output *Output, action string, result record.StateResult, err error) error {
	if err != nil {
		return err
	}
	if output.JSON() {
		return output.WriteJSON(result.Issue)
	}
	return output.Noticef("%s %s.", action, result.Issue.ID)
}

// StateReadOperations reads one issue's current recovery state.
type StateReadOperations interface {
	GetState(context.Context, record.GetStateRequest) (record.GetStateResult, error)
}

var _ StateReadOperations = (*record.Recorder)(nil)

type stateShowCommand struct {
	ID string `arg:"" name:"id" help:"Issue whose mutable state will be shown."`
}

func (c *stateShowCommand) referencedIssueIDs() []string { return []string{c.ID} }

// Help describes reading only the current mutable state.
func (*stateShowCommand) Help() string {
	return "Show the current mutable recovery state without log history."
}

// Run reads and renders one issue state.
func (c *stateShowCommand) Run(inv *Invocation, operations StateReadOperations) error {
	result, err := operations.GetState(inv.Context, record.GetStateRequest{IssueID: c.ID})
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(newStateOutput(result.IssueID, result.State))
	}
	if result.State == nil {
		return nil
	}
	var output strings.Builder
	writeMarkdown(&output, result.State.Body)
	if result.State.NextAction != "" {
		output.WriteString("\n**Next action**\n\n")
		writeMarkdown(&output, result.State.NextAction)
	}
	return inv.Output.WriteString(output.String())
}

// StateCommitOperations atomically commits current State and selects its
// replacement.
type StateCommitOperations interface {
	CommitState(context.Context, issue.Invocation, record.CommitStateRequest) (record.CommitStateResult, error)
}

var _ StateCommitOperations = (*record.Recorder)(nil)

type stateCommitCommand struct {
	ID   string  `arg:"" name:"id" help:"Issue whose current State will be committed."`
	Set  *string `name:"set" placeholder:"MARKDOWN" help:"State body after the commit. Use - to read standard input."`
	Next *string `name:"next" placeholder:"ACTION" help:"Optional next-action Markdown for a non-empty --set. Use - to read standard input."`
}

func (c *stateCommitCommand) referencedIssueIDs() []string { return []string{c.ID} }

// Help describes the atomic State disposition selected by each flag.
func (*stateCommitCommand) Help() string {
	return "Commit changed State to the Log. Clear current State by default or install its next value with --set."
}

// Run selects Markdown and delegates the atomic State commit.
func (c *stateCommitCommand) Run(
	inv *Invocation,
	markdown *MarkdownInput,
	operations StateCommitOperations,
) error {
	request := record.CommitStateRequest{
		IssueID:     c.ID,
		Disposition: record.CommitStateClear,
	}
	if c.Next != nil && c.Set == nil {
		return UsageErrorf("--next requires a non-empty --set")
	}
	inputs := []*string{}
	if c.Set != nil {
		inputs = append(inputs, c.Set)
	}
	if c.Next != nil {
		inputs = append(inputs, c.Next)
	}
	if err := markdown.ValidateSingleStdinConsumer(inputs...); err != nil {
		return err
	}
	if c.Set != nil {
		body, _, err := markdown.Read(c.Set)
		if err != nil {
			return err
		}
		if body == "" {
			if c.Next != nil {
				return UsageErrorf("--next requires a non-empty --set")
			}
			request.Disposition = record.CommitStateClear
		} else {
			if strings.TrimSpace(body) == "" {
				return UsageErrorf("state body must not be blank")
			}
			nextAction := ""
			if c.Next != nil {
				nextAction, _, err = markdown.Read(c.Next)
				if err != nil {
					return err
				}
				if strings.TrimSpace(nextAction) == "" {
					return UsageErrorf("next action must not be blank")
				}
			}
			request.Disposition = record.CommitStateReplace
			request.Replacement = record.StateReplacement{
				Body: body, NextAction: nextAction,
			}
		}
	}

	result, err := operations.CommitState(
		inv.Context,
		issue.NewInvocation(inv.Actor),
		request,
	)
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(newStateCommitOutput(result))
	}
	if result.LogEntry == nil {
		return inv.Output.Noticef(
			"Committed state on %s; no new snapshot.",
			result.Issue.ID,
		)
	}
	return inv.Output.Noticef(
		"Committed state on %s as %s.",
		result.Issue.ID,
		result.LogEntry.ID,
	)
}

type resultCommand struct {
	Set  resultSetCommand  `cmd:"" help:"Set the issue result."`
	Show resultShowCommand `cmd:"" help:"Show the issue result."`
}

// Help distinguishes the durable result from state and log entries.
func (*resultCommand) Help() string {
	return "Set or show one nonblank durable issue outcome."
}

// ResultWriteOperations stores one issue result.
type ResultWriteOperations interface {
	SetResult(context.Context, issue.Invocation, record.SetResultRequest) (record.SetResultResult, error)
}

var _ ResultWriteOperations = (*record.Recorder)(nil)

type resultSetCommand struct {
	ID   string  `arg:"" name:"id" help:"Issue whose result will be set."`
	Body *string `arg:"" optional:"" name:"body" help:"Result Markdown. Use - or omit with piped input to read standard input."`
}

func (c *resultSetCommand) referencedIssueIDs() []string { return []string{c.ID} }

// Help describes durable result input.
func (*resultSetCommand) Help() string {
	return "Set a nonblank durable result without closing the issue."
}

// Run selects Markdown input and stores one result.
func (c *resultSetCommand) Run(inv *Invocation, markdown *MarkdownInput, operations ResultWriteOperations) error {
	body, provided, err := markdown.Read(c.Body)
	if err != nil {
		return err
	}
	if !provided {
		return UsageErrorf("result body is required as an argument or standard input")
	}
	result, err := operations.SetResult(
		inv.Context,
		issue.NewInvocation(inv.Actor),
		record.SetResultRequest{IssueID: c.ID, Body: body},
	)
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(resultOutput{IssueID: result.IssueID, Body: result.Body})
	}
	return inv.Output.Noticef("Set result on %s.", result.IssueID)
}

// ResultReadOperations reads one durable issue result.
type ResultReadOperations interface {
	GetResult(context.Context, record.GetResultRequest) (issue.Result, error)
}

var _ ResultReadOperations = (*record.Recorder)(nil)

type resultShowCommand struct {
	ID string `arg:"" name:"id" help:"Issue whose durable result will be shown."`
}

func (c *resultShowCommand) referencedIssueIDs() []string { return []string{c.ID} }

// Help describes finite result retrieval.
func (*resultShowCommand) Help() string {
	return "Show one durable issue result; fail when no result exists."
}

// Run reads and renders one result.
func (c *resultShowCommand) Run(inv *Invocation, operations ResultReadOperations) error {
	result, err := operations.GetResult(inv.Context, record.GetResultRequest{IssueID: c.ID})
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(newResultOutput(result))
	}
	var output strings.Builder
	writeMarkdown(&output, result.Body)
	return inv.Output.WriteString(output.String())
}

type resultOutput struct {
	IssueID string `json:"issue_id"`
	Title   string `json:"title,omitempty"`
	Body    string `json:"body"`
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

func newResultOutput(result issue.Result) resultOutput {
	return resultOutput{IssueID: result.IssueID, Title: result.Title, Body: result.Body}
}

type closeIssuesOutput struct {
	Issues                     []issueSummaryOutput `json:"issues"`
	ParentsWithoutOpenChildren []string             `json:"parents_without_open_children"`
}

func newCloseIssuesOutput(result execution.CloseIssuesResult) closeIssuesOutput {
	issues := make([]issueSummaryOutput, len(result.Issues))
	for i, summary := range result.Issues {
		issues[i] = newIssueSummaryOutput(summary)
	}
	return closeIssuesOutput{
		Issues:                     issues,
		ParentsWithoutOpenChildren: result.ParentsWithoutOpenChildren,
	}
}

type cancelIssuesOutput struct {
	Issues                     []issue.Issue `json:"issues"`
	Requested                  int           `json:"requested"`
	Dependents                 int           `json:"dependents"`
	ParentsWithoutOpenChildren []string      `json:"parents_without_open_children"`
}

type reopenedIssueOutput struct {
	Issue                   issueSummaryOutput  `json:"issue"`
	UnresolvedPrerequisites []issueStatusOutput `json:"unresolved_prerequisites"`
}

type reopenIssuesOutput struct {
	Issues []reopenedIssueOutput `json:"issues"`
}

func newReopenIssuesOutput(result execution.ReopenIssuesResult) reopenIssuesOutput {
	issues := make([]reopenedIssueOutput, len(result.Issues))
	for i, reopened := range result.Issues {
		prerequisites := make([]issueStatusOutput, len(reopened.UnresolvedPrerequisites))
		for j, prerequisite := range reopened.UnresolvedPrerequisites {
			prerequisites[j] = issueStatusOutput{ID: prerequisite.ID, Status: prerequisite.Status}
		}
		issues[i] = reopenedIssueOutput{
			Issue:                   newIssueSummaryOutput(reopened.Issue),
			UnresolvedPrerequisites: prerequisites,
		}
	}
	return reopenIssuesOutput{Issues: issues}
}

type issueStatusOutput struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type checkpointOutput struct {
	Decision                   issue.CheckpointDecisionView `json:"decision"`
	Issue                      *issue.Issue                 `json:"issue"`
	Cancelled                  []issue.Issue                `json:"cancelled"`
	ParentsWithoutOpenChildren []string                     `json:"parents_without_open_children"`
}

type stateOutput struct {
	IssueID            string       `json:"issue_id"`
	Body               *string      `json:"body"`
	NextAction         *string      `json:"next_action,omitempty"`
	Author             *string      `json:"author,omitempty"`
	Updated            *int64       `json:"updated,omitempty"`
	SnapshotLogEntryID *issue.LogID `json:"snapshot_log_entry_id,omitempty"`
}

func newStateOutput(issueID string, state *issue.RecoveryState) stateOutput {
	output := stateOutput{IssueID: issueID}
	if state == nil {
		return output
	}
	output.Body = &state.Body
	if state.NextAction != "" {
		output.NextAction = new(state.NextAction)
	}
	if state.Author != "" {
		author := state.Author.String()
		output.Author = &author
	}
	if state.UpdatedAt != nil {
		updated := state.UpdatedAt.Unix()
		output.Updated = &updated
	}
	output.SnapshotLogEntryID = state.SnapshotLogEntryID
	return output
}

type stateRecordOutput struct {
	Body               string       `json:"body"`
	NextAction         *string      `json:"next_action,omitempty"`
	Author             *string      `json:"author,omitempty"`
	Updated            *int64       `json:"updated,omitempty"`
	SnapshotLogEntryID *issue.LogID `json:"snapshot_log_entry_id,omitempty"`
}

func newStateRecordOutput(state *issue.RecoveryState) *stateRecordOutput {
	if state == nil {
		return nil
	}
	output := newStateOutput("", state)
	return &stateRecordOutput{
		Body: *output.Body, NextAction: output.NextAction,
		Author: output.Author, Updated: output.Updated,
		SnapshotLogEntryID: output.SnapshotLogEntryID,
	}
}

type stateCommitOutput struct {
	IssueID         string             `json:"issue_id"`
	SnapshotCreated bool               `json:"snapshot_created"`
	State           *stateRecordOutput `json:"state"`
	LogEntry        *logEntryOutput    `json:"log_entry,omitempty"`
}

func newStateCommitOutput(result record.CommitStateResult) stateCommitOutput {
	output := stateCommitOutput{
		IssueID: result.Issue.ID,
		State:   newStateRecordOutput(result.State),
	}
	if result.LogEntry != nil {
		entry := newLogEntryOutput(*result.LogEntry)
		output.SnapshotCreated = true
		output.LogEntry = &entry
	}
	return output
}

func formatIssueView(view issue.View) string {
	var output strings.Builder
	if view.Context != nil {
		if view.Context.Board.Description != nil {
			output.WriteString("Board context\n")
			writeMarkdown(&output, *view.Context.Board.Description)
			output.WriteByte('\n')
		}
		for _, ancestor := range view.Context.Ancestors {
			fmt.Fprintf(&output, "Ancestor %s: %s\n", ancestor.Issue.ID, ancestor.Issue.Title)
			if ancestor.Issue.Summary != nil {
				writeMarkdown(&output, *ancestor.Issue.Summary)
			}
			if ancestor.Issue.State != nil {
				output.WriteString("Current state\n")
				writeMarkdown(&output, *ancestor.Issue.State)
				if ancestor.Issue.NextAction != nil {
					output.WriteString("\n**Next action**\n\n")
					writeMarkdown(&output, *ancestor.Issue.NextAction)
				}
			}
			if ancestor.DetailsBytes > 0 {
				fmt.Fprintf(
					&output, "%d bytes of details available: %s\n",
					ancestor.DetailsBytes, ancestor.Issue.ID,
				)
			}
			fmt.Fprintf(&output, "Log entries: %d\n\n", ancestor.LogSummary.Count)
		}
		for _, result := range view.Context.DependencyResults {
			fmt.Fprintf(&output, "Completed prerequisite %s: %s\n", result.Issue.ID, result.Issue.Title)
			writeMarkdown(&output, result.Body)
			output.WriteByte('\n')
		}
		if len(view.Context.Pins) > 0 {
			output.WriteString("Pinned issues:\n")
			for _, pin := range view.Context.Pins {
				fmt.Fprintf(&output, "- %s: %s\n", pin.ID, pin.Title)
			}
			output.WriteByte('\n')
		}
	}

	current := view.Detail.Issue
	fmt.Fprintf(&output, "Issue %s\n", current.ID)
	fmt.Fprintf(&output, "Title: %s\n", current.Title)
	fmt.Fprintf(&output, "Type: %s\n", current.Type)
	fmt.Fprintf(&output, "Status: %s\n", current.Status)
	fmt.Fprintf(&output, "Priority: %d\n", current.Priority)
	if current.Assignee != nil {
		fmt.Fprintf(&output, "Assignee: %s\n", *current.Assignee)
	}
	if len(view.Detail.Labels) > 0 {
		fmt.Fprintf(&output, "Labels: %s\n", strings.Join(view.Detail.Labels, ", "))
	}
	if current.Summary != nil {
		output.WriteString("\nSummary\n")
		writeMarkdown(&output, *current.Summary)
	}
	if current.Details != nil {
		output.WriteString("\nDetails\n")
		writeMarkdown(&output, *current.Details)
	}
	if current.State != nil {
		output.WriteString("\nCurrent state\n")
		writeMarkdown(&output, *current.State)
		if current.NextAction != nil {
			output.WriteString("\n**Next action**\n\n")
			writeMarkdown(&output, *current.NextAction)
		}
	}
	if view.Detail.CurrentResult != nil {
		output.WriteString("\nResult\n")
		writeMarkdown(&output, view.Detail.CurrentResult.Body)
	}
	return output.String()
}

func writeMarkdown(output *strings.Builder, value string) {
	output.WriteString(value)
	if value == "" || value[len(value)-1] != '\n' {
		output.WriteByte('\n')
	}
}

func writeParentNotices(output *Output, parents []string) error {
	for _, parent := range parents {
		if err := output.Noticef("Parent %s has no open children.", parent); err != nil {
			return err
		}
	}
	return nil
}

func plural(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
