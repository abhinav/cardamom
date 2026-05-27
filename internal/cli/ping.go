package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rovak/clu/internal/store"
)

// PingCmd writes a fire-and-forget mailbox row addressed to one
// agent. Distinct from `clu comment add`: doesn't attach to an issue,
// auto-expires, doesn't pollute the work log. The body can come from
// positional args, '-', or piped stdin (same convention as comment add).
type PingCmd struct {
	Recipient string        `arg:"" name:"recipient" help:"Agent name to ping."`
	Body      []string      `arg:"" optional:"" help:"Message body. Use '-' or omit (with piped stdin) to read from stdin."`
	Agent     string        `short:"a" name:"agent" default:"${user}" help:"Sender identity (defaults to $USER)."`
	TTL       time.Duration `name:"ttl" default:"168h" help:"How long the ping lives in the recipient's mailbox before auto-purge. Capped at 720h (30 days)."`
}

func (c *PingCmd) Run(r *runCtx) error {
	body, err := readBody(c.Body)
	if err != nil {
		return err
	}
	if body == "" {
		return errors.New("ping body required (positional args, '-', or pipe via stdin)")
	}
	return withStore(r, func(s *store.Store) error {
		m, err := s.PingSend(r.ctx, c.Agent, c.Recipient, body, c.TTL)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(m)
		}
		r.notice("pinged %s as %s (#%d, expires %s)\n",
			m.Recipient, m.Sender, m.ID,
			time.Unix(m.Expires, 0).Format(time.RFC3339))
		return nil
	})
}

// InboxCmd reads the mailbox addressed to one agent. By default lists
// unread pings and marks them read on the way out.
//
//	--peek     read without marking as read
//	--all      include already-read pings (still bounded by TTL)
//	--clear    mark every unread as read without listing
//	--watch    continuous emission as new pings arrive
type InboxCmd struct {
	Agent    string        `short:"a" name:"agent" default:"${user}" help:"Whose inbox to read (defaults to $USER). Ignored when --global is set."`
	Global   bool          `short:"A" name:"global" help:"Tail/list every recipient's pings (system-wide). Never marks as read; --peek is implied."`
	All      bool          `name:"all" help:"Include already-read pings (default: unread only)."`
	Since    time.Duration `name:"since" default:"0" help:"Only pings newer than this (e.g. 1h). 0 = no floor."`
	Peek     bool          `name:"peek" help:"Read without marking as read. Default: listing marks them."`
	Clear    bool          `name:"clear" help:"Mark every unread ping as read without listing them. Mutually exclusive with --peek and --watch."`
	Watch    bool          `short:"w" name:"watch" help:"Keep emitting as new pings arrive; redraws the screen on each tick. Ctrl+C to exit."`
	Tail     bool          `name:"tail" help:"Like --watch, but append-only: each new ping prints on its own line without redrawing. Closer to 'tail -f'. Ctrl+C to exit."`
	Interval time.Duration `name:"interval" default:"1s" help:"Poll interval when --watch or --tail is set."`
	Limit    int           `short:"n" default:"50" help:"Max rows to list (0 = unlimited)."`
}

func (c *InboxCmd) Run(r *runCtx) error {
	if c.Global && c.Clear {
		return errors.New("--global is read-only across all inboxes; --clear is per-recipient")
	}
	if c.Clear && (c.Peek || c.Watch || c.Tail) {
		return errors.New("--clear is mutually exclusive with --peek, --watch, and --tail")
	}
	if c.Watch && c.Tail {
		return errors.New("--watch and --tail are mutually exclusive (they render differently)")
	}
	if c.Tail && c.Peek {
		return errors.New("--tail and --peek are mutually exclusive (peek would re-emit the same pings forever)")
	}
	if c.Clear {
		return c.runClear(r)
	}
	if c.Watch {
		if r.json {
			return errors.New("--watch is not supported with --json (JSON output is a single document)")
		}
		return c.runWatch(r)
	}
	if c.Tail {
		if r.json {
			return errors.New("--tail is not supported with --json (use --watch + a downstream JSON consumer instead)")
		}
		return c.runTail(r)
	}
	return c.runOnce(r)
}

