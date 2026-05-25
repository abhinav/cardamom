package cli

import "github.com/rovak/beadsv2/internal/store"

type ReopenCmd struct {
	ID string `arg:"" help:"Issue ID."`
}

func (c *ReopenCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		i, err := s.Reopen(r.ctx, c.ID)
		if err != nil {
			return err
		}
		r.notice("reopened %s\n", i.ID)
		return nil
	})
}
