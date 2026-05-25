package cli

import (
	"fmt"

	"github.com/rovak/beadsv2/internal/store"
)

type DepCmd struct {
	Add DepAddCmd `cmd:"" help:"Add a dependency edge."`
	Rm  DepRmCmd  `cmd:"" aliases:"remove" help:"Remove a dependency edge."`
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
		fmt.Fprintf(r.stdout, "%s depends on %s\n", c.Child, c.Parent)
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
		fmt.Fprintf(r.stdout, "removed %s -> %s\n", c.Child, c.Parent)
		return nil
	})
}
