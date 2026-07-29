package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"buf.build/go/protovalidate"
	"github.com/alecthomas/kong"
	"google.golang.org/protobuf/encoding/protojson"

	"go.abhg.dev/cardamom/internal/errkind"
	cardamomv1 "go.abhg.dev/cardamom/internal/gen/cardamom/v1"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
)

// labelTerms normalizes CLI signed label terms into unsigned additions and
// removals. Signed input is consumed here so application requests never carry
// the command grammar.
type labelTerms struct {
	add    []string
	remove []string
}

// Decode accepts one comma-separated label term flag.
//
// A term with no prefix or a leading plus is an addition. A leading minus is a
// removal. Repeated flags append to the same normalized edit.
func (l *labelTerms) Decode(ctx *kong.DecodeContext) error {
	return l.decode(ctx, "label")
}

func (l *labelTerms) decode(ctx *kong.DecodeContext, kind string) error {
	token := ctx.Scan.Pop()
	if token.IsEOL() {
		return fmt.Errorf(`missing value, expecting "<%s>"`, kind)
	}
	if token.InferredType() == kong.FlagToken {
		return fmt.Errorf(
			"expected %s value but got %q (%s)",
			kind,
			token.String(),
			token.InferredType(),
		)
	}
	value, ok := token.Value.(string)
	if !ok {
		return fmt.Errorf("expected %s value but got %q", kind, token.Value)
	}
	for _, term := range kong.SplitEscaped(value, ctx.Value.Tag.Sep) {
		if label, ok := strings.CutPrefix(term, "-"); ok {
			l.remove = append(l.remove, label)
			continue
		}
		l.add = append(l.add, strings.TrimPrefix(term, "+"))
	}
	return nil
}

// dependencyTerms maps signed CLI terms into dependency additions and removals.
type dependencyTerms labelTerms

func (d *dependencyTerms) Decode(ctx *kong.DecodeContext) error {
	return (*labelTerms)(d).decode(ctx, "dependency")
}

type createCommand struct {
	Title     string     `arg:"" name:"title" help:"Title of the issue."`
	Type      string     `name:"type" default:"task" enum:"workstream,task,checkpoint,routine" placeholder:"TYPE" help:"Issue type. Defaults to task."`
	Priority  int        `name:"priority" default:"2" placeholder:"PRIORITY" help:"Priority from 0 (highest) through 4 (lowest). Defaults to 2."`
	Labels    labelTerms `name:"label" placeholder:"TERM" help:"Label term to attach. No prefix or + adds; - is invalid. Repeat for multiple labels."`
	DependsOn []string   `name:"depends-on" placeholder:"ISSUE" help:"Prerequisite issue ID. Repeat for multiple prerequisites."`
	Parent    string     `name:"parent" placeholder:"ISSUE" help:"Containment parent issue ID."`
	Summary   string     `name:"summary" placeholder:"MARKDOWN" help:"Concise Markdown issue summary."`
	Details   string     `name:"details" placeholder:"MARKDOWN" help:"Expanded Markdown issue details."`
}

func (*createCommand) Help() string {
	return `Create one issue and its labels, prerequisites, and containment parent atomically.

Issue types are workstream, task, checkpoint, and routine.`
}

// CreateIssueOperation creates one issue with its initial relationships.
type CreateIssueOperation interface {
	// CreateIssue creates the requested issue atomically.
	CreateIssue(context.Context, issue.Invocation, planning.CreateIssueRequest) (planning.CreateIssueResult, error)
}

func (c *createCommand) Run(inv *Invocation, operation CreateIssueOperation) error {
	if len(c.Labels.remove) != 0 {
		return UsageErrorf("create --label does not accept removal terms")
	}
	result, err := operation.CreateIssue(inv.Context, issue.NewInvocation(inv.Actor), planning.CreateIssueRequest{
		Title: c.Title, Type: c.Type, Priority: c.Priority,
		Labels: c.Labels.add, DependsOn: c.DependsOn, Parent: c.Parent,
		Summary: c.Summary, Details: c.Details,
	})
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(newIssueDetailOutput(result.Issue))
	}
	return inv.Output.WriteString(result.Issue.Issue.ID + "\n")
}

