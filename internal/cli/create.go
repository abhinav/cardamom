package cli

import (
	"fmt"
	"strings"

	"github.com/rovak/clu/internal/config"
	"github.com/rovak/clu/internal/store"
)

type CreateCmd struct {
	Priority   int      `short:"p" default:"2" help:"Priority (0=highest)."`
	Type       string   `short:"t" default:"task" help:"Issue type."`
	Agent      string   `short:"a" help:"Assign directly to an agent lane (e.g. code-reviewer)."`
	Capability []string `name:"capability" sep:"," help:"Required capability for routing (e.g. go-review). Expands to a cap:* label. Comma-separated or repeatable."`
	Dep        []string `name:"dep" short:"d" sep:"," help:"Parent issue IDs the new issue depends on. Comma-separated or repeatable; equivalent to running 'clu link <new-id> <parent>' for each."`
	Title      []string `arg:"" required:"" help:"Issue title."`
}

func (c *CreateCmd) Run(r *runCtx) error {
	title := strings.Join(c.Title, " ")
	return withStore(r, func(s *store.Store) error {
		// Warn (don't reject) if -a names an agent not declared in
		// config.yaml. The local-only nature of this tool makes a hard
		// rejection too strict — ad-hoc lanes are legitimate — but a
		// typo'd routing that nobody picks up is the failure mode we
		// want to surface. Warning goes to stderr so it doesn't
		// pollute the ID-on-stdout contract; suppressed under --quiet.
		if c.Agent != "" && !r.quiet {
			cfg, _ := config.Load(r.dir)
			if _, ok := cfg.Agents[c.Agent]; !ok {
				fmt.Fprintf(r.stderr, "warning: agent %q is not declared in .clu/config.yaml; the issue will only be visible to claimers passing --agent %s\n", c.Agent, c.Agent)
			}
		}
		// CreateWithLinks does the insert + cap labels + dep edges in
		// one transaction. This matters for --dep: without the tx, the
		// new issue would briefly exist as a parent-less open row and
		// a concurrent `claim --watch` could grab it before the edges
		// land. With the tx, the row is never visible without its
		// edges.
		i, err := s.CreateWithLinks(r.ctx, title, c.Type, c.Priority, agentPtr(c.Agent), c.Capability, c.Dep)
		if err != nil {
			return err
		}
		if r.json {
			// Reload to include the just-added cap labels in the JSON view.
			refreshed, err := s.Get(r.ctx, i.ID)
			if err == nil {
				i = refreshed
			}
			return r.emitJSON(issueOut{Issue: i})
		}
		fmt.Fprintln(r.stdout, i.ID)
		return nil
	})
}
