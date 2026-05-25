package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

type Env struct {
	Dir    string
	Stdout io.Writer
	Stderr io.Writer
}

func DefaultEnv() *Env {
	dir := os.Getenv("BEADS_DIR")
	if dir == "" {
		dir = ".beads"
	}
	return &Env{Dir: dir, Stdout: os.Stdout, Stderr: os.Stderr}
}

func (e *Env) dbPath() string { return filepath.Join(e.Dir, "beads.db") }

func (e *Env) openStore() (*Store, error) {
	if _, err := os.Stat(e.dbPath()); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no beads database at %s — run `bd init`", e.dbPath())
	}
	return Open(e.dbPath())
}

func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "unknown"
}

// Run dispatches a subcommand. Returns an exit code.
func Run(env *Env, args []string) int {
	if len(args) < 1 {
		printUsage(env.Stderr)
		return 2
	}
	cmd, rest := args[0], args[1:]
	var err error
	switch cmd {
	case "init":
		err = cmdInit(env, rest)
	case "create":
		err = cmdCreate(env, rest)
	case "list":
		err = cmdList(env, rest)
	case "ready":
		err = cmdReady(env, rest)
	case "show":
		err = cmdShow(env, rest)
	case "claim":
		err = cmdClaim(env, rest)
	case "close":
		err = cmdClose(env, rest)
	case "update":
		err = cmdUpdate(env, rest)
	case "dep":
		err = cmdDep(env, rest)
	case "help", "-h", "--help":
		printUsage(env.Stdout)
		return 0
	default:
		fmt.Fprintf(env.Stderr, "unknown command: %s\n", cmd)
		printUsage(env.Stderr)
		return 2
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, "error:", err)
		return 1
	}
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `bd — minimal issue tracker

Usage:
  bd init                            initialize .beads/ in current directory
  bd create "title" [-p N] [-t TYPE] create a new issue
  bd list [--status open|closed|all] list issues
  bd ready [-n N]                    list ready (unblocked, unassigned) issues
  bd show <id>                       show one issue
  bd claim [<id>] [--as NAME]        claim next ready issue (or specific id)
  bd close <id>                      close an issue
  bd update <id> [flags]             update fields: -p N, --status S, --assignee A, --title T
  bd dep add <child> <parent>        add a dependency edge
  bd dep rm  <child> <parent>        remove a dependency edge

Environment:
  BEADS_DIR    override .beads/ directory
`)
}

func cmdInit(env *Env, _ []string) error {
	if err := os.MkdirAll(env.Dir, 0o755); err != nil {
		return err
	}
	s, err := Open(env.dbPath())
	if err != nil {
		return err
	}
	defer s.Close()
	fmt.Fprintf(env.Stdout, "initialized %s\n", env.dbPath())
	return nil
}

func cmdCreate(env *Env, args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	priority := fs.Int("p", 2, "priority (0=highest)")
	typ := fs.String("t", "task", "issue type")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("title required")
	}
	title := strings.Join(fs.Args(), " ")
	s, err := env.openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	i, err := s.Create(title, *typ, *priority)
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, i.ID)
	return nil
}

func cmdList(env *Env, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	status := fs.String("status", "open", "filter by status: open|in_progress|closed|all")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := env.openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	filter := *status
	if filter == "all" {
		filter = ""
	}
	issues, err := s.List(filter)
	if err != nil {
		return err
	}
	printIssues(env.Stdout, issues)
	return nil
}

