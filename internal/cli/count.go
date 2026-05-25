package cli

import (
	"encoding/json"
	"fmt"

	"github.com/rovak/beadsv2/internal/store"
)

type CountCmd struct {
	listFilterFlags
}

func (c *CountCmd) Run(r *runCtx) error {
	f, err := c.listFilterFlags.toFilter()
	if err != nil {
		return err
	}
	return withStore(r, func(s *store.Store) error {
		n, err := s.Count(r.ctx, f)
		if err != nil {
			return err
		}
		if r.json {
			return json.NewEncoder(r.stdout).Encode(map[string]int{"count": n})
		}
		fmt.Fprintln(r.stdout, n)
		return nil
	})
}
