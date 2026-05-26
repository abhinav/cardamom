package cli

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rovak/clu/internal/config"
	"github.com/rovak/clu/internal/store"
)

//go:embed AGENTS.md
var agentsManual string

// BriefCmd prints the workflow context for an agent: the agent-facing
// manual (AGENTS.md, embedded at build time), the agents declared in
// the project's config.yaml, who's currently live (heartbeat), and any
// persisted memories.
//
// Intended for `clu brief | $YOUR_AGENT_RUNTIME` at session start so a
// fresh agent loads the right context in one shot.
type BriefCmd struct{}

type briefJSON struct {
	Manual      string            `json:"manual"`
	Agents      []briefAgentJSON  `json:"agents"`
	Active      []briefActiveJSON `json:"active"`
	UnreadPings int               `json:"unread_pings"`
	Memories    []briefMemoryJSON `json:"memories"`
}

type briefAgentJSON struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type briefActiveJSON struct {
	Name         string   `json:"name"`
	PID          int      `json:"pid"`
	Host         string   `json:"host"`
	StartedAt    int64    `json:"started_at"`
	LastSeen     int64    `json:"last_seen"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// briefMemoryJSON is a placeholder for the persisted-memory feature.
// Empty list until that lands.
type briefMemoryJSON struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (c *BriefCmd) Run(r *runCtx) error {
	cfg, _ := config.Load(r.dir) // config absence is fine

	// Collect declared agents from config.yaml.
	declared := make([]briefAgentJSON, 0, len(cfg.Agents))
	for name, a := range cfg.Agents {
		declared = append(declared, briefAgentJSON{
			Name:         name,
			Description:  a.Description,
			Capabilities: append([]string(nil), a.Capabilities...),
		})
	}
	sort.Slice(declared, func(i, j int) bool { return declared[i].Name < declared[j].Name })

	// Collect active agents + unread ping count from the DB if it
	// exists. DB absence is fine — `clu brief` works pre-init for
	// bootstrap.
	var active []briefActiveJSON
	unread := 0
	s, err := r.openStore()
	if err == nil {
		defer s.Close()
		rows, err := s.AgentList(r.ctx, store.AgentStaleThresholdSec)
		if err == nil {
			for _, row := range rows {
				active = append(active, briefActiveJSON{
					Name:         row.Name,
					PID:          row.PID,
					Host:         row.Host,
					StartedAt:    row.StartedAt,
					LastSeen:     row.LastSeen,
					Capabilities: row.DecodeCapabilities(),
				})
			}
		}
		// Surface unread mailbox count for the calling identity ($USER).
		// Cheap query and a high-value heads-up at session start.
		if n, err := s.InboxUnreadCount(r.ctx, currentUser()); err == nil {
			unread = n
		}
	}

	if r.json {
		return r.emitJSON(briefJSON{
			Manual:      agentsManual,
			Agents:      declared,
			Active:      active,
			UnreadPings: unread,
			Memories:    []briefMemoryJSON{},
		})
	}

	// Human output: the manual verbatim, then the project-specific sections.
	fmt.Fprint(r.stdout, agentsManual)
	if !strings.HasSuffix(agentsManual, "\n") {
		fmt.Fprintln(r.stdout)
	}
	fmt.Fprintln(r.stdout, "---")
	fmt.Fprintln(r.stdout)
	fmt.Fprintln(r.stdout, "## This project's agents")
	fmt.Fprintln(r.stdout)
	if len(declared) == 0 {
		fmt.Fprintln(r.stdout, "_No agents declared in `.clu/config.yaml`. Add some under the `agents:` key to make them discoverable._")
	} else {
		for _, a := range declared {
			fmt.Fprintf(r.stdout, "- **%s**", a.Name)
			if a.Description != "" {
				fmt.Fprintf(r.stdout, " — %s", a.Description)
			}
			fmt.Fprintln(r.stdout)
			if len(a.Capabilities) > 0 {
				fmt.Fprintf(r.stdout, "    capabilities: %s\n", strings.Join(a.Capabilities, ", "))
			}
		}
	}
	fmt.Fprintln(r.stdout)
	fmt.Fprintln(r.stdout, "## Currently active")
	fmt.Fprintln(r.stdout)
	if len(active) == 0 {
		fmt.Fprintln(r.stdout, "_No agents currently heartbeating._")
	} else {
		for _, a := range active {
			fmt.Fprintf(r.stdout, "- **%s** (pid %d on %s, last seen %ds ago)\n",
				a.Name, a.PID, a.Host, time.Now().Unix()-a.LastSeen)
		}
	}
	fmt.Fprintln(r.stdout)
	fmt.Fprintln(r.stdout, "## Your mailbox")
	fmt.Fprintln(r.stdout)
	if unread == 0 {
		fmt.Fprintln(r.stdout, "_No unread pings._")
	} else {
		fmt.Fprintf(r.stdout, "**%d unread ping(s).** Read with `clu inbox` (marks them read), or `clu inbox --peek` to see without consuming.\n", unread)
	}
	fmt.Fprintln(r.stdout)
	fmt.Fprintln(r.stdout, "## Persisted memories")
	fmt.Fprintln(r.stdout)
	fmt.Fprintln(r.stdout, "_Memory system not yet implemented._")
	return nil
}
