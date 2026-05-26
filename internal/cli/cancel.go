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
	IDs    []string `arg:"" required:"" name:"id" help:"Issue IDs to cancel (cascades to dependents)."`
	Reason string   `name:"reason" short:"r" help:"Optional comment posted on each root issue before cancel."`
	Agent  string   `short:"a" name:"agent" default:"${user}" help:"Author for --reason comments. Defaults to $USER."`
}

func (c *CancelCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		// Post reason on the root issues (the ones explicitly named on
		// the CLI) before the cancel transaction. Cascade descendants
		// don't get the reason — it's about the user's decision, not
		// every node downstream of it.
		if c.Reason != "" {
			for _, id := range c.IDs {
				if _, err := s.AddComment(r.ctx, id, c.Agent, c.Reason); err != nil {
					return err
				}
			}
		}
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
