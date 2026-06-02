package cli

import (
	"github.com/Rovak/agents-clu/internal/store"
)

type CloseCmd struct {
	IDs    []string `arg:"" required:"" name:"id" help:"One or more issue IDs to close."`
	Reason string   `name:"reason" short:"r" help:"Optional comment posted on each issue before close. Avoids the comment-then-close two-step."`
	Agent  string   `short:"a" name:"agent" default:"${user}" help:"Author for --reason comments. Defaults to $USER."`
}

func (c *CloseCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		return eachID(r, c.IDs, func(id string) (any, error) {
			// Post the reason first, while the issue is still open and
			// AddComment's validation can apply. If close then fails,
			// the comment still went on the right state of the issue.
			if c.Reason != "" {
				if _, err := s.AddComment(r.ctx, id, c.Agent, c.Reason); err != nil {
					return nil, err
				}
			}
			i, err := s.MarkClosed(r.ctx, id)
			if err != nil {
				return nil, err
			}
			r.notice("closed %s\n", i.ID)
			return i, nil
		})
	})
}
