package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParseScheduleAliases(t *testing.T) {
	cases := map[string]time.Duration{
		"@hourly":  time.Hour,
		"@daily":   24 * time.Hour,
		"@weekly":  7 * 24 * time.Hour,
		"@monthly": 30 * 24 * time.Hour,
		"+1h":      time.Hour,
		"+3d":      3 * 24 * time.Hour,
		"+2w":      2 * 7 * 24 * time.Hour,
	}
	for in, want := range cases {
		got, err := parseSchedule(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got != want {
			t.Fatalf("%q: want %s, got %s", in, want, got)
		}
	}
}

func TestParseScheduleRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"", "1h", "+0h", "+foo", "@yearly", "tomorrow", "+-1h"} {
		if _, err := parseSchedule(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestCLICronAddListRm(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	out := c.run("cron", "add", "ci-check",
		"--schedule", "@hourly",
		"--", "create", "-a", "infra-agent", "Check CI")
	if !strings.Contains(out, "ci-check") {
		t.Fatalf("expected add notice:\n%s", out)
	}
	out = c.run("cron", "list")
	if !strings.Contains(out, "ci-check") || !strings.Contains(out, "every @hourly") {
		t.Fatalf("list missing job:\n%s", out)
	}
	c.run("cron", "rm", "ci-check")
	out = c.run("cron", "list")
	if strings.Contains(out, "ci-check") {
		t.Fatalf("rm didn't drop job:\n%s", out)
	}
}

func TestCLICronAddRejectsNestedCron(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("cron", "add", "loopy",
		"--schedule", "@hourly",
		"--", "cron", "run")
}

func TestCLICronAddRejectsBadSchedule(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("cron", "add", "bad", "--schedule", "tomorrow", "--", "list")
}

func TestCLICronDuplicateNameRejected(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("cron", "add", "x", "--schedule", "@hourly", "--", "list")
	c.runFail("cron", "add", "x", "--schedule", "@daily", "--", "list")
}

func TestCLICronEnableDisable(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("cron", "add", "j", "--schedule", "@hourly", "--", "list")
	c.run("cron", "disable", "j")
	// `run` should not pick up disabled jobs even if due.
	out := c.run("cron", "run")
	if !strings.Contains(out, "(nothing due)") {
		t.Fatalf("disabled job should not run:\n%s", out)
	}
	c.run("cron", "enable", "j")
}

func TestCLICronRunStartNowExecutesAndAdvances(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	// --start-now: next_run = now, so it's due immediately.
	c.run("cron", "add", "ci-check",
		"--schedule", "@hourly",
		"--start-now",
		"--", "create", "-a", "infra-agent", "Auto: check CI")

	// The job creates an issue. After run, there should be one issue.
	out := c.run("cron", "run")
	if !strings.Contains(out, "ci-check") || !strings.Contains(out, "ok") {
		t.Fatalf("expected job to report ok:\n%s", out)
	}
	listOut := c.run("list")
	if !strings.Contains(listOut, "Auto: check CI") {
		t.Fatalf("cron job didn't create the issue:\n%s", listOut)
	}
	// Re-running should be a no-op now that next_run advanced.
	out = c.run("cron", "run")
	if !strings.Contains(out, "(nothing due)") {
		t.Fatalf("second run should be empty (next_run advanced):\n%s", out)
	}
}

func TestCLICronForceIgnoresSchedule(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	// Without --start-now, the new job's next_run is +1h, so it's NOT due.
	c.run("cron", "add", "j",
		"--schedule", "@hourly",
		"--", "create", "manual force test")

	// --force runs it anyway, even though schedule says no.
	out := c.run("cron", "run", "--force", "j")
	if !strings.Contains(out, "ok") {
		t.Fatalf("--force should execute:\n%s", out)
	}
	if !strings.Contains(c.run("list"), "manual force test") {
		t.Fatal("--force didn't actually run the job")
	}
}

func TestCLICronDryRunDoesNotExecuteOrAdvance(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("cron", "add", "j",
		"--schedule", "@hourly",
		"--start-now",
		"--", "create", "should not appear")

	out := c.run("cron", "run", "--dry-run")
	if !strings.Contains(out, "would execute") {
		t.Fatalf("--dry-run should announce, not run:\n%s", out)
	}
	if strings.Contains(c.run("list"), "should not appear") {
		t.Fatal("--dry-run actually executed the job")
	}
	// And next_run wasn't advanced — the job is still due.
	out = c.run("cron", "run", "--dry-run")
	if !strings.Contains(out, "would execute") {
		t.Fatalf("job should still be due after dry-run:\n%s", out)
	}
}

func TestCLICronCapturesErrorOutput(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	// `cli show bd-zzzz` exits non-zero with "issue not found".
	c.run("cron", "add", "j",
		"--schedule", "@hourly",
		"--start-now",
		"--", "show", "bd-zzzz")

	// Run still exits 0 (`cron run` reports per-job status; one failing
	// job shouldn't fail the whole sweep).
	out := c.run("cron", "run")
	if !strings.Contains(out, "error") {
		t.Fatalf("expected error captured in run output:\n%s", out)
	}
	// The error is stored in last_status; list shows it.
	out = c.run("cron", "list")
	if !strings.Contains(out, "last_status:") {
		t.Fatalf("expected last_status to be surfaced:\n%s", out)
	}
}

func TestCLICronListJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("cron", "add", "j",
		"--schedule", "@hourly",
		"--", "list")
	out := c.run("--json", "cron", "list")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) != 1 || rows[0]["name"] != "j" {
		t.Fatalf("unexpected JSON: %+v", rows)
	}
}
