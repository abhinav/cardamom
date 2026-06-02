package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Rovak/agents-clu/internal/config"
	"github.com/Rovak/agents-clu/internal/store"
)

// ---- Heartbeat helper (used by --wait / --watch loops) ----

// resolveAgent looks up `name` in the project's config.yaml. Returns
// the matched capabilities, or nil if the agent isn't declared (treated
// as ad-hoc — heartbeat with empty capabilities, no cap-label routing).
func resolveAgent(dir, name string) []string {
	if name == "" {
		return nil
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return nil
	}
	if a, ok := cfg.Agents[name]; ok {
		return append([]string(nil), a.Capabilities...) // copy
	}
	return nil
}

// startHeartbeat upserts an active_agents row for `name` and returns a
// cleanup function the caller must defer. The cleanup deletes the row
// using a background context so it still fires when the parent ctx is
// already cancelled (Ctrl-C).
//
// If name is empty, this is a no-op (returns a no-op cleanup).
func startHeartbeat(s *store.Store, name string, caps []string) (func(), error) {
	if name == "" {
		return func() {}, nil
	}
	host, _ := os.Hostname()
	pid := os.Getpid()
	if err := s.AgentTouch(context.Background(), name, pid, host, caps); err != nil {
		return func() {}, err
	}
	cleanup := func() {
		// Use Background so the row is cleared even when the wider ctx
		// was cancelled (Ctrl-C → ctx.Done before defer runs).
		_ = s.AgentRemove(context.Background(), name)
	}
	return cleanup, nil
}

// heartbeatTick re-upserts the row to advance last_seen. Called every
// poll interval. Errors are swallowed — a stale row is a soft failure
// (queries will filter it; reclaim happens on the next tick).
func heartbeatTick(s *store.Store, name string, caps []string) {
	if name == "" {
		return
	}
	host, _ := os.Hostname()
	_ = s.AgentTouch(context.Background(), name, os.Getpid(), host, caps)
}

// ---- `clu agent` commands ----

type AgentCmd struct {
	Start AgentStartCmd `cmd:"" help:"Launch a declared agent (config.yaml command + prompts), heartbeating while it runs."`
	Ls    AgentLsCmd    `cmd:"" aliases:"list" help:"List declared agents (config) with an active indicator."`
	Show  AgentShowCmd  `cmd:"" help:"Show one agent's full record."`
	Gc    AgentGcCmd    `cmd:"" help:"Forcibly drop stale active_agents rows (older than --stale-seconds)."`
}

// ---- agent ls ----

type AgentLsCmd struct {
	StaleSec int64 `name:"stale-seconds" default:"30" help:"Consider rows older than this dead."`
}

// agentLsRow is what we print / emit. Combines the declarative side
// (config.yaml) with the live side (active_agents heartbeat).
type agentLsRow struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Active       bool     `json:"active"`
	PID          int      `json:"pid,omitempty"`
	Host         string   `json:"host,omitempty"`
	LastSeen     int64    `json:"last_seen,omitempty"`
}

func (c *AgentLsCmd) Run(r *runCtx) error {
	cfg, err := config.Load(r.dir)
	if err != nil {
		return err
	}
	return withStore(r, func(s *store.Store) error {
		active, err := s.AgentList(r.ctx, c.StaleSec)
		if err != nil {
			return err
		}
		// Merge config + live state by name. An agent in active but not
		// in config is "ad-hoc"; include it so coordinators can see it.
		liveByName := map[string]store.ActiveAgent{}
		for _, a := range active {
			liveByName[a.Name] = a
		}
		// Build the merged set of names.
		nameSet := map[string]struct{}{}
		for n := range cfg.Agents {
			nameSet[n] = struct{}{}
		}
		for n := range liveByName {
			nameSet[n] = struct{}{}
		}
		names := make([]string, 0, len(nameSet))
		for n := range nameSet {
			names = append(names, n)
		}
		sort.Strings(names)

		rows := make([]agentLsRow, 0, len(names))
		for _, n := range names {
			row := agentLsRow{Name: n}
			if a, ok := cfg.Agents[n]; ok {
				row.Description = a.Description
				row.Capabilities = a.Capabilities
			}
			if live, ok := liveByName[n]; ok {
				row.Active = true
				row.PID = live.PID
				row.Host = live.Host
				row.LastSeen = live.LastSeen
				if len(row.Capabilities) == 0 {
					// Ad-hoc agent: surface the caps it advertised at heartbeat.
					row.Capabilities = live.DecodeCapabilities()
				}
			}
			rows = append(rows, row)
		}
		if r.json {
			return r.emitJSON(rows)
		}
		if len(rows) == 0 {
			fmt.Fprintln(r.stdout, "(no agents declared and none active)")
			return nil
		}
		// Header.
		fmt.Fprintf(r.stdout, "%-20s %-7s %-40s %s\n", "NAME", "ACTIVE", "CAPABILITIES", "DESCRIPTION")
		for _, row := range rows {
			active := " "
			if row.Active {
				active = "●"
			}
			caps := strings.Join(row.Capabilities, ", ")
			if caps == "" {
				caps = "-"
			}
			desc := row.Description
			if desc == "" {
				desc = "-"
			}
			fmt.Fprintf(r.stdout, "%-20s %-7s %-40s %s\n", row.Name, active, caps, desc)
		}
		return nil
	})
}

