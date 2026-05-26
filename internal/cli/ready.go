package cli

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/rovak/clu/internal/store"
)

type ReadyCmd struct {
	N         int           `short:"n" default:"20" help:"Maximum number of issues."`
	Agent     string        `short:"a" help:"Lane to query (default: unassigned)."`
	Wait      bool          `help:"Block until at least one issue is ready, then print and exit."`
	Watch     bool          `short:"w" name:"watch" help:"Keep emitting the ready list as it changes. Ctrl+C to exit."`
	Interval  time.Duration `default:"250ms" help:"Poll interval when --wait or --watch is set."`
	Heartbeat bool          `name:"heartbeat" help:"While waiting/watching, register -a's name as a live agent."`
}

func (c *ReadyCmd) Run(r *runCtx) error {
	if c.Wait && c.Watch {
		return errors.New("--wait and --watch are mutually exclusive (wait = one-shot, watch = continuous)")
	}
	return withStore(r, func(s *store.Store) error {
		caps := resolveAgent(r.dir, c.Agent)

		// Heartbeat is opt-in. -a doubles as the agent identity.
		if (c.Wait || c.Watch) && c.Heartbeat {
			if c.Agent == "" {
				return errors.New("--heartbeat requires -a <name>")
			}
			cleanup, err := startHeartbeat(s, c.Agent, caps)
			if err != nil {
				return err
			}
			defer cleanup()
		}

		if c.Watch {
			if r.json {
				return errors.New("--watch is not supported with --json (JSON output is a single document)")
			}
			interval := c.Interval
			if interval < time.Second {
				// `ready` defaults to 250ms (good for --wait one-shot);
				// watch needs a calmer cadence so the terminal/Monitor
				// downstream doesn't churn.
				interval = time.Second
			}
			hbName := ""
			var hbCaps []string
			if c.Heartbeat {
				hbName = c.Agent
				hbCaps = caps
			}
			return watchLoop(r.ctx, r.stdout, interval, func() (string, error) {
				heartbeatTick(s, hbName, hbCaps)
				issues, err := s.Ready(r.ctx, c.N, agentPtr(c.Agent), caps)
				if err != nil {
					return "", err
				}
				labels, err := loadLabelsFor(r.ctx, s, issues)
				if err != nil {
					return "", err
				}
				var buf bytes.Buffer
				sub := *r
				sub.stdout = &buf
				if len(issues) == 0 {
					fmt.Fprintln(&buf, "(no ready issues)")
				} else {
					// Ready by definition has no open blockers — pass nil
					// so the display short-circuits the lookup.
					printIssues(&sub, issues, labels, nil)
				}
				return buf.String(), nil
			})
		}

		var (
			issues []store.Issue
			err    error
		)
		if c.Wait {
			issues, err = s.WaitReady(r.ctx, c.N, agentPtr(c.Agent), caps, c.Interval)
		} else {
			issues, err = s.Ready(r.ctx, c.N, agentPtr(c.Agent), caps)
		}
		if err != nil {
			return err
		}
		labels, err := loadLabelsFor(r.ctx, s, issues)
		if err != nil {
			return err
		}
		printIssues(r, issues, labels, nil)
		return nil
	})
}
