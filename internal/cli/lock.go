package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/rovak/clu/internal/store"
)

// LockCmd implements `clu lock <name> [-- cmd args...]`: acquire a
// named lock, optionally run a command while holding it.
//
// Every acquire requires a finite --ttl so a crashed holder can't
// wedge the lock forever. The trailing-command form is the preferred
// shape — clu owns acquire+release so leakage is impossible:
//
//	clu lock deploy --ttl 1h -- ./deploy.sh production
//
// The bare form exists for cases where the work spans multiple shell
// invocations; pair it with `clu unlock`.
type LockCmd struct {
	Name        string        `arg:"" required:"" help:"Lock name (e.g. deploy, build, test-db)."`
	Cmd         []string      `arg:"" optional:"" passthrough:"" help:"Optional command to run while holding the lock. Pass after '--'."`
	Agent       string        `short:"a" name:"agent" default:"${user}" help:"Holder identity (defaults to $USER)."`
	TTL         time.Duration `name:"ttl" default:"5m" help:"How long the lock lives if not explicitly released. Crashed holders auto-expire after this."`
	Wait        bool          `name:"wait" help:"Block until the lock becomes free, then acquire."`
	WaitTimeout time.Duration `name:"wait-timeout" default:"0" help:"Give up waiting after this (0 = forever). Only meaningful with --wait."`
	Interval    time.Duration `name:"interval" default:"500ms" help:"Poll interval when --wait is set."`
}

func (c *LockCmd) Run(r *runCtx) error {
	if c.TTL <= 0 {
		return errors.New("--ttl must be > 0")
	}
	host, _ := os.Hostname()
	pid := os.Getpid()

	return withStore(r, func(s *store.Store) error {
		acquired, err := acquireLock(r, s, c.Name, c.Agent, pid, host, c.TTL, c.Wait, c.WaitTimeout, c.Interval)
		if err != nil {
			return err
		}

		// Kong's passthrough keeps the literal `--` separator as the
		// first arg. Strip it so exec sees the command directly.
		cmdArgs := c.Cmd
		if len(cmdArgs) > 0 && cmdArgs[0] == "--" {
			cmdArgs = cmdArgs[1:]
		}
		if len(cmdArgs) == 0 {
			r.notice("acquired lock %q for %s until %s\n",
				acquired.Name, acquired.Holder,
				time.Unix(acquired.Expires, 0).Format(time.RFC3339))
			if r.json {
				return r.emitJSON(acquired)
			}
			return nil
		}

		// Trailing-command form: run the command, release on exit.
		// Errors from the command are propagated as the process exit
		// code; the lock is released either way.
		r.notice("acquired lock %q; running: %v\n", acquired.Name, cmdArgs)
		defer func() {
			if err := s.LockRelease(r.ctx, c.Name, c.Agent); err != nil && !errors.Is(err, store.ErrLockNotFound) {
				fmt.Fprintf(r.stderr, "warning: failed to release lock %q: %v\n", c.Name, err)
				return
			}
			r.notice("released lock %q\n", c.Name)
		}()

		cmd := exec.CommandContext(r.ctx, cmdArgs[0], cmdArgs[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = r.stdout
		cmd.Stderr = r.stderr
		return cmd.Run()
	})
}

// acquireLock encapsulates the wait/retry logic so we can share it
// between bare and --while modes.
func acquireLock(r *runCtx, s *store.Store, name, holder string, pid int, host string, ttl time.Duration, wait bool, waitTimeout, interval time.Duration) (store.Lock, error) {
	deadline := time.Time{}
	if wait && waitTimeout > 0 {
		deadline = time.Now().Add(waitTimeout)
	}
	for {
		got, err := s.LockAcquire(r.ctx, name, holder, pid, host, ttl)
		if err == nil {
			return got, nil
		}
		if !errors.Is(err, store.ErrLockHeld) {
			return store.Lock{}, err
		}
		if !wait {
			return store.Lock{}, fmt.Errorf("%w by %s until %s", err, got.Holder,
				time.Unix(got.Expires, 0).Format(time.RFC3339))
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return store.Lock{}, fmt.Errorf("timed out waiting for lock %q (held by %s)", name, got.Holder)
		}
		select {
		case <-r.ctx.Done():
			return store.Lock{}, r.ctx.Err()
		case <-time.After(interval):
		}
	}
}

// UnlockCmd implements `clu unlock <name>`.
type UnlockCmd struct {
	Name  string `arg:"" required:"" help:"Lock name to release."`
	Agent string `short:"a" name:"agent" default:"${user}" help:"Caller identity. Must match the current holder (use --force to release someone else's lock)."`
	Force bool   `name:"force" help:"Release the lock even if held by another holder. Use to clear a stuck lock after a crash."`
}

func (c *UnlockCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		holder := c.Agent
		if c.Force {
			// LockRelease normally checks holder; with --force we look
			// up the row, then bypass by passing the actual holder.
			cur, err := s.LockGet(r.ctx, c.Name)
			if err != nil {
				return err
			}
			holder = cur.Holder
		}
		if err := s.LockRelease(r.ctx, c.Name, holder); err != nil {
			return err
		}
		r.notice("released lock %q\n", c.Name)
		return nil
	})
}

// LocksCmd implements `clu locks`: list current locks with live/stale
// annotations.
type LocksCmd struct{}

func (c *LocksCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		locks, err := s.LockList(r.ctx)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(locks)
		}
		if len(locks) == 0 {
			fmt.Fprintln(r.stdout, "(no locks)")
			return nil
		}
		t := time.Now().Unix()
		for _, l := range locks {
			state := "🔒 live"
			ttl := time.Duration(l.Expires-t) * time.Second
			if l.Expires <= t {
				state = "💀 stale"
				ttl = time.Duration(t-l.Expires) * time.Second
				ttl = -ttl
			}
			fmt.Fprintf(r.stdout, "%-20s  %s  held by %s (pid %d on %s)  ttl %s\n",
				l.Name, state, l.Holder, l.PID, l.Host, ttl.Round(time.Second))
		}
		return nil
	})
}