// ---- agent show ----

type AgentShowCmd struct {
	Name string `arg:"" help:"Agent name."`
}

type agentShowOut struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	Capabilities []string       `json:"capabilities,omitempty"`
	Active       *agentLiveInfo `json:"active,omitempty"`
	PendingWork  int            `json:"pending_work"`
}

type agentLiveInfo struct {
	PID       int    `json:"pid"`
	Host      string `json:"host"`
	StartedAt int64  `json:"started_at"`
	LastSeen  int64  `json:"last_seen"`
}

func (c *AgentShowCmd) Run(r *runCtx) error {
	cfg, err := config.Load(r.dir)
	if err != nil {
		return err
	}
	declared, isDeclared := cfg.Agents[c.Name]
	return withStore(r, func(s *store.Store) error {
		out := agentShowOut{Name: c.Name}
		if isDeclared {
			out.Description = declared.Description
			out.Capabilities = declared.Capabilities
		}
		live, err := s.AgentGet(r.ctx, c.Name)
		if err == nil {
			out.Active = &agentLiveInfo{
				PID: live.PID, Host: live.Host,
				StartedAt: live.StartedAt, LastSeen: live.LastSeen,
			}
			if !isDeclared {
				out.Capabilities = live.DecodeCapabilities()
			}
		}
		if !isDeclared && out.Active == nil {
			return fmt.Errorf("agent %q: not declared in config.yaml and not currently active", c.Name)
		}
		// Count pending work the agent could pick up: its lane + cap-routed.
		agentName := c.Name
		pending, err := s.Ready(r.ctx, 1000, &agentName, out.Capabilities)
		if err != nil {
			return err
		}
		out.PendingWork = len(pending)
		if r.json {
			return r.emitJSON(out)
		}
		fmt.Fprintf(r.stdout, "Name:         %s\n", out.Name)
		if out.Description != "" {
			fmt.Fprintf(r.stdout, "Description:  %s\n", out.Description)
		}
		if len(out.Capabilities) > 0 {
			fmt.Fprintf(r.stdout, "Capabilities: %s\n", strings.Join(out.Capabilities, ", "))
		}
		if out.Active != nil {
			ts := time.Unix(out.Active.LastSeen, 0).Format(time.RFC3339)
			fmt.Fprintf(r.stdout, "Status:       active (pid %d on %s, last seen %s)\n",
				out.Active.PID, out.Active.Host, ts)
		} else {
			fmt.Fprintln(r.stdout, "Status:       not currently active")
		}
		fmt.Fprintf(r.stdout, "Pending work: %d issues\n", out.PendingWork)
		return nil
	})
}

// ---- agent gc ----

type AgentGcCmd struct {
	StaleSec int64 `name:"stale-seconds" default:"30" help:"Delete rows older than this."`
}

func (c *AgentGcCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		all, err := s.AgentList(r.ctx, 0) // all, including stale
		if err != nil {
			return err
		}
		cutoff := time.Now().Unix() - c.StaleSec
		var dropped int
		var droppedNames []string
		for _, a := range all {
			if a.LastSeen < cutoff {
				if err := s.AgentRemove(r.ctx, a.Name); err != nil {
					return err
				}
				dropped++
				droppedNames = append(droppedNames, a.Name)
			}
		}
		if r.json {
			return r.emitJSON(map[string]any{"dropped": dropped, "names": droppedNames})
		}
		if dropped == 0 {
			r.notice("no stale rows\n")
			return nil
		}
		r.notice("dropped %d stale row(s): %s\n", dropped, strings.Join(droppedNames, ", "))
		return nil
	})
}
