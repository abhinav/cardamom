package main

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

func (c *runCtx) openStore() (*Store, error) {
	if _, err := os.Stat(c.dbPath()); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no beads database at %s — run `bd init`", c.dbPath())
	}
	return Open(c.dbPath())
}

func withStore(c *runCtx, fn func(*Store) error) error {
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
// Used to translate the CLI's empty-default convention into the store's
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

// ---- Commands ----

type InitCmd struct{}

func (c *InitCmd) Run(r *runCtx) error {
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return err
	}
	s, err := Open(r.dbPath())
	if err != nil {
		return err
	}
	defer s.Close()
	fmt.Fprintf(r.stdout, "initialized %s\n", r.dbPath())
	return nil
}

type CreateCmd struct {
	Priority int      `short:"p" default:"2" help:"Priority (0=highest)."`
	Type     string   `short:"t" default:"task" help:"Issue type."`
	Agent    string   `short:"a" help:"Assign to an agent lane (e.g. code-reviewer)."`
	Title    []string `arg:"" required:"" help:"Issue title."`
}

func (c *CreateCmd) Run(r *runCtx) error {
	title := strings.Join(c.Title, " ")
	return withStore(r, func(s *Store) error {
		i, err := s.Create(title, c.Type, c.Priority, agentPtr(c.Agent))
		if err != nil {
			return err
		}
		fmt.Fprintln(r.stdout, i.ID)
		return nil
	})
}

type ListCmd struct {
	Status string `default:"open" enum:"open,in_progress,closed,all" help:"Filter by status."`
	Agent  string `short:"a" help:"Filter by agent lane."`
}

func (c *ListCmd) Run(r *runCtx) error {
	filter := c.Status
	if filter == "all" {
		filter = ""
	}
	return withStore(r, func(s *Store) error {
		issues, err := s.List(filter, agentPtr(c.Agent))
		if err != nil {
			return err
		}
		printIssues(r.stdout, issues)
		return nil
	})
}

type ReadyCmd struct {
	N        int           `short:"n" default:"20" help:"Maximum number of issues."`
	Agent    string        `short:"a" help:"Lane to query (default: unassigned)."`
	Wait     bool          `help:"Block until at least one issue is ready."`
	Interval time.Duration `default:"250ms" help:"Poll interval when --wait is set."`
}

func (c *ReadyCmd) Run(r *runCtx) error {
	return withStore(r, func(s *Store) error {
		var (
			issues []Issue
			err    error
		)
		if c.Wait {
			issues, err = s.WaitReady(r.ctx, c.N, agentPtr(c.Agent), c.Interval)
		} else {
			issues, err = s.Ready(c.N, agentPtr(c.Agent))
		}
		if err != nil {
			return err
		}
		printIssues(r.stdout, issues)
		return nil
	})
}

type ShowCmd struct {
	ID string `arg:"" help:"Issue ID."`
}

func (c *ShowCmd) Run(r *runCtx) error {
	return withStore(r, func(s *Store) error {
		i, err := s.Get(c.ID)
		if err != nil {
			return err
		}
		parents, blocks, err := s.Deps(i.ID)
		if err != nil {
			return err
		}
		printIssue(r.stdout, i, parents, blocks)
		return nil
	})
}

type ClaimCmd struct {
	As       string        `default:"${user}" help:"Assignee name (defaults to current user)."`
	Agent    string        `short:"a" help:"Lane to claim from (default: unassigned)."`
	Wait     bool          `help:"Block until something is claimable in this lane."`
	Interval time.Duration `default:"250ms" help:"Poll interval when --wait is set."`
	ID       string        `arg:"" optional:"" help:"Specific issue to claim; omit for next ready."`
}

func (c *ClaimCmd) Run(r *runCtx) error {
	return withStore(r, func(s *Store) error {
		if c.ID != "" {
			i, err := s.ClaimByID(c.ID, c.As)
			if err != nil {
				return err
			}
			fmt.Fprintf(r.stdout, "claimed %s (%s)\n", i.ID, i.Title)
			return nil
		}
		agent := agentPtr(c.Agent)
		for {
			i, err := s.Claim(c.As, agent)
			if err == nil {
				fmt.Fprintf(r.stdout, "claimed %s (%s)\n", i.ID, i.Title)
				return nil
			}
			if !errors.Is(err, ErrNotFound) {
				return err
			}
			if !c.Wait {
				return errors.New("no ready issues")
			}
			// Wait for something to become ready, then retry the claim.
			// Another agent may steal it between Wait and Claim — that's
			// fine, we'll just block again.
			if _, err := s.WaitReady(r.ctx, 1, agent, c.Interval); err != nil {
				return err
			}
		}
	})
}

type CloseCmd struct {
	ID string `arg:"" help:"Issue ID."`
}

func (c *CloseCmd) Run(r *runCtx) error {
	return withStore(r, func(s *Store) error {
		i, err := s.MarkClosed(c.ID)
		if err != nil {
			return err
		}
		fmt.Fprintf(r.stdout, "closed %s\n", i.ID)
		return nil
	})
}

type UpdateCmd struct {
	ID       string  `arg:"" help:"Issue ID."`
	Priority *int    `short:"p" help:"New priority (0=highest)."`
	Status   *string `help:"New status."`
	Assignee *string `help:"Set assignee."`
	Unassign bool    `help:"Clear assignee."`
	Agent    *string `short:"a" help:"Set agent lane."`
	NoAgent  bool    `help:"Clear agent lane."`
	Title    *string `help:"New title."`
}

func (c *UpdateCmd) Run(r *runCtx) error {
	return withStore(r, func(s *Store) error {
		var f UpdateFields
		f.Priority = c.Priority
		f.Status = c.Status
		f.Title = c.Title
		switch {
		case c.Unassign:
			var none *string
			f.Assignee = &none
		case c.Assignee != nil:
			f.Assignee = &c.Assignee
		}
		switch {
		case c.NoAgent:
			var none *string
			f.Agent = &none
		case c.Agent != nil:
			f.Agent = &c.Agent
		}
		i, err := s.Update(c.ID, f)
		if err != nil {
			return err
		}
		fmt.Fprintf(r.stdout, "updated %s\n", i.ID)
		return nil
	})
}

type DepCmd struct {
	Add DepAddCmd `cmd:"" help:"Add a dependency edge."`
	Rm  DepRmCmd  `cmd:"" aliases:"remove" help:"Remove a dependency edge."`
}

type DepAddCmd struct {
	Child  string `arg:"" help:"Child issue (the one that depends)."`
	Parent string `arg:"" help:"Parent issue (the blocker)."`
}

func (c *DepAddCmd) Run(r *runCtx) error {
	return withStore(r, func(s *Store) error {
		if err := s.AddDep(c.Child, c.Parent); err != nil {
			return err
		}
		fmt.Fprintf(r.stdout, "%s depends on %s\n", c.Child, c.Parent)
		return nil
	})
}

type DepRmCmd struct {
	Child  string `arg:"" help:"Child issue."`
	Parent string `arg:"" help:"Parent issue."`
}

func (c *DepRmCmd) Run(r *runCtx) error {
	return withStore(r, func(s *Store) error {
		if err := s.RemoveDep(c.Child, c.Parent); err != nil {
			return err
		}
		fmt.Fprintf(r.stdout, "removed %s -> %s\n", c.Child, c.Parent)
		return nil
	})
}

// ---- Formatting helpers ----

func printIssues(w io.Writer, issues []Issue) {
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

func printIssue(w io.Writer, i Issue, parents, blocks []string) {
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