type applyCommand struct {
	File   string `arg:"" name:"file" optional:"" type:"path" help:"Issue graph JSON file. Use - or omit with piped input to read standard input."`
	DryRun bool   `name:"dry-run" help:"Validate and plan without allocating durable state."`
}

func (*applyCommand) Help() string {
	return `Apply one version 1 issue graph atomically.

Input is a strict JSON object with version, issues, and optional on_existing.
Unknown fields are rejected.

Top-level fields:
  version: required integer; must be 1.
  issues: required non-empty array of issue objects.
  on_existing: optional string: error, skip, or update; defaults to error.

error rejects existing targets, skip preserves them, and update reconciles
present fields.

Issue object fields:
  alias: optional local reference token; trimmed, unique, and has no whitespace.
  id: optional existing issue ID matching [A-Za-z0-9][A-Za-z0-9-]*.
  key: optional non-empty exact producer string; unique in the document and
       scoped to the selected board.
  title: optional string; trimmed and nonblank when present.
  type: optional string: workstream, task, checkpoint, or routine.
  priority: optional integer from 0 through 4.
  summary: optional string; the configured summary byte limit applies.
  details: optional string.
  labels: optional {"values":[STRING, ...]}; replaces every label.
  parent: optional issue reference; replaces containment.
  clear_parent: optional {}; removes containment and cannot appear with parent.
  depends_on: optional {"values":[REFERENCE, ...]}; replaces all prerequisites.

Label strings are trimmed, nonempty, and cannot start with + or -.

An issue reference contains exactly one string field:
  {"alias":"TOKEN"} selects an entry's document-local alias.
  {"id":"ISSUE"} selects a durable issue in the selected board.
  {"key":"STRING"} selects an exact board-scoped producer key.

New issues require title and type. Omitted priority defaults to 2; omitted
labels and depends_on are empty, and an omitted parent leaves no containment.

With on_existing update, present editable fields replace current values.

Omitted fields are preserved. Empty summary or details strings clear those
fields.

Empty labels.values and depends_on.values arrays clear those sets.
parent replaces containment; clear_parent removes it; omitting both preserves
containment. Updates require an open, unclaimed issue.`
}

// ApplyDocumentOperation validates or persists one canonical apply document.
type ApplyDocumentOperation interface {
	// ApplyDocument executes one shared dry-run or commit operation.
	ApplyDocument(context.Context, issue.Invocation, planning.ApplyDocumentRequest) (planning.ApplyReceipt, error)
}

func (c *applyCommand) Run(inv *Invocation, operation ApplyDocumentOperation) error {
	document, err := readApplyDocument(inv, c.File)
	if err != nil {
		return err
	}
	request, err := applyDocumentRequest(document, c.DryRun)
	if err != nil {
		return err
	}
	result, err := operation.ApplyDocument(
		inv.Context,
		issue.NewInvocation(inv.Actor),
		request,
	)
	if err != nil {
		return err
	}
	return renderApplyReceipt(inv.Output, result)
}

func readApplyDocument(inv *Invocation, filename string) (*cardamomv1.ApplyDocument, error) {
	var body []byte
	var err error
	switch {
	case filename != "" && filename != "-":
		body, err = os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("read apply document %q: %w", filename, err)
		}
	case filename == "" && inv.StdinIsTerminal:
		return nil, UsageErrorf("apply requires a file or piped JSON input")
	default:
		if err := inv.Context.Err(); err != nil {
			return nil, err
		}
		body, err = io.ReadAll(inv.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read apply document from standard input: %w", err)
		}
		if err := inv.Context.Err(); err != nil {
			return nil, err
		}
	}

	if len(body) == 0 {
		return nil, UsageErrorf("apply document is empty")
	}
	var document cardamomv1.ApplyDocument
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(body, &document); err != nil {
		return nil, UsageErrorf("decode apply document: %v", err)
	}
	if err := protovalidate.Validate(&document); err != nil {
		return nil, UsageErrorf("validate apply document: %v", err)
	}
	return &document, nil
}

