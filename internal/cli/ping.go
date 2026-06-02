package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Rovak/agents-clu/internal/store"
)

var (
	osGetpid   = os.Getpid
	osHostname = os.Hostname
)

// PingCmd writes a fire-and-forget mailbox row to `Recipient`. Direct
// delivery always lands in `Recipient`'s mailbox; the store also fans
// out to every active subscription whose pattern matches the
// recipient name (see `clu inbox --topic`). The body can come from
// positional args, '-', or piped stdin (same convention as comment add).
//
// Topic-style names are conventional: `release.urgent`, `frontend.build-broken`.
// They're just strings — a listener subscribed to `release.*` picks
// them up, otherwise the row sits in the literal-name inbox until TTL.
//
// Distinct from `clu comment add`: doesn't attach to an issue,
// auto-expires, doesn't pollute the work log.
type PingCmd struct {
	Recipient string        `arg:"" name:"recipient" help:"Agent name or topic. Direct delivery + fan-out to matching --topic subscribers."`
	Body      []string      `arg:"" optional:"" help:"Message body. Use '-' or omit (with piped stdin) to read from stdin."`
	Agent     string        `short:"a" name:"agent" default:"${user}" help:"Sender identity (defaults to $USER)."`
	TTL       time.Duration `name:"ttl" default:"168h" help:"How long each delivered row lives in the recipient's mailbox before auto-purge. Capped at 720h (30 days)."`
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
		sent, err := s.PingSend(r.ctx, c.Agent, c.Recipient, body, c.TTL)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(sent)
		}
		// Single delivery (no subscription fan-out): keep the original
		// tight one-liner. Otherwise summarize.
		if len(sent) == 1 {
			m := sent[0]
			r.notice("pinged %s as %s (#%d, expires %s)\n",
				m.Recipient, m.Sender, m.ID,
				time.Unix(m.Expires, 0).Format(time.RFC3339))
			return nil
		}
		names := make([]string, len(sent))
		for i, m := range sent {
			names[i] = m.Recipient
		}
		r.notice("pinged %s + %d subscriber(s): %s\n",
			c.Recipient, len(sent)-1, strings.Join(names, ", "))
		return nil
	})
}

// InboxCmd reads the mailbox addressed to one agent. By default lists
// unread pings and marks them read on the way out. Also manages topic
// subscriptions — when --topic is set with --watch/--tail, the
// subscription is refreshed every tick and lives for the duration of
// the loop. Persistent subscriptions can be created by passing
// --topic without --watch/--tail (uses --topic-ttl).
type InboxCmd struct {
	Agent    string        `short:"a" name:"agent" default:"${user}" help:"Whose inbox to read (defaults to $USER). Ignored when --global is set."`
	Global   bool          `short:"A" name:"global" help:"Tail/list every recipient's pings (system-wide). Never marks as read; --peek is implied."`
	All      bool          `name:"all" help:"Include already-read pings (default: unread only)."`
	Since    string        `name:"since" help:"Only pings newer than this (e.g. 1h, 2d, 1w). Empty = no floor."`
	Peek     bool          `name:"peek" help:"Read without marking as read. Default: listing marks them."`
	Clear    bool          `name:"clear" help:"Mark every unread ping as read without listing them. Mutually exclusive with --peek and --watch."`
	Watch    bool          `short:"w" name:"watch" help:"Keep emitting as new pings arrive; redraws the screen on each tick. Ctrl+C to exit."`
	Tail     bool          `name:"tail" help:"Like --watch, but append-only: each new ping prints on its own line without redrawing. Closer to 'tail -f'. Ctrl+C to exit."`
	Interval time.Duration `name:"interval" default:"1s" help:"Poll interval when --watch or --tail is set."`
	Limit    int           `short:"n" default:"50" help:"Max rows to list (0 = unlimited)."`

	Topic    []string      `name:"topic" help:"Subscribe to a topic pattern (glob: 'release.*'). Repeatable. With --watch/--tail the subscription is refreshed every tick and dies on exit; without, it persists for --topic-ttl."`
	NoTopic  []string      `name:"no-topic" help:"Unsubscribe from a topic pattern. Repeatable."`
	TopicTTL time.Duration `name:"topic-ttl" default:"15m" help:"How long a subscription lives between refreshes. Capped at 168h."`
	Topics   bool          `name:"topics" help:"List active subscriptions (across listeners by default; -a <name> to scope to one). Doesn't read messages."`
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
	if c.Topics {
		return c.runListTopics(r)
	}
	// --no-topic runs first so a "remove" call works without also
	// needing to read messages.
	if len(c.NoTopic) > 0 {
		if err := c.runUnsubscribe(r); err != nil {
			return err
		}
		if len(c.Topic) == 0 && !c.Watch && !c.Tail {
			return nil
		}
	}
	// Persistent subscription: --topic without --watch/--tail just
	// registers the topic(s) and exits.
	if len(c.Topic) > 0 && !c.Watch && !c.Tail {
		return c.runSubscribe(r)
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

// runSubscribe registers the requested --topic patterns as persistent
// subscriptions for c.Agent. Each is idempotent (UPSERT); calling
// repeatedly just bumps the TTL.
func (c *InboxCmd) runSubscribe(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		pid, host := pidHost()
		out := make([]store.Subscription, 0, len(c.Topic))
		for _, pat := range c.Topic {
			row, err := s.SubscriptionUpsert(r.ctx, c.Agent, pat, pid, host, c.TopicTTL)
			if err != nil {
				return fmt.Errorf("subscribe %q: %w", pat, err)
			}
			out = append(out, row)
		}
		if r.json {
			return r.emitJSON(out)
		}
		for _, row := range out {
			r.notice("subscribed %s to %q (expires %s)\n",
				row.Listener, row.Pattern,
				time.Unix(row.Expires, 0).Format(time.RFC3339))
		}
		return nil
	})
}

