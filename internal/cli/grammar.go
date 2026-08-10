package cli

import "github.com/alecthomas/kong"

// commandTree is the declarative grammar shared by help, parsing, and shell
// completion. Command-family files add syntax and Run methods to these nodes;
// commandTree does not perform domain work.
type commandTree struct {
	Store         string `name:"store" group:"global" placeholder:"PATH" help:"Physical Cardamom store directory. Overrides automatic discovery ($CARDAMOM_STORE)."`
	BoardSelector string `name:"board" group:"global" predictor:"boards" placeholder:"BOARD" help:"Coordination board ID or exact name ($CARDAMOM_BOARD)."`

	// AmbientStore and AmbientBoardSelector retain environment defaults separately
	// from explicit flags so aggregate web mode can discard local selectors.
	AmbientStore         string `name:"private-store-from-env" hidden:"" env:"CARDAMOM_STORE"`
	AmbientBoardSelector string `name:"private-board-from-env" hidden:"" env:"CARDAMOM_BOARD"`

	Actor string `name:"actor" group:"global" env:"CARDAMOM_ACTOR" default:"${default_actor}" placeholder:"ACTOR" help:"Identity that owns active claims and attributes changes. Defaults to the current OS username."`
	JSON  bool   `name:"json" group:"global" help:"Emit machine-readable JSON or JSON Lines output."`
	Quiet bool   `name:"quiet" short:"q" group:"global" help:"Suppress status notices while preserving requested output and errors."`

	Init    initCommand          `cmd:"" group:"starting" help:"Initialize a store, project, and first board."`
	Project projectCommand       `cmd:"" group:"starting" help:"Create, inspect, edit, and list project namespaces."`
	Board   boardCommand         `cmd:"" group:"starting" help:"Create, select, and inspect coordination boards."`
	Config  configurationCommand `cmd:"" group:"configuration" help:"Inspect and update typed configuration."`

	Create createCommand    `cmd:"" group:"planning" help:"Create a board-scoped issue."`
	Apply  applyCommand     `cmd:"" group:"planning" help:"Apply an issue graph from JSON."`
	Edit   issueEditCommand `cmd:"" group:"planning" help:"Edit issue metadata and graph relationships."`

	List    listCommand    `cmd:"" group:"inspection" help:"List issues."`
	Ready   readyCommand   `cmd:"" group:"inspection" help:"List issues that are ready to work on."`
	Blocked blockedCommand `cmd:"" group:"inspection" help:"List issues with open dependencies."`
	Show    showCommand    `cmd:"" group:"inspection" help:"Show details for one issue."`
	Dump    dumpCommand    `cmd:"" group:"inspection" help:"Publish a human-readable Markdown dump."`

	Claim      claimCommand      `cmd:"" group:"execution" help:"Claim ready work or a specific issue."`
	Release    releaseCommand    `cmd:"" group:"execution" help:"Release custody held by the current actor."`
	Close      closeCommand      `cmd:"" group:"execution" help:"Close successfully completed issues."`
	Cancel     cancelCommand     `cmd:"" group:"execution" help:"Cancel issues and their dependents."`
	Reopen     reopenCommand     `cmd:"" group:"execution" help:"Reopen terminal issues without claiming them."`
	Checkpoint checkpointCommand `cmd:"" group:"execution" help:"Approve or deny a checkpoint."`

	Log    logCommand    `cmd:"" group:"records" help:"Post and show durable issue log entries."`
	State  stateCommand  `cmd:"" group:"records" help:"Manage mutable issue recovery state."`
	Result resultCommand `cmd:"" group:"records" help:"Manage an issue result."`

	Attachment attachmentCommand `cmd:"" group:"attachments" help:"Add, inspect, download, remove, and collect board attachments."`

	Backup  backupCommand  `cmd:"" group:"backup" help:"Write selected boards to a portable archive."`
	Restore restoreCommand `cmd:"" group:"backup" help:"Restore every board from a portable archive."`

	Mail  mailCommand  `cmd:"" group:"mail" help:"Send and receive ephemeral mail."`
	Lease leaseCommand `cmd:"" group:"leases" help:"Coordinate ownership of named external resources."`

	Info       infoCommand       `cmd:"" group:"process" help:"Show store identity and inventory."`
	Version    versionCommand    `cmd:"" group:"process" help:"Print version information."`
	Skill      skillCommand      `cmd:"" group:"process" help:"Install the embedded Cardamom skill."`
	Web        webCommand        `cmd:"" group:"process" help:"Launch the local web application."`
	Completion completionCommand `cmd:"" group:"process" help:"Generate shell completion."`
}

func commandGroups() kong.Option {
	return kong.ExplicitGroups([]kong.Group{
		{Key: "starting", Title: "Starting a board"},
		{Key: "configuration", Title: "Configuring Cardamom"},
		{Key: "planning", Title: "Planning work"},
		{Key: "inspection", Title: "Inspecting work"},
		{Key: "execution", Title: "Executing work"},
		{Key: "records", Title: "Recording work"},
		{Key: "attachments", Title: "Managing attachments"},
		{Key: "backup", Title: "Backup and restoration"},
		{Key: "mail", Title: "Mail"},
		{Key: "leases", Title: "Leases"},
		{Key: "process", Title: "Running locally"},
		{Key: "global", Title: "Global flags"},
	})
}
