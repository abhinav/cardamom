package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/board/selection"
	"go.abhg.dev/cardamom/internal/project"
	projectcreation "go.abhg.dev/cardamom/internal/project/creation"
	"go.abhg.dev/komplete"
)

type initCommand struct {
	Path      string  `arg:"" optional:"" name:"path" help:"Project directory."`
	Prefix    *string `name:"prefix" placeholder:"PREFIX" help:"Project issue ID prefix ending in a dash."`
	BoardName *string `name:"board-name" placeholder:"NAME" help:"Name for the first board."`
	NoBoard   bool    `name:"no-board" help:"Initialize the project without a first board."`
	NoConfig  bool    `name:"no-config" help:"Do not create a missing config.yaml."`
}

// Help describes initialization selection and repeat behavior.
func (*initCommand) Help() string {
	return `Initialize a physical Cardamom store and project namespace.

Without --store, card creates <path>/.cardamom, where <path> defaults to the current
working directory.

With --store, <path> is ignored. The current working directory supplies the
project name and inferred first-board name.

The first board name defaults to the basename of <path> without --store.
Use --board-name to override it or --no-board to skip board creation.

For a fresh project, omit --prefix to inherit an active store prefix or infer a
project prefix from the selected directory basename. Inference lowercases and
normalizes the basename within the 16-character prefix limit, with cm- as the
fallback.

An explicit --prefix establishes a project-level override for a fresh or
retained project.

Use --no-config to leave a missing config.yaml absent. An existing config.yaml
remains active.

Repeated initialization preserves existing projects and boards; first-init
board options never add or rename them, and omitted --prefix never infers a new
project prefix. Use card project create NAME to add another project.

Set CARDAMOM_NO_GITIGNORE to any non-empty value to prevent initialization from
changing Git's local exclude file.`
}

// InitConfigMode selects how initialization handles a missing store
// configuration file.
type InitConfigMode uint8

const (
	// InitConfigWriteMissing publishes the default template or requested
	// overrides when config.yaml is missing.
	InitConfigWriteMissing InitConfigMode = iota

	// InitConfigSkipMissing leaves a missing config.yaml absent.
	InitConfigSkipMissing
)

// InitRequest carries parsed initialization choices to process composition.
// Process composition owns filesystem discovery, configuration, and store
// publication.
type InitRequest struct {
	// Store is the explicit or environment-selected store. Empty requests the
	// normal initialization target derived from ProjectPath.
	Store string

	// ProjectPath is the optional project directory argument.
	ProjectPath string

	// IDPrefix is the explicitly requested project issue prefix. Nil lets
	// initialization apply store inheritance or basename inference.
	IDPrefix *string

	// BoardName overrides first-board name inference when non-nil.
	BoardName *string

	// NoBoard suppresses first-board creation.
	NoBoard bool

	// ConfigMode selects how a missing config.yaml is handled.
	ConfigMode InitConfigMode
}

// InitIgnoreOutcome records how initialization handled checkout-local ignore
// configuration so the CLI can render the corresponding notice.
type InitIgnoreOutcome uint8

const (
	// InitIgnoreUnchanged emits no ignore notice.
	InitIgnoreUnchanged InitIgnoreOutcome = iota

	// InitIgnoreStoragePatternsAdded reports database and blob exclusions.
	InitIgnoreStoragePatternsAdded

	// InitIgnoreManual reports that automatic Git exclusion was unavailable.
	InitIgnoreManual
)

// InitResult is the complete public result of one initialization invocation.
type InitResult struct {
	// Dir is the initialized physical store directory.
	Dir string `json:"dir"`

	// IDPrefix and IDStrategy are the effective project configuration.
	IDPrefix   string `json:"id_prefix"`
	IDStrategy string `json:"id_strategy"`

	// SchemaVersion is the store schema reached by this invocation.
	SchemaVersion int `json:"schema_version"`

	// ConfigWritten and DatabaseWritten identify artifacts created by this
	// invocation.
	ConfigWritten   bool `json:"config_written"`
	DatabaseWritten bool `json:"db_written"`

	// ProjectCreated reports whether this invocation established ProjectID and
	// ProjectName.
	ProjectCreated bool    `json:"project_created"`
	ProjectID      *string `json:"project_id"`
	ProjectName    *string `json:"project_name"`

	// BoardCreated reports whether this invocation established BoardID and
	// BoardName.
	BoardCreated bool    `json:"board_created"`
	BoardID      *string `json:"board_id"`
	BoardName    *string `json:"board_name"`

	// AlreadyInitialized reports that existing namespace state was retained.
	AlreadyInitialized bool `json:"already_initialized"`

	// IgnoreOutcome selects the human checkout-exclusion notice.
	IgnoreOutcome InitIgnoreOutcome `json:"-"`
}

