package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/Rovak/agents-clu/internal/config"
	"github.com/Rovak/agents-clu/internal/store"
)

type CreateCmd struct {
	Priority    int      `short:"p" default:"2" help:"Priority (0=highest)."`
	Type        string   `short:"t" default:"task" help:"Issue type."`
	Agent       string   `short:"a" help:"Assign directly to an agent lane (e.g. code-reviewer)."`
	Capability  []string `name:"capability" sep:"," help:"Required capability for routing (e.g. go-review). Expands to a cap:* label. Comma-separated or repeatable."`
	Dep         []string `name:"dep" short:"d" sep:"," help:"Parent issue IDs the new issue depends on. Comma-separated or repeatable; equivalent to running 'clu link <new-id> <parent>' for each."`
	Description string   `name:"description" help:"Long description. Set atomically with the issue (avoids the race between create + 'clu describe')."`
	Notes       string   `name:"notes" help:"Initial working notes. Set atomically with the issue."`
	Title       []string `arg:"" required:"" help:"Issue title."`
}

func (c *CreateCmd) Run(r *runCtx) error {
	title := strings.Join(c.Title, " ")
	// Kong's sep="," drops bare empty values silently, so the user can
	// pass `--capability ""` and never hit the loop check below. Catch
	// that case by scanning os.Args directly for the literal empty
	// form before validating the parsed slice.
	if hasEmptyFlagValue(os.Args, "--capability") {
		return fmt.Errorf("--capability: value cannot be empty")
	}
	// Enforce the same name rule that config.yaml validation uses.
	// Without this, `clu create --capability foo:bar` silently produced
	// a cap:foo:bar label that no declared agent could match.
	for _, cap := range c.Capability {
		if cap == "" {
			return fmt.Errorf("--capability: value cannot be empty")
		}
		if !config.ValidAgentOrCapName(cap) {
			return fmt.Errorf("--capability %q: must be lowercase letters/digits/dashes, starting with a letter (same rules as agent names in config.yaml)", cap)
		}
	}
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
		// CreateWithLinks does the insert + cap labels + dep edges +
		// description + notes in one transaction. Crucial for --dep
		// (no parent-less window) and also for --description (no
		// race where a watching claim grabs the issue before its
		// body is set — agents shouldn't have to do `create` then
		// `describe` as two commands).
		i, err := s.CreateWithLinks(r.ctx, title, c.Type, c.Priority, agentPtr(c.Agent), store.CreateOpts{
			Caps:        c.Capability,
			Parents:     c.Dep,
			Description: c.Description,
			Notes:       c.Notes,
		})
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