func renderApplyReceipt(output *Output, result planning.ApplyReceipt) error {
	message := applyReceiptMessage(result)
	if err := protovalidate.Validate(message); err != nil {
		return fmt.Errorf("validate apply receipt: %w", err)
	}
	if output.JSON() {
		body, err := (protojson.MarshalOptions{}).Marshal(message)
		if err != nil {
			return fmt.Errorf("encode apply receipt: %w", err)
		}
		return output.WriteString(string(body) + "\n")
	}
	verb := "applied"
	if result.DryRun {
		verb = "planned"
	}
	if err := output.Noticef(
		"%s document: %d create, %d update, %d skip, %d no change",
		verb,
		result.Counts.Create,
		result.Counts.Update,
		result.Counts.Skip,
		result.Counts.NoChange,
	); err != nil {
		return err
	}
	for _, entry := range result.Entries {
		identity := "-"
		switch {
		case entry.ID != nil:
			identity = *entry.ID
		case entry.Key != nil:
			identity = *entry.Key
		case entry.Alias != nil:
			identity = *entry.Alias
		}
		action := entry.Action.String()
		if action == "" {
			action = "unknown"
		}
		if err := output.WriteString(fmt.Sprintf(
			"%d\t%s\t%s\n",
			entry.InputIndex,
			action,
			identity,
		)); err != nil {
			return err
		}
	}
	return nil
}

func applyDocumentRequest(
	value *cardamomv1.ApplyDocument,
	dryRun bool,
) (planning.ApplyDocumentRequest, error) {
	existing, err := planning.NewApplyExistingPolicy(value.GetOnExisting())
	if err != nil {
		return planning.ApplyDocumentRequest{}, UsageErrorf("%v", err)
	}
	request := planning.ApplyDocumentRequest{
		Version:    int(value.GetVersion()),
		Issues:     make([]planning.ApplyIssue, 0, len(value.GetIssues())),
		OnExisting: existing,
		Mode:       planning.ApplyModeCommit,
	}
	if dryRun {
		request.Mode = planning.ApplyModeDryRun
	}
	for index, entry := range value.GetIssues() {
		converted, err := applyIssueRequest(entry)
		if err != nil {
			return planning.ApplyDocumentRequest{}, fmt.Errorf("issue %d: %w", index+1, err)
		}
		request.Issues = append(request.Issues, converted)
	}
	return request, nil
}

func applyIssueRequest(value *cardamomv1.ApplyIssue) (planning.ApplyIssue, error) {
	if value == nil {
		return planning.ApplyIssue{}, errkind.Errorf(errkind.InvalidInput, "invalid input: issue required")
	}
	request := planning.ApplyIssue{
		Alias: value.Alias, ID: value.Id, Key: value.Key,
		Title: value.Title, Summary: value.Summary, Details: value.Details,
	}
	if value.Type != nil {
		kind, err := issue.NewKind(value.GetType())
		if err != nil {
			return planning.ApplyIssue{}, UsageErrorf("%v", err)
		}
		converted := kind.String()
		request.Type = &converted
	}
	if value.Priority != nil {
		priority := int(value.GetPriority())
		request.Priority = &priority
	}
	if value.Labels != nil {
		labels := value.Labels.GetValues()
		request.Labels = &labels
	}
	switch parent := value.GetParentChange().(type) {
	case nil:
	case *cardamomv1.ApplyIssue_Parent:
		converted, err := applyIssueReference(parent.Parent)
		if err != nil {
			return planning.ApplyIssue{}, err
		}
		request.Parent = planning.ApplyParentChange{
			Kind: planning.ParentReplace, Reference: converted,
		}
	case *cardamomv1.ApplyIssue_ClearParent:
		if parent.ClearParent == nil {
			return planning.ApplyIssue{}, errkind.Errorf(
				errkind.InvalidInput,
				"invalid input: clear_parent value required",
			)
		}
		request.Parent = planning.ApplyParentChange{Kind: planning.ParentClear}
	default:
		return planning.ApplyIssue{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: unknown parent change",
		)
	}
	if value.DependsOn != nil {
		dependencies := make([]planning.ApplyIssueReference, 0, len(value.DependsOn.GetValues()))
		for _, reference := range value.DependsOn.GetValues() {
			converted, err := applyIssueReference(reference)
			if err != nil {
				return planning.ApplyIssue{}, err
			}
			dependencies = append(dependencies, converted)
		}
		request.DependsOn = &dependencies
	}
	return request, nil
}

