// Package cli wires the `clu` command-line interface on top of internal/store.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/rovak/clu/internal/config"
	"github.com/rovak/clu/internal/store"
)

// CLI is the kong-defined command structure.
type CLI struct {
	Dir   string `name:"dir" env:"CLU_DIR" default:".clu" help:"Project directory (database, config, templates). When unset, falls back to the main worktree's .clu/ so secondary worktrees share state automatically."`
	JSON  bool   `name:"json" help:"Emit machine-readable JSON instead of human output."`
	Quiet bool   `short:"q" name:"quiet" help:"Suppress non-essential output (errors still go to stderr)."`

	// Group keys must match those declared in kong.ExplicitGroups
	// below; mismatches surface as kong.New errors at construct time.
	// Order of fields here drives order of commands within each group.

	// --- issues: the core CRUD + state loop ---
	Init    InitCmd    `cmd:"" group:"issues" help:"Initialize the database in the current directory."`
	Create  CreateCmd  `cmd:"" group:"issues" help:"Create a new issue."`
	List    ListCmd    `cmd:"" group:"issues" aliases:"ls" help:"List issues."`
	Ready   ReadyCmd   `cmd:"" group:"issues" help:"List issues that are ready to work on."`
	Blocked BlockedCmd `cmd:"" group:"issues" help:"List issues that have at least one open dependency."`
	Show    ShowCmd    `cmd:"" group:"issues" help:"Show details for one issue."`
	Claim   ClaimCmd   `cmd:"" group:"issues" help:"Claim the next ready issue (or a specific one)."`
	Close   CloseCmd   `cmd:"" group:"issues" help:"Close an issue."`
	Cancel  CancelCmd  `cmd:"" group:"issues" help:"Cancel an issue and all its transitive dependents."`
	Reopen  ReopenCmd  `cmd:"" group:"issues" help:"Reopen a closed issue."`
	Update  UpdateCmd  `cmd:"" group:"issues" help:"Update fields on an issue."`

	// --- edits: sugar + relations ---
	Assign   AssignCmd   `cmd:"" group:"edits" help:"Assign an issue (sugar for 'update --assignee')."`
	Priority PriorityCmd `cmd:"" group:"edits" help:"Set an issue's priority (sugar for 'update -p N')."`
	Describe DescribeCmd `cmd:"" group:"edits" help:"Set or clear an issue's description (sugar for 'update --description')."`
	Note     NoteCmd     `cmd:"" group:"edits" help:"Manage an issue's freeform notes."`
	Comment  CommentCmd  `cmd:"" group:"edits" help:"Manage threaded comments on an issue."`
	Label    LabelCmd    `cmd:"" group:"edits" help:"Manage labels on an issue."`
	Tag      TagCmd      `cmd:"" group:"edits" help:"Add labels to an issue (alias for 'label add')."`
	Dep      DepCmd      `cmd:"" group:"edits" help:"Manage dependency edges."`
	Link     LinkCmd     `cmd:"" group:"edits" help:"Add a dependency edge (alias for 'dep add')."`
	Defer    DeferCmd    `cmd:"" group:"edits" help:"Defer an issue until a later time."`
	Undefer  UndeferCmd  `cmd:"" group:"edits" help:"Clear an issue's deferral."`

	// --- coordination: agents, brief, locks, mailbox ---
	Agent  AgentCmd  `cmd:"" group:"coord" help:"Manage agents — list declared (config.yaml) and live (heartbeat) state."`
	Brief  BriefCmd  `cmd:"" group:"coord" help:"Print agent workflow context: AGENTS.md, declared agents, who's live, persisted memories."`
	Lock   LockCmd   `cmd:"" group:"coord" help:"Acquire a named lock for ad-hoc coordination (deploy slots, build dirs, shared resources)."`
	Unlock UnlockCmd `cmd:"" group:"coord" help:"Release a named lock."`
	Locks  LocksCmd  `cmd:"" group:"coord" help:"List current locks (live and stale)."`
	Ping   PingCmd   `cmd:"" group:"coord" help:"Send a fire-and-forget message to another agent's mailbox (TTL'd, doesn't pollute the work log)."`
	Inbox  InboxCmd  `cmd:"" group:"coord" help:"Read pings addressed to you (the mailbox). Marks read on consume unless --peek."`

	// --- workflows ---
	Run        RunCmd        `cmd:"" group:"workflow" help:"Instantiate a workflow template into issues + deps."`
	Template   TemplateCmd   `cmd:"" group:"workflow" help:"Inspect and validate workflow templates."`
	Checkpoint CheckpointCmd `cmd:"" group:"workflow" help:"Pass or fail a checkpoint step."`
	Approve    ApproveCmd    `cmd:"" group:"workflow" help:"Approve a checkpoint (sugar for 'checkpoint pass')."`

	// --- inspection: read-only diagnostics ---
	Count    CountCmd    `cmd:"" group:"inspect" help:"Count issues matching the same filters as 'list'."`
	Stats    StatsCmd    `cmd:"" group:"inspect" help:"Show issue counts grouped by status, agent, and type."`
	Info     InfoCmd     `cmd:"" group:"inspect" help:"Show database path, schema version, and a summary of issues."`
	Doctor   DoctorCmd   `cmd:"" group:"inspect" help:"Run integrity and health checks against the database."`
	Statuses StatusesCmd `cmd:"" group:"inspect" help:"List valid issue statuses."`
	Types    TypesCmd    `cmd:"" group:"inspect" help:"List valid issue types."`

	// --- data + servers ---
	KV     KVCmd     `cmd:"" group:"data" help:"Manage a generic key-value store (feature flags, env, scratch data)."`
	Export ExportCmd `cmd:"" group:"data" help:"Export all issues + deps + labels as JSONL."`
	Import ImportCmd `cmd:"" group:"data" help:"Import JSONL produced by 'clu export'."`
	SQL    SqlCmd    `cmd:"" group:"data" name:"sql" help:"Run an ad-hoc SQL query (read-only by default; pass --write for DML/DDL)."`
	Cron   CronCmd   `cmd:"" group:"data" help:"Schedule recurring clu invocations (drive from OS cron / launchd)."`
	HTTP   HTTPCmd   `cmd:"" group:"data" name:"http" help:"Start a REST API server backed by the project's store."`
	Web    WebCmd    `cmd:"" group:"data" name:"web" help:"Launch the web UI (REST API in-process + TanStack Start server)."`

	// --- environments: git worktrees and their bootstrap ---
	Worktree WorktreeCmd `cmd:"" group:"coord" help:"Manage git worktrees with project-defined bootstrap (copy files, run setup commands)."`

	// --- meta ---
	Version    VersionCmd    `cmd:"" group:"meta" help:"Print version information."`
	Completion CompletionCmd `cmd:"" group:"meta" help:"Generate a shell completion script."`
}

