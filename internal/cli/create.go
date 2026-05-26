package cli

import (
	"fmt"
	"strings"

	"github.com/rovak/clu/internal/store"
)

type CreateCmd struct {
	Priority int      `short:"p" default:"2" help:"Priority (0=highest)."`
	Type     string   `short:"t" default:"task" help:"Issue type."`
	Agent    string   `short:"a" help:"Assign to an agent lane (e.g. code-reviewer)."`
	Title    []string `arg:"" required:"" help:"Issue title."`
}

func (c *CreateCmd) Run(r *runCtx) error {
	title := strings.Join(c.Title, " ")
	return withStore(r, func(s *store.Store) error {
		i, err := s.Create(r.ctx, title, c.Type, c.Priority, agentPtr(c.Agent))
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(issueOut{Issue: i})
		}
		fmt.Fprintln(r.stdout, i.ID)
		return nil
	})
}