// InitOperation owns initialization beyond command parsing and rendering.
type InitOperation interface {
	Initialize(context.Context, InitRequest) (InitResult, error)
}

// Run validates initialization syntax, delegates the operation, and renders
// its public result.
func (c *initCommand) Run(invocation *Invocation, operation InitOperation) error {
	if c.BoardName != nil && c.NoBoard {
		return UsageErrorf("init: --board-name and --no-board cannot be combined")
	}
	if c.BoardName != nil && strings.TrimSpace(*c.BoardName) == "" {
		return UsageErrorf("init: --board-name: board name required")
	}
	configMode := InitConfigWriteMissing
	if c.NoConfig {
		configMode = InitConfigSkipMissing
	}

	result, err := operation.Initialize(invocation.Context, InitRequest{
		Store: invocation.Store, ProjectPath: c.Path, IDPrefix: c.Prefix,
		BoardName: c.BoardName, NoBoard: c.NoBoard, ConfigMode: configMode,
	})
	if err != nil {
		return err
	}
	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(result)
	}
	return writeInitResult(invocation.Output, result)
}

// writeInitResult writes human initialization status and ignore guidance.
// Output owns standard-output delivery and quiet or JSON suppression.
func writeInitResult(output *Output, result InitResult) error {
	status := "initialized " + result.Dir
	if result.AlreadyInitialized {
		status = fmt.Sprintf(
			"already initialized: %s; kept existing projects and boards",
			result.Dir,
		)
	} else if !result.BoardCreated {
		status += " without a board"
	}
	if err := output.Noticef("%s", status); err != nil {
		return err
	}
	if result.AlreadyInitialized {
		if err := output.Noticef(
			"Use card project create NAME to add another project.",
		); err != nil {
			return err
		}
	}

	switch result.IgnoreOutcome {
	case InitIgnoreUnchanged:
		return nil
	case InitIgnoreStoragePatternsAdded:
		return output.Noticef(
			"This checkout now excludes Cardamom's local database and attachment blob files. " +
				".cardamom/config.yaml remains suitable to commit.",
		)
	case InitIgnoreManual:
		return output.Noticef(`Automatic local exclusion failed. Add these patterns to .gitignore:
.cardamom/board.sqlite3
.cardamom/board.sqlite3-shm
.cardamom/board.sqlite3-wal
.cardamom/blobs/`)
	default:
		return fmt.Errorf("unsupported initialization ignore outcome %d", result.IgnoreOutcome)
	}
}

type projectCommand struct {
	List   projectListCommand   `cmd:"" help:"List project namespaces."`
	Create projectCreateCommand `cmd:"" help:"Create a project namespace."`
}

// Help explains project ownership inside the selected physical store.
func (*projectCommand) Help() string {
	return `A project is a repository or product namespace inside a Cardamom store.

Project operations use the selected physical store without selecting or creating
a board.`
}

type projectListCommand struct{}

// Run lists every project without requiring a board selection.
func (*projectListCommand) Run(
	invocation *Invocation,
	projects *project.Service,
) error {
	states, err := projects.List(invocation.Context)
	if err != nil {
		return err
	}
	records := make([]projectSummaryOut, 0, len(states))
	for _, state := range states {
		records = append(records, newProjectSummaryOut(state))
	}
	if invocation.Output.JSON() {
		return WriteJSONLines(invocation.Output, records)
	}
	for _, record := range records {
		if err := invocation.Output.WriteString(
			fmt.Sprintf("%s\t%s\n", record.ID, record.Name),
		); err != nil {
			return err
		}
	}
	return nil
}

type projectCreateCommand struct {
	Name   string  `arg:"" name:"name" help:"Name of the new project."`
	Prefix *string `name:"prefix" placeholder:"PREFIX" help:"Project issue ID prefix ending in a dash."`
}

// Help explains project naming, prefix selection, and board independence.
func (*projectCreateCommand) Help() string {
	return `Create one project in the selected physical store.

NAME is trimmed and must contain a non-whitespace character. Project names need
not be unique; duplicate exact names make later name selection ambiguous, so use
the project ID when selecting one.

An explicit --prefix establishes a project-level override. Without --prefix,
the new project inherits an active store prefix or persists a prefix inferred
from NAME.

Project creation does not create or select a board.`
}

