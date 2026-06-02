package cli

import (
	"fmt"
	"strings"

	"github.com/arjia-labs/clu/internal/store"
)

type DepCmd struct {
	Add DepAddCmd `cmd:"" help:"Add a dependency edge."`
	Rm  DepRmCmd  `cmd:"" aliases:"remove" help:"Remove a dependency edge."`
	Ls  DepLsCmd  `cmd:"" aliases:"list" help:"List dependency edges for an issue (parents it needs + children it blocks)."`
}

type DepAddCmd struct {
	Child  string `arg:"" help:"Child issue (the one that depends)."`
	Parent string `arg:"" help:"Parent issue (the blocker)."`
}

func (c *DepAddCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		if err := s.AddDep(r.ctx, c.Child, c.Parent); err != nil {
			return err
		}
		if r.json {
			// Echo the affected child issue so a pipeline can keep
			// working with its new state.
			i, err := s.Get(r.ctx, c.Child)
			if err != nil {
				return err
			}
			return r.emitJSON(issueOut{Issue: i})
		}
		r.notice("%s depends on %s\n", c.Child, c.Parent)
		return nil
	})
}

type DepRmCmd struct {
	Child  string `arg:"" help:"Child issue."`
	Parent string `arg:"" help:"Parent issue."`
}

func (c *DepRmCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		if err := s.RemoveDep(r.ctx, c.Child, c.Parent); err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(map[string]any{"child": c.Child, "parent": c.Parent, "removed": true})
		}
		r.notice("removed %s -> %s\n", c.Child, c.Parent)
		return nil
	})
}

// DepLsCmd prints the dep edges for an issue, both directions: what
// it depends on (parents) and what depends on it (children/blocks).
// Mirrors the `Depends:` / `Blocks:` lines in `clu show` but standalone
// + scriptable via --json.
type DepLsCmd struct {
	ID string `arg:"" help:"Issue ID."`
}

type depLsJSON struct {
	ID      string   `json:"id"`
	Depends []string `json:"depends_on"`
	Blocks  []string `json:"blocks"`
}

func (c *DepLsCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		// Get() validates existence — `dep ls clu-9999` errors instead
		// of silently returning empty lists.
		if _, err := s.Get(r.ctx, c.ID); err != nil {
			return err
		}
		parents, blocks, err := s.Deps(r.ctx, c.ID)
		if err != nil {
			return err
		}
		// Always emit non-nil slices so JSON consumers can iterate
		// without a nil check.
		if parents == nil {
			parents = []string{}
		}
		if blocks == nil {
			blocks = []string{}
		}
		if r.json {
			return r.emitJSON(depLsJSON{ID: c.ID, Depends: parents, Blocks: blocks})
		}
		fmt.Fprintf(r.stdout, "%s\n", c.ID)
		if len(parents) == 0 {
			fmt.Fprintln(r.stdout, "  depends on: (none)")
		} else {
			fmt.Fprintf(r.stdout, "  depends on: %s\n", strings.Join(parents, ", "))
		}
		if len(blocks) == 0 {
			fmt.Fprintln(r.stdout, "  blocks:     (none)")
		} else {
			fmt.Fprintf(r.stdout, "  blocks:     %s\n", strings.Join(blocks, ", "))
		}
		return nil
	})
}
