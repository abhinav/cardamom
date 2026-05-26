package cli

import "github.com/rovak/clu/internal/store"

// LinkCmd is sugar for `clu dep add <child> <parent>`.
type LinkCmd struct {
	Child  string `arg:"" help:"Child issue (the one that depends)."`
	Parent string `arg:"" help:"Parent issue (the blocker)."`
}

func (c *LinkCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		if err := s.AddDep(r.ctx, c.Child, c.Parent); err != nil {
			return err
		}
		r.notice("%s depends on %s\n", c.Child, c.Parent)
		return nil
	})
}