// runCtx is passed to each command's Run method.
type runCtx struct {
	ctx    context.Context
	dir    string
	stdout io.Writer
	stderr io.Writer
	json   bool
	quiet  bool
}

// eachID applies fn to every id, collecting successes and per-id errors.
// Successful results are emitted as a JSON array in --json mode (call sites
// supply a slice with one entry per success). In human mode, fn is expected
// to print its own notice as it goes. Errors are aggregated via errors.Join
// so partial work still completes — useful for batch close/reopen/undefer.
func eachID(r *runCtx, ids []string, fn func(string) (any, error)) error {
	results := make([]any, 0, len(ids))
	var errs []error
	for _, id := range ids {
		v, err := fn(id)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", id, err))
			continue
		}
		results = append(results, v)
	}
	if r.json {
		_ = r.emitJSON(results)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// watchLoop calls render() once per `interval`, emitting output only when
// it changes from the previous render. Returns when ctx is cancelled,
// returning ctx.Err() so the caller maps it to exit 130 (same convention
// as `clu ready --wait`).
//
// Two output styles, picked based on whether w is a TTY:
//
//   - TTY: in-place ANSI redraw. ESC[<n>A moves the cursor up n lines,
//     ESC[J clears to end of screen. Clean for human viewing.
//
//   - Non-TTY (pipes, files, tests): emit each new state as a clean
//     block separated by a blank line. No ANSI cursor codes, which a
//     downstream process can parse cleanly. Crucially, downstream
//     consumers are NOT woken on every tick — only on actual change.
//
// In both cases, an unchanged tick is silent — no bytes written.
func watchLoop(ctx context.Context, w io.Writer, interval time.Duration, render func() (string, error)) error {
	if interval <= 0 {
		interval = time.Second
	}
	tty := isTerminal(w)
	var prev string
	var prevLines int
	first := true
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		out, err := render()
		if err != nil {
			return err
		}
		if first || out != prev {
			if tty {
				if prevLines > 0 {
					fmt.Fprintf(w, "\033[%dA\033[J", prevLines)
				}
				fmt.Fprint(w, out)
				prevLines = strings.Count(out, "\n")
			} else {
				if !first {
					fmt.Fprintln(w) // blank-line separator between blocks
				}
				fmt.Fprint(w, out)
			}
			prev = out
			first = false
		}
		select {
		case <-ctx.Done():
			if tty {
				// Newline so the shell prompt doesn't land mid-line
				// after the final redraw.
				fmt.Fprintln(w)
			}
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// isTerminal reports whether w refers to a character device (interactive
// terminal). Returns false for pipes, files, and any non-*os.File writer
// (e.g. bytes.Buffer in tests).
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// notice writes a friendly status message to stdout. Suppressed by
// --quiet, and also suppressed in --json mode so it doesn't pollute
// the JSON document on stdout. Use for narrative output ("closed
// bd-1234", "claimed …"); data output (IDs, lists, JSON) goes
// directly to r.stdout.
func (r *runCtx) notice(format string, args ...any) {
	if r.quiet || r.json {
		return
	}
	fmt.Fprintf(r.stdout, format, args...)
}

// emitJSON writes v to stdout as a single JSON value. HTML escaping is
// disabled so substrings like "<none>" survive round-trip; the trailing
// newline json.Encoder always appends keeps output line-oriented.
// Used by every write command so the contract under --json is uniform:
// exactly one JSON value per invocation.
func (r *runCtx) emitJSON(v any) error {
	enc := json.NewEncoder(r.stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func (c *runCtx) dbPath() string { return filepath.Join(c.dir, "data.sqlite") }

// openStore loads the per-project config and opens the database with it.
// Config absence is fine; defaults are applied silently.
func (c *runCtx) openStore() (*store.Store, error) {
	if _, err := os.Stat(c.dbPath()); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no database at %s — run `clu init`", c.dbPath())
	}
	cfg, err := config.Load(c.dir)
	if err != nil {
		return nil, err
	}
	return store.Open(c.dbPath(), store.WithIDPrefix(cfg.IDPrefix))
}

func withStore(c *runCtx, fn func(*store.Store) error) error {
	s, err := c.openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	return fn(s)
}

func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "unknown"
}

// agentPtr returns nil for an empty agent string, else a pointer to it.
// Translates the CLI's empty-default convention into the store's
// "nil = unassigned lane" convention.
func agentPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// hasEmptyFlagValue reports whether argv contains a literal empty value
// for `flag` — i.e. `--flag ""` or `--flag=""`. Kong's sep-list parser
// silently drops a bare empty into an empty slice, which makes it
// impossible to distinguish "user typed --flag with no content" from
// "user didn't pass --flag at all" by looking at the parsed slice
// alone. Used by flags where empty should be a hard error (capability).
func hasEmptyFlagValue(argv []string, flag string) bool {
	prefix := flag + "="
	for i, a := range argv {
		if a == flag && i+1 < len(argv) && argv[i+1] == "" {
			return true
		}
		if a == prefix {
			return true
		}
	}
	return false
}

// Run is the entrypoint used by main and the tests.
func Run(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	// Pre-flight: handle `-V` / `--version` / `--version-flag` before
	// kong, which requires a subcommand and conflicts with our custom
	// Exit hook. Anywhere in the arg list short-circuits to the version
	// command.
	for _, a := range args {
		if a == "-V" || a == "--version" || a == "--version-flag" {
			rctx := &runCtx{ctx: ctx, stdout: stdout, stderr: stderr}
			// Ignore err — VersionCmd.Run only fails when emitJSON
			// fails (i.e. stdout's broken), at which point we have
			// no way to report the error anyway.
			_ = (&VersionCmd{}).Run(rctx)
			return 0
		}
	}
	// Pre-flight: detect --help / -h anywhere in args. Kong prints help
	// inside parser.Parse() (via the helpFlag's BeforeReset hook), but it
	// also calls Exit(0) which we've overridden to a no-op — so without
	// short-circuiting here we'd then run the selected command anyway.
	// Asking for help should be exit 0, not 2, and must never run a side
	// effect (e.g. `cli init --help` creating the database).
	// Bare `clu` (no args) is treated the same as `clu --help` — print
	// usage and exit 0. Matches git/gh/kubectl conventions; avoids
	// kong's default "error: expected one of …" which is hostile to
	// someone just trying to remember the tool's shape.
	wantHelp := len(args) == 0
	for _, a := range args {
		if a == "--help" || a == "-h" {
			wantHelp = true
			break
		}
	}
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("clu"),
		kong.Description("clu — SQLite-backed issue tracker for AI agents. Use -V or 'clu version' to print version."),
		kong.Writers(stdout, stderr),
		kong.Exit(func(int) {}), // we manage exit codes ourselves
		kong.UsageOnError(),
		// Compact + NoExpandSubcommands keeps the top-level help to one
		// line per command (name + description in columns) instead of
		// the default two-line-per-command layout. With ~40 commands
		// the default form is a wall of text.
		kong.ConfigureHelp(kong.HelpOptions{
			Compact:             true,
			NoExpandSubcommands: true,
		}),
		// Group headings for `clu --help`. The order here is the order
		// they appear in the help output. Group keys must match the
		// `group:"..."` tags on each command field in the CLI struct
		// above; an unknown key fails at parser construction.
		kong.ExplicitGroups([]kong.Group{
			{Key: "issues", Title: "Working with issues"},
			{Key: "edits", Title: "Edits & relations"},
			{Key: "coord", Title: "Multi-agent coordination"},
			{Key: "workflow", Title: "Workflows"},
			{Key: "inspect", Title: "Inspection"},
			{Key: "data", Title: "Data & servers"},
			{Key: "meta", Title: "Meta"},
		}),
		kong.Vars{"user": currentUser()},
	)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	if wantHelp {
		// Inject --help when the user gave no args at all, so kong's
		// helpFlag hook fires. Without this, Parse([]) silently exits
		// without printing anything.
		if len(args) == 0 {
			args = []string{"--help"}
		}
		// Parse for the side effect of printing help. Any parse error is
		// secondary to the help request — it would've been raised because
		// the user didn't specify a subcommand (or a required arg), which
		// is irrelevant when they're asking for help.
		_, _ = parser.Parse(args)
		return 0
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	cli.Dir = resolveCluDir(cli.Dir)
	rctx := &runCtx{ctx: ctx, dir: cli.Dir, stdout: stdout, stderr: stderr, json: cli.JSON, quiet: cli.Quiet}
	if err := kctx.Run(rctx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130 // standard for SIGINT / cancelled wait
		}
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

// ---- Formatting helpers ----

// loadLabelsFor fetches labels for every issue in the slice in one query.
// Always returns a non-nil map.
func loadLabelsFor(ctx context.Context, s *store.Store, issues []store.Issue) (map[string][]string, error) {
	ids := make([]string, len(issues))
	for i, is := range issues {
		ids[i] = is.ID
	}
	return s.LoadLabels(ctx, ids)
}

// loadBlockedFor returns the set of issue IDs that have at least one
// non-closed dependency. Always non-nil.
func loadBlockedFor(ctx context.Context, s *store.Store, issues []store.Issue) (map[string]bool, error) {
	ids := make([]string, len(issues))
	for i, is := range issues {
		ids[i] = is.ID
	}
	return s.IDsBlocked(ctx, ids)
}

// issueOut wraps a store.Issue with its labels for JSON output. Embeds
// store.Issue so its json tags are promoted to top-level.
type issueOut struct {
	store.Issue
	Labels  []string `json:"labels,omitempty"`
	Blocked bool     `json:"blocked,omitempty"` // derived: any non-closed parent
}

// issueShowOut adds parents/blocks/comments for the show command.
// Slices and the blocked flag are emitted *unconditionally* (omitempty
// removed) so generic consumers see a uniform shape — `.labels[]`
// always exists as an array, never as a missing key.
type issueShowOut struct {
	store.Issue
	Labels   []string        `json:"labels"`
	Depends  []string        `json:"depends_on"`
	Blocks   []string        `json:"blocks"`
	Comments []store.Comment `json:"comments"`
	Blocked  bool            `json:"blocked"`
}

// displayStatus is the user-facing status. It's the stored status for
// closed/in_progress rows; for open rows with at least one non-closed
// dependency, it becomes the derived "blocked".
func displayStatus(i store.Issue, blocked bool) string {
	if i.Status == "open" && blocked {
		return "blocked"
	}
	return i.Status
}

func printIssues(r *runCtx, issues []store.Issue, labels map[string][]string, blocked map[string]bool) {
	if r.json {
		out := make([]issueOut, len(issues))
		for i, is := range issues {
			out[i] = issueOut{Issue: is, Labels: labels[is.ID], Blocked: blocked[is.ID]}
		}
		_ = r.emitJSON(out)
		return
	}
	if len(issues) == 0 {
		fmt.Fprintln(r.stdout, "(none)")
		return
	}
	now := time.Now().Unix()
	for _, i := range issues {
		assignee := "-"
		if i.Assignee != nil {
			assignee = *i.Assignee
		}
		status := displayStatus(i, blocked[i.ID])
		// Surface a deferred row's wait state inline. `ready` already
		// excludes deferred issues; `list` previously showed them as
		// plain "open" with no indication. Past-due defers show as
		// "overdue" so the user notices something they thought was
		// gated is back in play.
		if i.DeferUntil != nil && status != "closed" && status != "cancelled" {
			if *i.DeferUntil > now {
				status = "deferred"
			} else {
				status = "overdue"
			}
		}
		fmt.Fprintf(r.stdout, "%s  p%d  %-12s  %-12s  %s", i.ID, i.Priority, status, assignee, i.Title)
		if ls := labels[i.ID]; len(ls) > 0 {
			fmt.Fprintf(r.stdout, "  [%s]", strings.Join(ls, ", "))
		}
		fmt.Fprintln(r.stdout)
	}
}

func printIssue(r *runCtx, i store.Issue, parents, blocks, labels []string, comments []store.Comment, blocked bool) {
	if r.json {
		// Always emit non-nil slices so `jq -r '.labels[]'` etc. don't
		// trip on missing keys when the issue has no labels / deps /
		// comments.
		if parents == nil {
			parents = []string{}
		}
		if blocks == nil {
			blocks = []string{}
		}
		if labels == nil {
			labels = []string{}
		}
		if comments == nil {
			comments = []store.Comment{}
		}
		out := issueShowOut{Issue: i, Labels: labels, Depends: parents, Blocks: blocks, Comments: comments, Blocked: blocked}
		_ = r.emitJSON(out)
		return
	}
	w := r.stdout
	fmt.Fprintf(w, "ID:       %s\n", i.ID)
	fmt.Fprintf(w, "Title:    %s\n", i.Title)
	fmt.Fprintf(w, "Type:     %s\n", i.Type)
	fmt.Fprintf(w, "Status:   %s\n", displayStatus(i, blocked))
	fmt.Fprintf(w, "Priority: %d\n", i.Priority)
	if i.Assignee != nil {
		fmt.Fprintf(w, "Assignee: %s\n", *i.Assignee)
	}
	fmt.Fprintf(w, "Created:  %s\n", time.Unix(i.Created, 0).Format(time.RFC3339))
	fmt.Fprintf(w, "Updated:  %s\n", time.Unix(i.Updated, 0).Format(time.RFC3339))
	if i.Closed != nil {
		fmt.Fprintf(w, "Closed:   %s\n", time.Unix(*i.Closed, 0).Format(time.RFC3339))
	}
	if i.DeferUntil != nil {
		fmt.Fprintf(w, "Deferred: %s\n", time.Unix(*i.DeferUntil, 0).Format(time.RFC3339))
	}
	if i.Description != nil && *i.Description != "" {
		fmt.Fprintf(w, "Description:\n  %s\n", strings.ReplaceAll(*i.Description, "\n", "\n  "))
	}
	if i.Notes != nil && *i.Notes != "" {
		fmt.Fprintf(w, "Notes:\n  %s\n", strings.ReplaceAll(*i.Notes, "\n", "\n  "))
	}
	if len(labels) > 0 {
		fmt.Fprintf(w, "Labels:   %s\n", strings.Join(labels, ", "))
	}
	if len(parents) > 0 {
		fmt.Fprintf(w, "Depends:  %s\n", strings.Join(parents, ", "))
	}
	if len(blocks) > 0 {
		fmt.Fprintf(w, "Blocks:   %s\n", strings.Join(blocks, ", "))
	}
	if len(comments) > 0 {
		fmt.Fprintf(w, "Comments (%d):\n", len(comments))
		for _, cm := range comments {
			ts := time.Unix(cm.Created, 0).Format(time.RFC3339)
			fmt.Fprintf(w, "  [#%d] %s %s\n", cm.ID, cm.Author, ts)
			for _, line := range strings.Split(cm.Body, "\n") {
				fmt.Fprintf(w, "    %s\n", line)
			}
		}
	}
}
