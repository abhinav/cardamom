package cli

import (
	"fmt"

	"github.com/rovak/clu/internal/store"
)

// CancelCmd cancels one or more issues *and all their transitive
// dependents*. "Cancelled" is a distinct terminal status from "closed":
// closed = done successfully (downstream unblocks); cancelled =
// abandoned (downstream stays blocked or is also cancelled here).
//
// To cancel only the target without cascade, use
// `clu update --status cancelled <id>` instead.
type CancelCmd struct {
	IDs []string `arg:"" required:"" name:"id" help:"Issue IDs to cancel (cascades to dependents)."`
}

func (c *CancelCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		changed, err := s.Cancel(r.ctx, c.IDs)
		if err != nil {
			return err
		}
		if len(changed) == 0 {
			r.notice("nothing to cancel — all targets were already closed or cancelled\n")
			if r.json {
				return r.emitJSON([]any{})
			}
			return nil
		}
		for _, i := range changed {
			r.notice("cancelled %s — %s\n", i.ID, i.Title)
		}
		r.notice("%s\n", summarize(len(c.IDs), len(changed)))
		if r.json {
			return r.emitJSON(changed)
		}
		return nil
	})
}

func summarize(roots, total int) string {
	cascaded := total - roots
	if cascaded <= 0 {
		return fmt.Sprintf("cancelled %d issue(s)", total)
	}
	return fmt.Sprintf("cancelled %d issue(s) total (%d requested + %d dependent)", total, roots, cascaded)
}