// Run delegates project creation and renders the created project.
func (c *projectCreateCommand) Run(
	invocation *Invocation,
	creator *projectcreation.Service,
) error {
	created, err := creator.CreateProject(
		invocation.Context,
		projectcreation.NewInvocation(invocation.Actor),
		projectcreation.Request{Name: c.Name, Prefix: c.Prefix},
	)
	if err != nil {
		return err
	}
	record := newProjectSummaryOut(created)
	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(record)
	}
	return invocation.Output.WriteString(
		fmt.Sprintf("created project %s (%s)\n", record.ID, record.Name),
	)
}

// projectSummaryOut is the structured command projection for one project.
type projectSummaryOut struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Created int64  `json:"created"`
}

func newProjectSummaryOut(state *project.State) projectSummaryOut {
	return projectSummaryOut{
		ID: state.ID().String(), Name: state.Name(), Created: state.Created().Unix(),
	}
}

type boardCommand struct {
	List   boardListCommand   `cmd:"" help:"List coordination boards."`
	Create boardCreateCommand `cmd:"" help:"Create a coordination board."`
	Copy   boardCopyCommand   `cmd:"" help:"Copy a board into another store."`
	Use    boardUseCommand    `cmd:"" help:"Select a board for this checkout."`
	Show   boardShowCommand   `cmd:"" help:"Show board metadata."`
	Edit   boardEditCommand   `cmd:"" help:"Edit board metadata."`
}

// Help explains the independent project, board, and store namespaces.
func (*boardCommand) Help() string {
	return `A project is a repository or product namespace.

A board is an explicitly created shared coordination context within a project.

Board operations work without changing the physical Cardamom store.`
}

type boardListCommand struct{}

// BoardCatalog supplies board reads used by commands that do not select one board.
type BoardCatalog interface {
	List(context.Context) ([]*board.State, error)
}

// Run lists every board without requiring one board to be selected.
func (*boardListCommand) Run(invocation *Invocation, catalog BoardCatalog) error {
	boards, err := catalog.List(invocation.Context)
	if err != nil {
		return err
	}
	records := make([]boardSummaryOut, 0, len(boards))
	for _, board := range boards {
		records = append(records, newBoardSummaryOut(board))
	}
	if invocation.Output.JSON() {
		return WriteJSONLines(invocation.Output, records)
	}
	for _, record := range records {
		line := fmt.Sprintf("%s\t%s\n", record.ID, record.Name)
		if err := invocation.Output.WriteString(line); err != nil {
			return err
		}
	}
	return nil
}

type boardCreateCommand struct {
	Name    string `arg:"" name:"name" help:"Name of the new board."`
	Project string `name:"project" predictor:"projects" placeholder:"PROJECT" help:"Project ID or exact name. When omitted, use the store's sole project."`
}

// Help explains board ownership and the persisted name contract.
func (*boardCreateCommand) Help() string {
	return `Create one board in a project.

A project is a repository or product namespace.
A board is a non-nested shared coordination context within one project.

NAME is trimmed and must contain a non-whitespace character.
Board names need not be unique; duplicate exact names make name selection
ambiguous, so use the board ID when selecting one.`
}

// Run resolves the containing project and creates one board.
func (c *boardCreateCommand) Run(
	invocation *Invocation,
	projects *project.Service,
	boards *board.Service,
) error {
	var selector *project.Selector
	if c.Project != "" {
		parsed, err := project.NewSelector(c.Project)
		if err != nil {
			return err
		}
		selector = &parsed
	}
	namespace, err := projects.Resolve(invocation.Context, selector)
	if err != nil {
		return err
	}
	created, err := boards.Create(invocation.Context, board.NewInvocation(invocation.Actor), board.CreateRequest{
		ProjectID: namespace.ID().String(), Name: c.Name,
	})
	if err != nil {
		return err
	}
	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(newBoardSummaryOut(created))
	}
	return invocation.Output.WriteString(created.ID().String() + "\n")
}

type boardUseCommand struct {
	Selector string `arg:"" name:"board" predictor:"boards" help:"Board ID or exact name."`
}

// Help explains that checkout selection does not alter store discovery.
func (*boardUseCommand) Help() string {
	return `Persist the selected board for this checkout. This changes which board
ambient board-scoped commands use; it does not change which Cardamom store is selected.`
}