func applyIssueReference(value *cardamomv1.IssueReference) (planning.ApplyIssueReference, error) {
	if value == nil {
		return planning.ApplyIssueReference{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: issue reference required",
		)
	}
	switch target := value.GetTarget().(type) {
	case *cardamomv1.IssueReference_Alias:
		return planning.ApplyIssueReference{
			Kind: planning.ApplyReferenceAlias, Alias: target.Alias,
		}, nil
	case *cardamomv1.IssueReference_Id:
		return planning.ApplyIssueReference{
			Kind: planning.ApplyReferenceID, ID: target.Id,
		}, nil
	case *cardamomv1.IssueReference_Key:
		return planning.ApplyIssueReference{
			Kind: planning.ApplyReferenceKey, Key: target.Key,
		}, nil
	default:
		return planning.ApplyIssueReference{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: issue reference target required",
		)
	}
}

func applyReceiptMessage(value planning.ApplyReceipt) *cardamomv1.ApplyReceipt {
	entries := make([]*cardamomv1.ApplyReceiptEntry, len(value.Entries))
	for index, entry := range value.Entries {
		entries[index] = &cardamomv1.ApplyReceiptEntry{
			InputIndex: uint32(entry.InputIndex), Alias: entry.Alias, Id: entry.ID,
			Key: entry.Key, Action: entry.Action.String(),
		}
	}
	return &cardamomv1.ApplyReceipt{
		Entries: entries,
		Counts: &cardamomv1.ApplyCounts{
			Create: uint32(value.Counts.Create), Update: uint32(value.Counts.Update),
			Skip: uint32(value.Counts.Skip), NoChange: uint32(value.Counts.NoChange),
		},
		Revision: value.Revision, DryRun: value.DryRun,
	}
}

type issueEditCommand struct {
	ID           string          `arg:"" name:"id" predictor:"issues" help:"Issue ID."`
	Title        *string         `name:"title" placeholder:"TITLE" help:"Replacement title."`
	Type         *string         `name:"type" enum:"workstream,task,routine" placeholder:"TYPE" help:"Replacement executable issue type."`
	Priority     *int            `name:"priority" placeholder:"PRIORITY" help:"Replacement priority from 0 through 4."`
	Summary      *string         `name:"summary" placeholder:"MARKDOWN" help:"Replacement Markdown summary. Use an empty value to clear it."`
	Details      *string         `name:"details" placeholder:"MARKDOWN" help:"Replacement Markdown details. Use an empty value to clear it."`
	Parent       *string         `name:"parent" placeholder:"ISSUE" help:"Replacement containment parent issue ID. Use an empty value to clear it."`
	Dependencies dependencyTerms `name:"depends-on" placeholder:"TERM" help:"Prerequisite term. No prefix or + adds; - removes. Repeat for multiple issues."`
	Labels       labelTerms      `name:"label" placeholder:"TERM" help:"Label term. No prefix or + adds; - removes. Repeat for multiple labels."`
}

func (c *issueEditCommand) referencedIssueIDs() []string { return []string{c.ID} }

func (*issueEditCommand) Help() string {
	return `Atomically edit requested scalar fields, labels, prerequisites, and containment.

Editable issue types are workstream, task, and routine.`
}

// EditIssueOperation changes issue metadata and relationships atomically.
type EditIssueOperation interface {
	// EditIssue applies the requested issue changes.
	EditIssue(context.Context, issue.Invocation, planning.EditIssueRequest) (planning.EditIssueResult, error)
}

func (c *issueEditCommand) Run(inv *Invocation, operation EditIssueOperation) error {
	parent := c.Parent
	if parent != nil && *parent == "" {
		parent = nil
	}
	result, err := operation.EditIssue(inv.Context, issue.NewInvocation(inv.Actor), planning.EditIssueRequest{
		ID: c.ID, Title: c.Title, Type: c.Type, Priority: c.Priority,
		Summary: c.Summary, SummarySet: c.Summary != nil,
		Details: c.Details, DetailsSet: c.Details != nil,
		Parent: parent, ParentSet: c.Parent != nil,
		AddDependencies: c.Dependencies.add, RemoveDependencies: c.Dependencies.remove,
		AddLabels: c.Labels.add, RemoveLabels: c.Labels.remove,
	})
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(newIssueDetailOutput(result.Issue))
	}
	return inv.Output.Noticef("edited %s", result.Issue.Issue.ID)
}
