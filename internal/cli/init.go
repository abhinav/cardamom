package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rovak/clu/internal/config"
	"github.com/rovak/clu/internal/store"
	"github.com/rovak/clu/internal/workflow"
)

// InitCmd lays down the project skeleton:
//
//	.clu/
//	  config.yaml          # project config (id_prefix, future knobs)
//	  data.sqlite          # the database (created by the schema migrations)
//	  templates/
//	    example.yaml       # a starter workflow you can copy
//
// Each piece is created only if missing — re-running `clu init` is a
// safe no-op for an already-initialised project. The command also
// surfaces the local-state gitignore recipe so the user knows what to
// add to their repo's .gitignore.
type InitCmd struct {
	Prefix string `name:"prefix" help:"ID prefix for newly-created issues (only used on first init; ignored if config.yaml already exists)."`
}

func (c *InitCmd) Run(r *runCtx) error {
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return err
	}

	// 1. config.yaml — write only if missing.
	cfgPath := config.Path(r.dir)
	cfgExisted := true
	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
		cfgExisted = false
		cfg := config.Default()
		if c.Prefix != "" {
			cfg.IDPrefix = c.Prefix
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("--prefix: %w", err)
			}
		}
		if err := config.Write(r.dir, cfg); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if c.Prefix != "" {
		// Pre-existing config + explicit --prefix: refuse to silently
		// override. Tell the user how to change it.
		return fmt.Errorf("%s exists; edit id_prefix directly to change it", cfgPath)
	}

	// Load the (now-guaranteed-present) config so the rest of init
	// (and any subsequent commands) know the prefix.
	cfg, err := config.Load(r.dir)
	if err != nil {
		return err
	}

	// 2. data.sqlite — created on demand by Store.Open's migrations.
	dbExisted := true
	if _, err := os.Stat(r.dbPath()); errors.Is(err, os.ErrNotExist) {
		dbExisted = false
	}
	s, err := store.Open(r.dbPath(), store.WithIDPrefix(cfg.IDPrefix))
	if err != nil {
		return err
	}
	s.Close()

	// 3. templates/example.yaml — write only if missing.
	tmplDir := workflow.TemplatesPath(r.dir)
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		return err
	}
	examplePath := filepath.Join(tmplDir, "example.yaml")
	tmplExisted := true
	if _, err := os.Stat(examplePath); errors.Is(err, os.ErrNotExist) {
		tmplExisted = false
		if err := os.WriteFile(examplePath, []byte(exampleTemplate), 0o644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// Summary. Quiet mode skips this whole block via r.notice.
	switch {
	case !cfgExisted && !dbExisted:
		r.notice("initialized %s\n", r.dir)
	case cfgExisted && dbExisted && tmplExisted:
		r.notice("already initialized: %s\n", r.dir)
	default:
		// Partial: tell the user what got added.
		if !cfgExisted {
			r.notice("wrote %s\n", cfgPath)
		}
		if !dbExisted {
			r.notice("wrote %s\n", r.dbPath())
		}
		if !tmplExisted {
			r.notice("wrote %s\n", examplePath)
		}
	}
	// Local state (data.sqlite + WAL siblings) should never be committed.
	// Add the patterns to .git/info/exclude (per-clone, untracked) on a
	// fresh init so users don't have to edit a tracked .gitignore — same
	// pattern `clu worktree add` uses for .worktrees/. If the user is
	// outside a git repo, fall back to the printed hint.
	if !cfgExisted && !r.quiet && !r.json {
		base := filepath.Base(r.dir)
		patterns := []string{
			base + "/data.sqlite",
			base + "/data.sqlite-shm",
			base + "/data.sqlite-wal",
		}
		any := false
		for _, p := range patterns {
			added, err := ensureGitExclude(r, p)
			if err != nil {
				fmt.Fprintln(r.stdout, "")
				fmt.Fprintln(r.stdout, "(could not update .git/info/exclude; add these to .gitignore manually:)")
				for _, p2 := range patterns {
					fmt.Fprintln(r.stdout, "  "+p2)
				}
				any = false
				break
			}
			if added {
				any = true
			}
		}
		if any {
			fmt.Fprintf(r.stdout, "\nadded %s/data.sqlite* patterns to .git/info/exclude (per-clone; not committed)\n", base)
		}
	}
	return nil
}

// exampleTemplate is a small, valid workflow template dropped into
// .clu/templates/example.yaml on a fresh init. The format matches
// internal/workflow.Template.
const exampleTemplate = `# Example workflow. Copy this file and edit to fit your process.
# Instantiate with:
#   clu run example --var scope=auth-fix
# Inspect without running:
#   clu template show example

name: example
description: |
  Three-step workflow: investigate → implement → review.
  Review is pre-assigned to the 'code-reviewer' agent lane.

vars:
  scope:
    description: "Short tag for this run (used in titles, e.g. 'auth-fix')."
    required: true

steps:
  - id: investigate
    title: "Investigate: {{scope}}"
    type: task
    priority: 2

  - id: implement
    title: "Implement: {{scope}}"
    type: task
    priority: 1
    needs: [investigate]

  - id: review
    title: "Review: {{scope}}"
    type: task
    priority: 1
    agent: code-reviewer
    needs: [implement]
`
