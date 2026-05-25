// Package cli wires the bd command-line interface on top of internal/store.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/rovak/beadsv2/internal/store"
)

// CLI is the kong-defined command structure.
type CLI struct {
	Dir string `name:"dir" env:"BEADS_DIR" default:".beads" help:"Beads directory."`

	Init   InitCmd   `cmd:"" help:"Initialize the beads database in the current directory."`
	Create CreateCmd `cmd:"" help:"Create a new issue."`
	List   ListCmd   `cmd:"" help:"List issues."`
	Ready  ReadyCmd  `cmd:"" help:"List issues that are ready to work on."`
	Show   ShowCmd   `cmd:"" help:"Show details for one issue."`
	Claim  ClaimCmd  `cmd:"" help:"Claim the next ready issue (or a specific one)."`
	Close  CloseCmd  `cmd:"" help:"Close an issue."`
	Update UpdateCmd `cmd:"" help:"Update fields on an issue."`
	Dep    DepCmd    `cmd:"" help:"Manage dependency edges."`
}

// runCtx is passed to each command's Run method.
type runCtx struct {
	ctx    context.Context
	dir    string
	stdout io.Writer
	stderr io.Writer
}

func (c *runCtx) dbPath() string { return filepath.Join(c.dir, "beads.db") }

func (c *runCtx) openStore() (*store.Store, error) {
	if _, err := os.Stat(c.dbPath()); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no beads database at %s — run `bd init`", c.dbPath())
	}
	return store.Open(c.dbPath())
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

// Run is the entrypoint used by main and the tests.
func Run(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("bd"),
		kong.Description("Minimal SQLite-backed issue tracker."),
		kong.Writers(stdout, stderr),
		kong.Exit(func(int) {}), // we manage exit codes ourselves
		kong.UsageOnError(),
		kong.Vars{"user": currentUser()},
	)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	rctx := &runCtx{ctx: ctx, dir: cli.Dir, stdout: stdout, stderr: stderr}
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

func printIssues(w io.Writer, issues []store.Issue) {
	if len(issues) == 0 {
		fmt.Fprintln(w, "(none)")
		return
	}
	for _, i := range issues {
		assignee := "-"
		if i.Assignee != nil {
			assignee = *i.Assignee
		}
		agent := "-"
		if i.Agent != nil {
			agent = *i.Agent
		}
		fmt.Fprintf(w, "%s  p%d  %-12s  %-12s  %-10s  %s\n", i.ID, i.Priority, i.Status, agent, assignee, i.Title)
	}
}

func printIssue(w io.Writer, i store.Issue, parents, blocks []string) {
	fmt.Fprintf(w, "ID:       %s\n", i.ID)
	fmt.Fprintf(w, "Title:    %s\n", i.Title)
	fmt.Fprintf(w, "Type:     %s\n", i.Type)
	fmt.Fprintf(w, "Status:   %s\n", i.Status)
	fmt.Fprintf(w, "Priority: %d\n", i.Priority)
	if i.Agent != nil {
		fmt.Fprintf(w, "Agent:    %s\n", *i.Agent)
	}
	if i.Assignee != nil {
		fmt.Fprintf(w, "Assignee: %s\n", *i.Assignee)
	}
	fmt.Fprintf(w, "Created:  %s\n", time.Unix(i.Created, 0).Format(time.RFC3339))
	fmt.Fprintf(w, "Updated:  %s\n", time.Unix(i.Updated, 0).Format(time.RFC3339))
	if i.Closed != nil {
		fmt.Fprintf(w, "Closed:   %s\n", time.Unix(*i.Closed, 0).Format(time.RFC3339))
	}
	if len(parents) > 0 {
		fmt.Fprintf(w, "Depends:  %s\n", strings.Join(parents, ", "))
	}
	if len(blocks) > 0 {
		fmt.Fprintf(w, "Blocks:   %s\n", strings.Join(blocks, ", "))
	}
}