// Run resolves and persists one checkout-local board selection.
func (c *boardUseCommand) Run(invocation *Invocation, resolver *selection.Resolver) error {
	selector, err := board.NewSelector(c.Selector)
	if err != nil {
		return err
	}
	board, err := resolver.Use(invocation.Context, selector)
	if err != nil {
		return err
	}
	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(newBoardSummaryOut(board))
	}
	return invocation.Output.Noticef("using %s (%s)", board.ID(), board.Name())
}

type boardShowCommand struct {
	Selector string `arg:"" optional:"" name:"board" predictor:"boards" help:"Board ID or exact name. When omitted, use the root board selection, checkout selection, or store's sole board."`
}

// Run resolves and renders one board without changing checkout selection.
func (c *boardShowCommand) Run(invocation *Invocation, resolver *selection.Resolver) error {
	board, err := resolveBoard(invocation, c.Selector, resolver)
	if err != nil {
		return err
	}
	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(newBoardDetailOut(board))
	}
	description := "(none)"
	if value := board.Description(); value != nil {
		description = *value
	}
	return invocation.Output.WriteString(fmt.Sprintf(
		"ID:      %s\nProject: %s\nName:    %s\nCreated: %s\nDescription: %s\n",
		board.ID(), board.ProjectID(), board.Name(),
		board.Created().Format(time.RFC3339), description,
	))
}

type boardEditCommand struct {
	Selector    string  `arg:"" optional:"" name:"board" predictor:"boards" help:"Board ID or exact name. When omitted, use the root board selection, checkout selection, or store's sole board."`
	Name        *string `name:"name" placeholder:"NAME" help:"Replace the board name."`
	Description *string `name:"description" placeholder:"MARKDOWN" help:"Replace the board description with Markdown. An empty value clears it. Use - to read standard input."`
}

// Help describes atomic board settings.
func (*boardEditCommand) Help() string {
	return `Edit one board's name, Markdown description, or both.
Name and description settings change atomically. An empty --description value
clears the board description.`
}

// Run parses one board settings edit and delegates the atomic mutation.
func (c *boardEditCommand) Run(
	invocation *Invocation,
	boards *board.Service,
	resolver *selection.Resolver,
	markdown *MarkdownInput,
) error {
	if c.Name == nil && c.Description == nil {
		return UsageErrorf("board edit: at least one setting is required")
	}

	selected, err := resolveBoard(invocation, c.Selector, resolver)
	if err != nil {
		return err
	}
	settings := board.SettingsEdit{Name: c.Name}
	if c.Description != nil {
		description, _, err := markdown.Read(c.Description)
		if err != nil {
			return err
		}
		var replacement *string
		if description != "" {
			replacement = &description
		}
		settings.Description = board.ReplaceDescription(replacement)
	}
	edited, err := boards.EditSettings(invocation.Context, board.NewInvocation(invocation.Actor), board.EditRequest{
		BoardID: selected.ID(), Settings: settings,
	})
	if err != nil {
		return err
	}
	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(newBoardDetailOut(edited))
	}

	message := "updated board settings"
	if c.Name == nil {
		message = "updated board description"
	} else if c.Description == nil {
		message = "updated board name"
	}
	return invocation.Output.Noticef("%s", message)
}

func resolveBoard(
	invocation *Invocation,
	commandSelector string,
	resolver *selection.Resolver,
) (*board.State, error) {
	selectorValue := commandSelector
	if selectorValue == "" {
		selectorValue = invocation.Board
	}
	var selector *board.Selector
	if selectorValue != "" {
		parsed, err := board.NewSelector(selectorValue)
		if err != nil {
			return nil, err
		}
		selector = &parsed
	}
	return resolver.Resolve(invocation.Context, selection.Request{
		Selector: selector,
	})
}

type boardSummaryOut struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Created   int64  `json:"created"`
}

func newBoardSummaryOut(board *board.State) boardSummaryOut {
	return boardSummaryOut{
		ID: board.ID().String(), ProjectID: board.ProjectID(),
		Name: board.Name(), Created: board.Created().Unix(),
	}
}

type boardDetailOut struct {
	boardSummaryOut
	Description *string `json:"description"`
}

func newBoardDetailOut(board *board.State) boardDetailOut {
	return boardDetailOut{
		boardSummaryOut: newBoardSummaryOut(board),
		Description:     board.Description(),
	}
}

type versionCommand struct {
	value string
}

// Run prints the process build version.
func (c *versionCommand) Run(invocation *Invocation) error {
	return invocation.Output.WriteString(c.value + "\n")
}

