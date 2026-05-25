package cli

import "github.com/rovak/beadsv2/internal/store"

type CloseCmd struct {
	ID string `arg:"" help:"Issue ID."`
}

func (c *CloseCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		i, err := s.MarkClosed(r.ctx, c.ID)
		if err != nil {
			return err
		}
		r.notice("closed %s\n", i.ID)
		return nil
	})
}