func cmdReady(env *Env, args []string) error {
	fs := flag.NewFlagSet("ready", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	limit := fs.Int("n", 20, "max issues")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := env.openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	issues, err := s.Ready(*limit)
	if err != nil {
		return err
	}
	printIssues(env.Stdout, issues)
	return nil
}

func cmdShow(env *Env, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: bd show <id>")
	}
	s, err := env.openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	i, err := s.Get(args[0])
	if err != nil {
		return err
	}
	parents, blocks, err := s.Deps(i.ID)
	if err != nil {
		return err
	}
	w := env.Stdout
	fmt.Fprintf(w, "ID:       %s\n", i.ID)
	fmt.Fprintf(w, "Title:    %s\n", i.Title)
	fmt.Fprintf(w, "Type:     %s\n", i.Type)
	fmt.Fprintf(w, "Status:   %s\n", i.Status)
	fmt.Fprintf(w, "Priority: %d\n", i.Priority)
	if i.Assignee.Valid {
		fmt.Fprintf(w, "Assignee: %s\n", i.Assignee.String)
	}
	fmt.Fprintf(w, "Created:  %s\n", time.Unix(i.Created, 0).Format(time.RFC3339))
	fmt.Fprintf(w, "Updated:  %s\n", time.Unix(i.Updated, 0).Format(time.RFC3339))
	if i.Closed.Valid {
		fmt.Fprintf(w, "Closed:   %s\n", time.Unix(i.Closed.Int64, 0).Format(time.RFC3339))
	}
	if len(parents) > 0 {
		fmt.Fprintf(w, "Depends:  %s\n", strings.Join(parents, ", "))
	}
	if len(blocks) > 0 {
		fmt.Fprintf(w, "Blocks:   %s\n", strings.Join(blocks, ", "))
	}
	return nil
}

func cmdClaim(env *Env, args []string) error {
	fs := flag.NewFlagSet("claim", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	as := fs.String("as", currentUser(), "assignee name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := env.openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	var i Issue
	if fs.NArg() >= 1 {
		i, err = s.ClaimByID(fs.Arg(0), *as)
	} else {
		i, err = s.Claim(*as)
		if errors.Is(err, ErrNotFound) {
			return errors.New("no ready issues")
		}
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "claimed %s (%s)\n", i.ID, i.Title)
	return nil
}

func cmdClose(env *Env, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: bd close <id>")
	}
	s, err := env.openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	i, err := s.Close_(args[0], "")
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "closed %s\n", i.ID)
	return nil
}

func cmdUpdate(env *Env, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: bd update <id> [flags]")
	}
	id := args[0]
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	priority := fs.Int("p", -1, "priority (0=highest); -1 = unchanged")
	status := fs.String("status", "", "new status")
	assignee := fs.String("assignee", "", "set assignee (use empty string with --unassign to clear)")
	unassign := fs.Bool("unassign", false, "clear assignee")
	title := fs.String("title", "", "new title")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	s, err := env.openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	var f UpdateFields
	if *priority >= 0 {
		f.Priority = priority
	}
	if *status != "" {
		f.Status = status
	}
	if *title != "" {
		f.Title = title
	}
	switch {
	case *unassign:
		ns := sql.NullString{}
		f.Assignee = &ns
	case *assignee != "":
		ns := sql.NullString{String: *assignee, Valid: true}
		f.Assignee = &ns
	}
	i, err := s.Update(id, f)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "updated %s\n", i.ID)
	return nil
}

func cmdDep(env *Env, args []string) error {
	if len(args) < 3 {
		return errors.New("usage: bd dep add|rm <child> <parent>")
	}
	op, child, parent := args[0], args[1], args[2]
	s, err := env.openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	switch op {
	case "add":
		if err := s.AddDep(child, parent); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "%s depends on %s\n", child, parent)
	case "rm", "remove":
		if err := s.RemoveDep(child, parent); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "removed %s -> %s\n", child, parent)
	default:
		return fmt.Errorf("unknown dep op: %s", op)
	}
	return nil
}

func printIssues(w io.Writer, issues []Issue) {
	if len(issues) == 0 {
		fmt.Fprintln(w, "(none)")
		return
	}
	for _, i := range issues {
		assignee := "-"
		if i.Assignee.Valid {
			assignee = i.Assignee.String
		}
		fmt.Fprintf(w, "%s  p%d  %-12s  %-10s  %s\n", i.ID, i.Priority, i.Status, assignee, i.Title)
	}
}
