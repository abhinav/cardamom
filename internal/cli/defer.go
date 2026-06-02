package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Rovak/agents-clu/internal/store"
)

type DeferCmd struct {
	ID   string `arg:"" help:"Issue ID."`
	When string `arg:"" help:"When to wake up: YYYY-MM-DD, RFC3339, or relative (+6h, +2d, +1w, tomorrow)."`
}

func (c *DeferCmd) Run(r *runCtx) error {
	until, err := parseWhen(c.When, time.Now())
	if err != nil {
		return err
	}
	u := until.Unix()
	return withStore(r, func(s *store.Store) error {
		i, err := s.SetDefer(r.ctx, c.ID, &u)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(issueOut{Issue: i})
		}
		r.notice("deferred %s until %s\n", c.ID, until.Format(time.RFC3339))
		return nil
	})
}

type UndeferCmd struct {
	IDs []string `arg:"" required:"" name:"id" help:"One or more issue IDs to undefer."`
}

func (c *UndeferCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		return eachID(r, c.IDs, func(id string) (any, error) {
			// Look first so we can distinguish "wasn't deferred" from
			// the operation succeeding. SetDefer alone can't tell us
			// the before-state.
			cur, err := s.Get(r.ctx, id)
			if err != nil {
				return nil, err
			}
			if cur.DeferUntil == nil {
				return nil, store.ErrNotDeferred
			}
			i, err := s.SetDefer(r.ctx, id, nil)
			if err != nil {
				return nil, err
			}
			r.notice("undeferred %s\n", id)
			return i, nil
		})
	})
}

// parseRelDuration accepts a positive relative duration with h/d/w
// units in addition to Go's stdlib units (ns/us/ms/s/m/h). Used by
// `inbox --since`, etc., so users typing `1d` or `2w` get the answer
// they expect — same parser the defer/cron paths already accept.
func parseRelDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	// Try the stdlib first so "1h30m", "500ms", etc. work unchanged.
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	// d/w fallback. Strip an optional leading '+' for symmetry with
	// `+Nd` in defer/cron — users habitually paste either form.
	body := strings.TrimPrefix(s, "+")
	if len(body) < 2 {
		return 0, fmt.Errorf("invalid duration %q (use Nh, Nd, or Nw)", s)
	}
	unit := body[len(body)-1]
	n, err := strconv.Atoi(body[:len(body)-1])
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (use Nh, Nd, or Nw)", s)
	}
	switch unit {
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid unit %q in %q (use h, d, or w)", string(unit), s)
	}
}

// parseWhen accepts:
//   - YYYY-MM-DD (date, interpreted as 00:00 local time)
//   - RFC3339 (absolute timestamp)
//   - "+<n><unit>" where unit is h, d, w (e.g. +6h, +2d, +1w)
//   - "tomorrow" (24h from now)
func parseWhen(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "tomorrow":
		return now.Add(24 * time.Hour), nil
	}
	if strings.HasPrefix(s, "+") {
		body := s[1:]
		if len(body) < 2 {
			return time.Time{}, fmt.Errorf("invalid relative duration %q", s)
		}
		unit := body[len(body)-1]
		nStr := body[:len(body)-1]
		n, err := strconv.Atoi(nStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid relative duration %q", s)
		}
		var d time.Duration
		switch unit {
		case 'h':
			d = time.Duration(n) * time.Hour
		case 'd':
			d = time.Duration(n) * 24 * time.Hour
		case 'w':
			d = time.Duration(n) * 7 * 24 * time.Hour
		default:
			return time.Time{}, fmt.Errorf("invalid unit %q in %q (use h, d, or w)", string(unit), s)
		}
		return now.Add(d), nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q (use YYYY-MM-DD, RFC3339, +Nh/d/w, or tomorrow)", s)
}