type infoCommand struct{}

// InfoRequest carries process-level store and board selectors to the
// information operation.
type InfoRequest struct {
	// Store is the explicit or environment-selected physical store.
	Store string

	// Board is the explicit or environment-selected board.
	Board string
}

// InfoResult is the typed store information document emitted by info.
type InfoResult struct {
	// Store identifies the selected physical persistence boundary.
	Store InfoStore `json:"store"`

	// Project identifies the project containing Board.
	Project InfoProject `json:"project"`

	// Board identifies the selected coordination board.
	Board InfoBoard `json:"board"`

	// Schema identifies persisted and running schema versions.
	Schema InfoSchema `json:"schema"`

	// Configuration is the selected board's effective configuration.
	Configuration InfoConfiguration `json:"configuration"`

	// Revision identifies the canonical store revision.
	Revision InfoRevision `json:"revision"`

	// Issues reports the selected board's issue population.
	Issues InfoIssueInventory `json:"issues"`
}

// InfoStore identifies the selected physical persistence boundary.
type InfoStore struct {
	// Directory is the selected Cardamom store directory.
	Directory string `json:"directory"`

	// DatabasePath is the SQLite database inside Directory.
	DatabasePath string `json:"database_path"`
}

// InfoProject identifies the selected project.
type InfoProject struct {
	// ID is the project's stable identity.
	ID string `json:"id"`

	// Name is the project's user-visible name.
	Name string `json:"name"`
}

// InfoBoard identifies the selected coordination board.
type InfoBoard struct {
	// ID is the board's stable identity.
	ID string `json:"id"`

	// ProjectID identifies the project containing the board.
	ProjectID string `json:"project_id"`

	// Name is the board's user-visible name.
	Name string `json:"name"`
}

// InfoSchema identifies persisted and running schema versions.
type InfoSchema struct {
	// DatabaseVersion is the latest migration recorded by the store.
	DatabaseVersion int64 `json:"database_version"`

	// CodeVersion is the latest migration understood by the running program.
	CodeVersion int64 `json:"code_version"`
}

// InfoConfiguration is the effective configuration for the selected board.
type InfoConfiguration struct {
	// Issue contains effective issue policy.
	Issue InfoIssueConfiguration `json:"issue"`

	// Attachment contains effective attachment policy.
	Attachment InfoAttachmentConfiguration `json:"attachment"`
}

// InfoIssueConfiguration contains effective issue policy.
type InfoIssueConfiguration struct {
	// ID contains effective issue identity policy.
	ID InfoIssueIDConfiguration `json:"id"`

	// Summary contains the effective summary limit.
	Summary InfoSummaryConfiguration `json:"summary"`
}

// InfoIssueIDConfiguration contains effective issue identity policy.
type InfoIssueIDConfiguration struct {
	// Prefix is prepended to newly allocated issue IDs.
	Prefix string `json:"prefix"`

	// Strategy selects the issue suffix allocation strategy.
	Strategy string `json:"strategy"`
}

// InfoSummaryConfiguration contains the effective summary limit.
type InfoSummaryConfiguration struct {
	// MaxBytes is the largest accepted summary in bytes.
	MaxBytes uint64 `json:"max_bytes"`
}

// InfoAttachmentConfiguration contains the effective attachment limit.
type InfoAttachmentConfiguration struct {
	// MaxBytes is the largest admitted attachment in bytes.
	MaxBytes uint64 `json:"max_bytes"`
}

// InfoRevision identifies the latest committed logical store change.
type InfoRevision struct {
	// Current is the canonical store revision.
	Current int64 `json:"current"`
}

// InfoIssueInventory reports the selected board's issue population.
type InfoIssueInventory struct {
	// Total is the number of issues in the selected board.
	Total int `json:"total"`

	// ByStatus reports counts in stable issue-status order.
	ByStatus []InfoIssueStatusCount `json:"by_status"`
}

// InfoIssueStatusCount reports one derived issue-status population.
type InfoIssueStatusCount struct {
	// Status is the derived issue status being counted.
	Status string `json:"status"`

	// Count is the number of selected-board issues with Status.
	Count int `json:"count"`
}

// InfoOperation resolves one selected store and board and reads its
// information document.
type InfoOperation interface {
	Read(context.Context, InfoRequest) (InfoResult, error)
}

