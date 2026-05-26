package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rovak/clu/internal/store"
)

// CronCmd dispatches the `clu cron …` subtree.
type CronCmd struct {
	Add     CronAddCmd     `cmd:"" help:"Add a scheduled job."`
	List    CronListCmd    `cmd:"" aliases:"ls" help:"List all scheduled jobs."`
	Rm      CronRmCmd      `cmd:"" aliases:"delete,remove" help:"Delete a scheduled job."`
	Enable  CronEnableCmd  `cmd:"" help:"Enable a scheduled job."`
	Disable CronDisableCmd `cmd:"" help:"Disable a scheduled job (keeps the row, skips it on run)."`
	Run     CronRunCmd     `cmd:"" help:"Run every job whose next_run has elapsed. Invoke periodically from OS cron / launchd."`
}

// ---- Job payload (the JSON we store in cron_jobs.job) ----

// cronJob is the in-memory shape of the JSON we stuff into cron_jobs.job.
// The schema is intentionally a tagged union so we can add new kinds later
// (maintenance ops, multi-step recipes) without another migration.
type cronJob struct {
	Kind string   `json:"kind"`           // "cli" today
	Args []string `json:"args,omitempty"` // for kind="cli"
}

// ---- Schedule parsing ----

// parseSchedule turns a schedule expression into an interval. Supported:
//
//	@hourly, @daily, @weekly, @monthly  (the @ aliases)
//	+Nh / +Nd / +Nw                     (interval since last run)
//
// @monthly is approximate (30 * 24h) — we deliberately don't try to honour
// calendar months. If a user needs "first of every month" they can drive
// `clu cron run` from a more capable OS cron line.
func parseSchedule(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "@hourly":
		return time.Hour, nil
	case "@daily":
		return 24 * time.Hour, nil
	case "@weekly":
		return 7 * 24 * time.Hour, nil
	case "@monthly":
		return 30 * 24 * time.Hour, nil
	}
	if strings.HasPrefix(s, "+") && len(s) >= 3 {
		body := s[1:]
		unit := body[len(body)-1]
		n, err := strconv.Atoi(body[:len(body)-1])
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid schedule %q (use +Nh/+Nd/+Nw or @hourly/@daily/@weekly/@monthly)", s)
		}
		switch unit {
		case 'h':
			return time.Duration(n) * time.Hour, nil
		case 'd':
			return time.Duration(n) * 24 * time.Hour, nil
		case 'w':
			return time.Duration(n) * 7 * 24 * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("invalid schedule %q (use +Nh/+Nd/+Nw or @hourly/@daily/@weekly/@monthly)", s)
}

// ---- Add ----

type CronAddCmd struct {
	Name     string   `arg:"" help:"Unique job name."`
	Schedule string   `name:"schedule" required:"" help:"+Nh/+Nd/+Nw or @hourly/@daily/@weekly/@monthly."`
	Args     []string `arg:"" passthrough:"" help:"After '--': the cli args to invoke (e.g. -- create -a infra-agent 'Check CI')."`
	StartNow bool     `name:"start-now" help:"Treat next_run as now; the first execution happens on the next 'cron run'."`
}

func (c *CronAddCmd) Run(r *runCtx) error {
	// Kong's passthrough leaves the literal "--" separator at the head of
	// the captured slice. Drop it so the stored args are the bare command.
	args := c.Args
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return errors.New("missing job args; use `--` to separate them, e.g. `clu cron add foo --schedule @hourly -- create -a x \"y\"`")
	}
	interval, err := parseSchedule(c.Schedule)
	if err != nil {
		return err
	}
	// Self-recursion guard. If args[0] is "cron", running this job
	// would re-enter the scheduler — infinite loop waiting to happen.
	if args[0] == "cron" {
		return errors.New("cron jobs cannot invoke `cron` (would recurse)")
	}
	payload, err := json.Marshal(cronJob{Kind: "cli", Args: args})
	if err != nil {
		return err
	}
	next := time.Now().Add(interval).Unix()
	if c.StartNow {
		next = time.Now().Unix()
	}
	j := store.CronJob{
		Name:     c.Name,
		Schedule: c.Schedule,
		Job:      string(payload),
		Enabled:  true,
		NextRun:  next,
	}
	return withStore(r, func(s *store.Store) error {
		if err := s.CronJobAdd(r.ctx, j); err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(j)
		}
		r.notice("added cron job %s (every %s)\n", c.Name, c.Schedule)
		return nil
	})
}

// ---- List ----

type CronListCmd struct{}

func (c *CronListCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		jobs, err := s.CronJobList(r.ctx)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(jobs)
		}
		if len(jobs) == 0 {
			fmt.Fprintln(r.stdout, "(no scheduled jobs)")
			return nil
		}
		now := time.Now().Unix()
		for _, j := range jobs {
			enabled := " "
			if !j.Enabled {
				enabled = "•" // visible "disabled" marker
			}
			due := ""
			if j.Enabled && j.NextRun <= now {
				due = " DUE"
			}
			next := time.Unix(j.NextRun, 0).Format(time.RFC3339)
			fmt.Fprintf(r.stdout, "%s %-20s  every %-9s  next %s%s\n", enabled, j.Name, j.Schedule, next, due)
			if j.LastStatus != nil && *j.LastStatus != "ok" {
				fmt.Fprintf(r.stdout, "    last_status: %s\n", *j.LastStatus)
			}
		}
		return nil
	})
}