// runClear dismisses every unread ping for the configured agent.
func (c *InboxCmd) runClear(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		n, err := s.InboxClear(r.ctx, c.Agent)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(map[string]any{"cleared": n})
		}
		r.notice("cleared %d unread ping(s) for %s\n", n, c.Agent)
		return nil
	})
}

// runOnce lists once and exits. Marks read unless --peek. In --global
// mode, never marks read (the caller isn't the recipient, by definition).
func (c *InboxCmd) runOnce(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		var sinceTs int64
		if c.Since > 0 {
			sinceTs = time.Now().Add(-c.Since).Unix()
		}
		rows, err := c.fetch(r, s, sinceTs)
		if err != nil {
			return err
		}
		printInboxScoped(r, rows, c.Global)
		if c.Global || c.Peek || len(rows) == 0 {
			return nil
		}
		ids := make([]int64, 0, len(rows))
		for _, m := range rows {
			if m.ReadAt == nil {
				ids = append(ids, m.ID)
			}
		}
		if _, err := s.PingMarkRead(r.ctx, c.Agent, ids); err != nil {
			return err
		}
		return nil
	})
}

// fetch is the read-side helper shared by all modes. Branches on
// --global so the recipient filter only applies when scoped.
func (c *InboxCmd) fetch(r *runCtx, s *store.Store, sinceTs int64) ([]store.Mailbox, error) {
	if c.Global {
		return s.InboxAll(r.ctx, c.All, sinceTs, c.Limit)
	}
	return s.Inbox(r.ctx, c.Agent, c.All, sinceTs, c.Limit)
}

// runWatch streams new pings as they arrive. Uses the shared
// watchLoop so unchanged ticks are silent. Each render reads the
// current unread set; consumed pings stop appearing once marked.
func (c *InboxCmd) runWatch(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		interval := c.Interval
		if interval < time.Second {
			interval = time.Second
		}
		return watchLoop(r.ctx, r.stdout, interval, func() (string, error) {
			var sinceTs int64
			if c.Since > 0 {
				sinceTs = time.Now().Add(-c.Since).Unix()
			}
			rows, err := c.fetch(r, s, sinceTs)
			if err != nil {
				return "", err
			}
			// Mark-as-read between renders so the same unread pings
			// don't keep showing up tick after tick. --peek opts out,
			// and --global is implicitly peek (the watcher isn't the
			// addressed recipient).
			if !c.Global && !c.Peek && len(rows) > 0 {
				ids := make([]int64, 0, len(rows))
				for _, m := range rows {
					if m.ReadAt == nil {
						ids = append(ids, m.ID)
					}
				}
				if _, err := s.PingMarkRead(r.ctx, c.Agent, ids); err != nil {
					return "", err
				}
			}
			var buf bytes.Buffer
			sub := *r
			sub.stdout = &buf
			printInboxScoped(&sub, rows, c.Global)
			return buf.String(), nil
		})
	})
}

