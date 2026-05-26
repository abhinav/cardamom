package cli

import (
	"fmt"

	"github.com/rovak/clu/internal/store"
)

type ReopenCmd struct {
	IDs []string `arg:"" required:"" name:"id" help:"One or more issue IDs to reopen."`
}

func (c *ReopenCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		return eachID(r, c.IDs, func(id string) (any, error) {
			i, err := s.Reopen(r.ctx, id)
			if err != nil {
				return nil, err
			}
			r.notice("reopened %s\n", i.ID)
			// Warn if the reopened issue still has non-closed parents —
			// it will be permanently blocked until you either reopen the
			// parents or remove the dep edge. Especially load-bearing
			// after a cancel-cascade reversal: cancelled parents stay
			// cancelled even if you reopen a child.
			parents, _, derr := s.Deps(r.ctx, id)
			if derr == nil && len(parents) > 0 {
				stuck := stuckParents(r, s, parents)
				if len(stuck) > 0 {
					fmt.Fprintf(r.stderr, "warning: %s has unresolved parents (%s) and will stay blocked — reopen them or `clu dep rm` the edges\n", id, joinIDs(stuck))
				}
			}
			return i, nil
		})
	})
}

// stuckParents returns the parent IDs whose status is not 'closed'
// (i.e. open, in_progress, or cancelled). A cancelled parent is the
// worst case: it never unblocks on its own.
func stuckParents(r *runCtx, s *store.Store, parents []string) []string {
	var out []string
	for _, pid := range parents {
		p, err := s.Get(r.ctx, pid)
		if err != nil {
			continue
		}
		if p.Status != "closed" {
			out = append(out, fmt.Sprintf("%s [%s]", pid, p.Status))
		}
	}
	return out
}

func joinIDs(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}
