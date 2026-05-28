package cli

import "github.com/rovak/clu/internal/store"

// PriorityCmd is sugar for `clu update <id> -p <N>`.
type PriorityCmd struct {
	ID    string `arg:"" help:"Issue ID."`
	Level int    `arg:"" help:"New priority (0=highest)."`
}

func (c *PriorityCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		i, err := s.Update(r.ctx, c.ID, store.UpdateFields{Priority: &c.Level})
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(issueOut{Issue: i})
		}
		r.notice("set %s priority to %d\n", i.ID, c.Level)
		return nil
	})
}
