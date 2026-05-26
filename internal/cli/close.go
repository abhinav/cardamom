package cli

import "github.com/rovak/beadsv2/internal/store"

type CloseCmd struct {
	IDs []string `arg:"" required:"" name:"id" help:"One or more issue IDs to close."`
}

func (c *CloseCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		return eachID(r, c.IDs, func(id string) (any, error) {
			i, err := s.MarkClosed(r.ctx, id)
			if err != nil {
				return nil, err
			}
			r.notice("closed %s\n", i.ID)
			return i, nil
		})
	})
}