// ---- Rm / Enable / Disable ----

type CronRmCmd struct {
	Name string `arg:"" help:"Job name."`
}

func (c *CronRmCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		if err := s.CronJobDelete(r.ctx, c.Name); err != nil {
			return err
		}
		r.notice("removed cron job %s\n", c.Name)
		return nil
	})
}

type CronEnableCmd struct {
	Name string `arg:"" help:"Job name."`
}

func (c *CronEnableCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		if err := s.CronJobSetEnabled(r.ctx, c.Name, true); err != nil {
			return err
		}
		r.notice("enabled %s\n", c.Name)
		return nil
	})
}

type CronDisableCmd struct {
	Name string `arg:"" help:"Job name."`
}

func (c *CronDisableCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		if err := s.CronJobSetEnabled(r.ctx, c.Name, false); err != nil {
			return err
		}
		r.notice("disabled %s\n", c.Name)
		return nil
	})
}

// ---- Run ----

type CronRunCmd struct {
	Force  string `name:"force" help:"Run this one job regardless of schedule (ignores enabled bit too)."`
	DryRun bool   `name:"dry-run" help:"Print what would run; don't execute or advance schedules."`
}

// cronRunResult is the per-job summary returned by `cron run`.
type cronRunResult struct {
	Name     string `json:"name"`
	Status   string `json:"status"`              // "ok" or "error: …"
	NextRun  int64  `json:"next_run"`            // unix epoch
	Output   string `json:"output,omitempty"`    // captured stdout, possibly truncated
	Truncated bool  `json:"truncated,omitempty"` // hint when output was clipped
}

// maxCapturedOutput keeps last_output from bloating the table when a job's
// underlying command is chatty. We store ~1KB for debugging context.
const maxCapturedOutput = 1024

func (c *CronRunCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		var jobs []store.CronJob
		if c.Force != "" {
			j, err := s.CronJobGet(r.ctx, c.Force)
			if err != nil {
				return err
			}
			jobs = []store.CronJob{j}
		} else {
			due, err := s.CronJobsDue(r.ctx, time.Now().Unix())
			if err != nil {
				return err
			}
			jobs = due
		}
		results := make([]cronRunResult, 0, len(jobs))
		for _, j := range jobs {
			res := runOneCronJob(r, s, j, c.DryRun)
			results = append(results, res)
		}
		if r.json {
			return r.emitJSON(results)
		}
		if len(results) == 0 {
			fmt.Fprintln(r.stdout, "(nothing due)")
			return nil
		}
		for _, res := range results {
			fmt.Fprintf(r.stdout, "%-20s  %s\n", res.Name, res.Status)
		}
		return nil
	})
}

// runOneCronJob decodes the job payload, executes it via in-process
// self-recursion, records the outcome, and returns a summary. Errors during
// execution become a "error: …" status — they don't abort the wider cron run.
func runOneCronJob(r *runCtx, s *store.Store, j store.CronJob, dryRun bool) cronRunResult {
	res := cronRunResult{Name: j.Name, NextRun: j.NextRun}

	// Decode payload.
	var payload cronJob
	if err := json.Unmarshal([]byte(j.Job), &payload); err != nil {
		res.Status = "error: invalid job payload: " + err.Error()
		_ = s.CronJobRecordRun(r.ctx, j.Name, time.Now().Unix(), j.NextRun, res.Status, "")
		return res
	}

	interval, _ := parseSchedule(j.Schedule)
	now := time.Now()
	nextRun := now.Add(interval).Unix()

	if dryRun {
		res.Status = "(dry-run) would execute"
		res.NextRun = nextRun
		return res
	}

	// Execute. Today only kind="cli" exists.
	var status string
	var output string
	switch payload.Kind {
	case "cli":
		out, runErr := invokeCLIWithinCron(r.ctx, r.dir, payload.Args)
		if runErr != nil {
			status = "error: " + runErr.Error()
		} else {
			status = "ok"
		}
		output = out
	default:
		status = "error: unknown job kind: " + payload.Kind
	}

	// Truncate captured output before persisting.
	truncated := false
	if len(output) > maxCapturedOutput {
		output = output[:maxCapturedOutput]
		truncated = true
	}

	if err := s.CronJobRecordRun(r.ctx, j.Name, now.Unix(), nextRun, status, output); err != nil {
		// Recording the outcome failed (rare). Surface it in the result;
		// the job's own work might still have succeeded.
		status = status + " (record failed: " + err.Error() + ")"
	}
	res.Status = status
	res.NextRun = nextRun
	res.Output = output
	res.Truncated = truncated
	return res
}

// invokeCLIWithinCron runs `Run(ctx, …, args)` in-process with the same
// --dir so the child invocation hits the same database. stdout and stderr
// are captured and joined into a single string. Non-zero exit becomes an
// error.
func invokeCLIWithinCron(ctx context.Context, dir string, args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("empty args")
	}
	if args[0] == "cron" {
		return "", errors.New("nested `cron` is forbidden")
	}
	// Prepend --dir so the recursive call uses the same DB as the parent.
	full := append([]string{"--dir", dir}, args...)

	var out, errb bytes.Buffer
	code := Run(ctx, &out, &errb, full)
	combined := out.String()
	if e := errb.String(); e != "" {
		if combined != "" {
			combined = combined + "\n" + e
		} else {
			combined = e
		}
	}
	if code != 0 {
		return combined, fmt.Errorf("exit %d", code)
	}
	return combined, nil
}