// runUnsubscribe removes the requested --no-topic patterns for c.Agent.
// Missing rows are quiet (per-pattern notice, no error) — repeated
// "remove" calls should not fail.
func (c *InboxCmd) runUnsubscribe(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		for _, pat := range c.NoTopic {
			err := s.SubscriptionRemove(r.ctx, c.Agent, pat)
			if errors.Is(err, store.ErrSubscriptionNotFound) {
				r.notice("not subscribed: %s → %q\n", c.Agent, pat)
				continue
			}
			if err != nil {
				return fmt.Errorf("unsubscribe %q: %w", pat, err)
			}
			r.notice("unsubscribed %s from %q\n", c.Agent, pat)
		}
		return nil
	})
}

// runListTopics renders the subscription table. Scoped to one listener
// when --agent is explicitly set; otherwise system-wide.
func (c *InboxCmd) runListTopics(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		// Treat the default ($USER) as "no scope" — listing one's own
		// subscriptions is rarely what you want; the cross-listener view
		// is. Use -a explicitly to scope.
		scope := ""
		if c.Global {
			scope = "" // global is already system-wide
		}
		rows, err := s.SubscriptionList(r.ctx, scope)
		if err != nil {
			return err
		}
		if r.json {
			if rows == nil {
				rows = []store.Subscription{}
			}
			return r.emitJSON(rows)
		}
		if len(rows) == 0 {
			fmt.Fprintln(r.stdout, "(no active subscriptions)")
			return nil
		}
		nowT := time.Now().Unix()
		fmt.Fprintf(r.stdout, "%-20s  %-30s  %s\n", "LISTENER", "PATTERN", "EXPIRES IN")
		for _, row := range rows {
			fmt.Fprintf(r.stdout, "%-20s  %-30s  %s\n",
				row.Listener, row.Pattern, relTime(row.Expires-nowT))
		}
		return nil
	})
}

// pidHost returns this process's pid + hostname for the subscription
// row. Diagnostic only — the lock/subscription machinery doesn't
// enforce holder identity.
func pidHost() (int, string) {
	host, _ := osHostname()
	return osGetpid(), host
}

// refreshTopics upserts every --topic pattern for the configured agent.
// Called once at watch/tail start (so the subscription exists from
// tick 0) and once per tick (so a long-running loop keeps the row
// fresh past its TTL). Cheap when nothing's set (no-op).
func (c *InboxCmd) refreshTopics(r *runCtx, s *store.Store) error {
	if len(c.Topic) == 0 {
		return nil
	}
	pid, host := pidHost()
	for _, pat := range c.Topic {
		if _, err := s.SubscriptionUpsert(r.ctx, c.Agent, pat, pid, host, c.TopicTTL); err != nil {
			return fmt.Errorf("refresh topic %q: %w", pat, err)
		}
	}
	return nil
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
		sinceTs, err := c.sinceTs()
		if err != nil {
			return err
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

// sinceTs resolves --since to a unix epoch. Empty → 0 (no floor).
// Accepts d/w units in addition to Go's stdlib durations, matching
// the parser `clu defer` and `clu cron` use.
func (c *InboxCmd) sinceTs() (int64, error) {
	if c.Since == "" {
		return 0, nil
	}
	d, err := parseRelDuration(c.Since)
	if err != nil {
		return 0, fmt.Errorf("--since: %w", err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("--since: must be > 0")
	}
	return time.Now().Add(-d).Unix(), nil
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
		if err := c.refreshTopics(r, s); err != nil {
			return err
		}
		return watchLoop(r.ctx, r.stdout, interval, func() (string, error) {
			if err := c.refreshTopics(r, s); err != nil {
				return "", err
			}
			sinceTs, sErr := c.sinceTs()
			if sErr != nil {
				return "", sErr
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
		if err := c.refreshTopics(r, s); err != nil {
			return err
		}
		tick := time.NewTicker(interval)
		defer tick.Stop()
		// lastSeenID is only consulted in --global mode (the per-
		// recipient path uses the read_at flag to dedupe instead).
		var lastSeenID int64
		// First tick fires immediately so the user gets backlog they
		// already have unread, then we settle into the interval.
		emit := func() error {
			if err := c.refreshTopics(r, s); err != nil {
				return err
			}
			sinceTs, sErr := c.sinceTs()
			if sErr != nil {
				return sErr
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
