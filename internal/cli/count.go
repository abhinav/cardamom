package cli

import (
	"fmt"

	"github.com/arjia-labs/clu/internal/store"
)

type CountCmd struct {
	listFilterFlags
}

func (c *CountCmd) Run(r *runCtx) error {
	f, err := c.toFilter()
	if err != nil {
		return err
	}
	return withStore(r, func(s *store.Store) error {
		n, err := s.Count(r.ctx, f)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(map[string]int{"count": n})
		}
		fmt.Fprintln(r.stdout, n)
		return nil
	})
}