// runTail streams new pings as one-liners, append-only — no screen
// redraw. Closer in spirit to `tail -f` than `watch`: you can pipe it
// into a logger, scroll back through the buffer, and the rendering
// doesn't fight with anything else printing to the same terminal.
//
// Each tick reads the *unread* set, marks it read, and prints one line
// per ping. Silent on ticks with nothing new.
func (c *InboxCmd) runTail(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		interval := c.Interval
		if interval < time.Second {
			interval = time.Second
		}
		tick := time.NewTicker(interval)
		defer tick.Stop()
		// lastSeenID is only consulted in --global mode (the per-
		// recipient path uses the read_at flag to dedupe instead).
		var lastSeenID int64
		// First tick fires immediately so the user gets backlog they
		// already have unread, then we settle into the interval.
		emit := func() error {
			var sinceTs int64
			if c.Since > 0 {
				sinceTs = time.Now().Add(-c.Since).Unix()
			}
			// Tail is always "unread only" — including all (--all) would
			// re-emit history every tick. In --global mode we can't
			// rely on the unread bit (we don't mark anything as read),
			// so we ratchet on the highest ID we've already seen
			// instead. That gives true "new-only" behavior across
			// every recipient.
			var rows []store.Mailbox
			var err error
			if c.Global {
				rows, err = s.InboxAll(r.ctx, true, sinceTs, c.Limit)
			} else {
				rows, err = s.Inbox(r.ctx, c.Agent, false, sinceTs, c.Limit)
			}
			if err != nil {
				return err
			}
			// InboxAll returns newest-first; we want chronological for
			// a tail. Filter to new-since-last-tick when global, then
			// reverse.
			if c.Global {
				var fresh []store.Mailbox
				for _, m := range rows {
					if m.ID > lastSeenID {
						fresh = append(fresh, m)
					}
				}
				rows = fresh
			}
			if len(rows) == 0 {
				return nil
			}
			// Print oldest-first so the stream reads top-to-bottom.
			for i := len(rows) - 1; i >= 0; i-- {
				m := rows[i]
				ts := time.Unix(m.Created, 0).Format("15:04:05")
				fmt.Fprintf(r.stdout, "[%s] %s → %s: %s\n", ts, m.Sender, m.Recipient, m.Body)
				if m.ID > lastSeenID {
					lastSeenID = m.ID
				}
			}
			if c.Global {
				return nil
			}
			ids := make([]int64, 0, len(rows))
			for _, m := range rows {
				if m.ReadAt == nil {
					ids = append(ids, m.ID)
				}
			}
			if _, err := s.PingMarkRead(r.ctx, c.Agent, ids); err != nil {
				return err
			}
			return nil
		}
		if err := emit(); err != nil {
			return err
		}
		for {
			select {
			case <-r.ctx.Done():
				return r.ctx.Err()
			case <-tick.C:
				if err := emit(); err != nil {
					return err
				}
			}
		}
	})
}

// printInbox renders rows in human or JSON form. Always emits an array
// in JSON (empty inbox = `[]`).
func printInbox(r *runCtx, rows []store.Mailbox) {
	printInboxScoped(r, rows, false)
}

// printInboxScoped is the common renderer. When `showRecipient` is true
// each row also shows who the ping was addressed to — the missing bit
// of context in --global mode where the reader isn't the recipient.
func printInboxScoped(r *runCtx, rows []store.Mailbox, showRecipient bool) {
	if r.json {
		if rows == nil {
			rows = []store.Mailbox{}
		}
		_ = r.emitJSON(rows)
		return
	}
	if len(rows) == 0 {
		fmt.Fprintln(r.stdout, "(no pings)")
		return
	}
	now := time.Now().Unix()
	for _, m := range rows {
		when := relTime(now - m.Created)
		read := ""
		if m.ReadAt != nil {
			read = " (read)"
		}
		if showRecipient {
			fmt.Fprintf(r.stdout, "[#%d] %s → %s, %s ago%s\n", m.ID, m.Sender, m.Recipient, when, read)
		} else {
			fmt.Fprintf(r.stdout, "[#%d] from %s, %s ago%s\n", m.ID, m.Sender, when, read)
		}
		for _, line := range strings.Split(m.Body, "\n") {
			fmt.Fprintf(r.stdout, "  %s\n", line)
		}
	}
}

// relTime renders a duration-in-seconds compactly: "12s", "4m", "2h",
// "3d". Same shape used elsewhere in clu output.
func relTime(sec int64) string {
	switch {
	case sec < 60:
		return fmt.Sprintf("%ds", sec)
	case sec < 3600:
		return fmt.Sprintf("%dm", sec/60)
	case sec < 86400:
		return fmt.Sprintf("%dh", sec/3600)
	default:
		return fmt.Sprintf("%dd", sec/86400)
	}
}