// Run delegates one information read and renders it.
func (*infoCommand) Run(invocation *Invocation, operation InfoOperation) error {
	result, err := operation.Read(invocation.Context, InfoRequest{
		Store: invocation.Store, Board: invocation.Board,
	})
	if err != nil {
		return err
	}
	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(result)
	}
	humanSummary := "Store:\n  Directory: %s\n  Database:  %s\n" +
		"Project:\n  ID:   %s\n  Name: %s\n" +
		"Board:\n  ID:      %s\n  Project: %s\n  Name:    %s\n" +
		"Schema:\n  Database: %d\n  Code:     %d\n" +
		"Configuration:\n  Issue ID prefix:      %s\n" +
		"  Issue ID strategy:    %s\n" +
		"  Summary maximum:      %d bytes\n" +
		"  Attachment maximum:   %d bytes\n" +
		"Revision:\n  Current: %d\nIssues:\n  Total: %d\n"
	if err := invocation.Output.WriteString(fmt.Sprintf(
		humanSummary,
		result.Store.Directory, result.Store.DatabasePath,
		result.Project.ID, result.Project.Name,
		result.Board.ID, result.Board.ProjectID, result.Board.Name,
		result.Schema.DatabaseVersion, result.Schema.CodeVersion,
		result.Configuration.Issue.ID.Prefix,
		result.Configuration.Issue.ID.Strategy,
		result.Configuration.Issue.Summary.MaxBytes,
		result.Configuration.Attachment.MaxBytes,
		result.Revision.Current, result.Issues.Total,
	)); err != nil {
		return err
	}
	for _, status := range result.Issues.ByStatus {
		if err := invocation.Output.WriteString(fmt.Sprintf(
			"  %s: %d\n", status.Status, status.Count,
		)); err != nil {
			return err
		}
	}
	return nil
}

type webCommand struct {
	webDevelopmentOptions

	Bind      string `name:"bind" default:"127.0.0.1" placeholder:"ADDRESS" help:"Interface for the local HTTP listener. Defaults to 127.0.0.1."`
	Port      int    `name:"port" default:"5757" placeholder:"PORT" help:"HTTP port. Defaults to 5757; use 0 for an ephemeral port."`
	NoBrowser bool   `name:"no-browser" help:"Do not open a browser after the server is ready."`
}

// Help describes the long-running embedded web application contract.
func (*webCommand) Help() string {
	return `Serve the API and browser application from embedded assets until the
invocation is cancelled. --json is not supported because the server does not
produce a finite structured result.`
}

// WebRequest carries one selected namespace and listener invocation to the
// process-owned web application.
type WebRequest struct {
	// Store and Board carry invocation-level namespace selectors.
	Store string
	Board string

	// Bind and Port select the sole public HTTP listener.
	Bind string
	Port int

	// NoBrowser disables browser startup after the server is ready.
	NoBrowser bool

	// Development selects the live frontend under a webdev build.
	Development bool

	// WebDir selects the live frontend source under a webdev build.
	WebDir string

	// Notice receives the public web and API addresses.
	Notice io.Writer

	// Diagnostic receives non-fatal server diagnostics.
	Diagnostic io.Writer
}

// WebOperation owns server composition and the complete web application
// lifetime. Run returns only after shutdown or startup failure.
type WebOperation interface {
	Run(context.Context, WebRequest) error
}

// Run rejects structured mode and delegates the long-running web lifetime.
func (c *webCommand) Run(invocation *Invocation, operation WebOperation) error {
	if invocation.Output.JSON() {
		return UsageErrorf("web: --json is not supported")
	}
	notice := invocation.Output.Stdout()
	if invocation.Output.Quiet() {
		notice = io.Discard
	}
	return operation.Run(invocation.Context, WebRequest{
		Store: invocation.Store, Board: invocation.Board,
		Bind: c.Bind, Port: c.Port, NoBrowser: c.NoBrowser,
		Development: c.Development, WebDir: c.WebDir,
		Notice: notice, Diagnostic: invocation.Output.Stderr(),
	})
}

type completionCommand struct {
	Shell string `arg:"" optional:"" default:"" enum:"bash,zsh,fish," help:"Shell name."`
}

// Help describes supported shells and automatic shell detection.
func (*completionCommand) Help() string {
	return `Generate a shell completion script. Accepted shells are bash, zsh, and fish.
When shell is omitted, card detects the shell from SHELL or FISH_VERSION and
fails when detection is not possible.`
}

// Run delegates completion script generation to the live Kong command model.
func (c *completionCommand) Run(context *kong.Context) error {
	return (&komplete.Command{Shell: c.Shell}).Run(context)
}
