package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testCLI bundles the args base ("--dir <tmp>/.beads") with stdio buffers.
type testCLI struct {
	t   *testing.T
	dir string
	ctx context.Context
	out bytes.Buffer
	err bytes.Buffer
}

func newTestCLI(t *testing.T) *testCLI {
	t.Helper()
	return &testCLI{t: t, dir: filepath.Join(t.TempDir(), ".beads"), ctx: context.Background()}
}

func (c *testCLI) run(args ...string) string {
	c.t.Helper()
	c.out.Reset()
	c.err.Reset()
	full := append([]string{"--dir", c.dir}, args...)
	code := Run(c.ctx, &c.out, &c.err, full)
	if code != 0 {
		c.t.Fatalf("bd %v exit %d\nstderr: %s", args, code, c.err.String())
	}
	return c.out.String()
}

func (c *testCLI) runFail(args ...string) {
	c.t.Helper()
	c.out.Reset()
	c.err.Reset()
	full := append([]string{"--dir", c.dir}, args...)
	if code := Run(c.ctx, &c.out, &c.err, full); code == 0 {
		c.t.Fatalf("bd %v unexpectedly succeeded", args)
	}
}

func TestCLIInitAndCreate(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "-p", "1", "first", "task"))
	if !strings.HasPrefix(id, "bd-") {
		t.Fatalf("expected id, got %q", id)
	}
	show := c.run("show", id)
	if !strings.Contains(show, "first task") {
		t.Fatalf("show output missing title:\n%s", show)
	}
}

func TestCLIWithoutInitFails(t *testing.T) {
	c := newTestCLI(t)
	c.runFail("create", "untitled")
}

func TestCLIReadyAndClaim(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "-p", "1", "task a"))
	b := strings.TrimSpace(c.run("create", "-p", "1", "task b"))
	c.run("dep", "add", b, a)

	ready := c.run("ready")
	if !strings.Contains(ready, a) {
		t.Fatalf("a should be ready: %s", ready)
	}
	if strings.Contains(ready, b) {
		t.Fatalf("b should NOT be ready: %s", ready)
	}

	claimed := c.run("claim", "--as", "alice")
	if !strings.Contains(claimed, a) {
		t.Fatalf("expected claim of %s, got %q", a, claimed)
	}

	c.run("close", a)
	ready = c.run("ready")
	if !strings.Contains(ready, b) {
		t.Fatalf("b should be ready after closing a: %s", ready)
	}
}

func TestCLIClaimByID(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "specific"))
	claimed := c.run("claim", id, "--as", "bob")
	if !strings.Contains(claimed, id) {
		t.Fatalf("expected claim of %s, got %q", id, claimed)
	}
}

func TestCLIClaimNoneReady(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("claim", "--as", "alice")
}

func TestCLIUpdate(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "todo"))
	c.run("update", id, "-p", "0", "--title", "urgent thing")
	show := c.run("show", id)
	if !strings.Contains(show, "urgent thing") || !strings.Contains(show, "Priority: 0") {
		t.Fatalf("update did not apply: %s", show)
	}
}

func TestCLIDepCycleRejected(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "a"))
	b := strings.TrimSpace(c.run("create", "b"))
	c.run("dep", "add", b, a)
	c.runFail("dep", "add", a, b)
}

func TestCLIList(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "open one"))
	b := strings.TrimSpace(c.run("create", "to close"))
	c.run("close", b)
	openList := c.run("list")
	if !strings.Contains(openList, a) || strings.Contains(openList, b) {
		t.Fatalf("list (open) wrong:\n%s", openList)
	}
	all := c.run("list", "--status", "all")
	if !strings.Contains(all, a) || !strings.Contains(all, b) {
		t.Fatalf("list (all) wrong:\n%s", all)
	}
}

func TestCLIUnknownCommand(t *testing.T) {
	c := newTestCLI(t)
	c.runFail("frobnicate")
}

func TestCLIAgentLanes(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	idShared := strings.TrimSpace(c.run("create", "shared task"))
	idCR := strings.TrimSpace(c.run("create", "-a", "code-reviewer", "review PR"))

	// bd ready (default lane) — only the shared task.
	out := c.run("ready")
	if !strings.Contains(out, idShared) || strings.Contains(out, idCR) {
		t.Fatalf("default-lane ready wrong:\n%s", out)
	}
	// bd ready -a code-reviewer — only the reviewer task.
	out = c.run("ready", "-a", "code-reviewer")
	if !strings.Contains(out, idCR) || strings.Contains(out, idShared) {
		t.Fatalf("code-reviewer-lane ready wrong:\n%s", out)
	}
	// bd list -a code-reviewer — only the reviewer task.
	out = c.run("list", "-a", "code-reviewer")
	if !strings.Contains(out, idCR) || strings.Contains(out, idShared) {
		t.Fatalf("code-reviewer-lane list wrong:\n%s", out)
	}
}

func TestCLIReadyWaitBlocksUntilAvailable(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")

	// Concurrently: create a code-reviewer task after ~80ms.
	go func() {
		time.Sleep(80 * time.Millisecond)
		c2 := &testCLI{t: t, dir: c.dir, ctx: context.Background()}
		c2.run("create", "-a", "code-reviewer", "incoming work")
	}()

	start := time.Now()
	out := c.run("ready", "-a", "code-reviewer", "--wait", "--interval", "20ms")
	elapsed := time.Since(start)
	if !strings.Contains(out, "incoming work") {
		t.Fatalf("expected new task in output:\n%s", out)
	}
	if elapsed < 60*time.Millisecond {
		t.Fatalf("--wait returned too fast (%s), didn't actually block", elapsed)
	}
}

func TestCLIClaimWaitThenSucceeds(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	go func() {
		time.Sleep(80 * time.Millisecond)
		c2 := &testCLI{t: t, dir: c.dir, ctx: context.Background()}
		c2.run("create", "-a", "writer", "blog post")
	}()
	out := c.run("claim", "-a", "writer", "--wait", "--interval", "20ms", "--as", "agent-1")
	if !strings.Contains(out, "blog post") {
		t.Fatalf("expected claim of waiting task:\n%s", out)
	}
}

func TestCLIWaitCancellation(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	c.ctx = ctx

	var wg sync.WaitGroup
	wg.Add(1)
	var code int
	go func() {
		defer wg.Done()
		full := []string{"--dir", c.dir, "ready", "-a", "noone", "--wait", "--interval", "10ms"}
		code = Run(c.ctx, &c.out, &c.err, full)
	}()
	wg.Wait()
	if code != 130 {
		t.Fatalf("expected exit 130 on cancellation, got %d (stderr: %s)", code, c.err.String())
	}
}

func TestCLILabelAddListRemove(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "needs labels"))
	c.run("label", "add", id, "security", "p0")
	ls := c.run("label", "ls", id)
	if !strings.Contains(ls, "security") || !strings.Contains(ls, "p0") {
		t.Fatalf("expected both labels listed:\n%s", ls)
	}
	c.run("label", "rm", id, "security")
	ls = c.run("label", "ls", id)
	if strings.Contains(ls, "security") || !strings.Contains(ls, "p0") {
		t.Fatalf("after rm, expected only p0:\n%s", ls)
	}
}

func TestCLIListShowsLabelsInline(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "labeled"))
	c.run("label", "add", id, "security", "p0")
	out := c.run("list")
	if !strings.Contains(out, "[p0, security]") {
		t.Fatalf("expected labels in list line:\n%s", out)
	}
}

func TestCLIShowDisplaysLabels(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "labeled"))
	c.run("label", "add", id, "needs-review")
	out := c.run("show", id)
	if !strings.Contains(out, "Labels:") || !strings.Contains(out, "needs-review") {
		t.Fatalf("show missing Labels line:\n%s", out)
	}
}

func TestCLIListFilterByLabel(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "tagged"))
	b := strings.TrimSpace(c.run("create", "untagged"))
	c.run("label", "add", a, "x")
	out := c.run("list", "-l", "x")
	if !strings.Contains(out, a) || strings.Contains(out, b) {
		t.Fatalf("expected only a:\n%s", out)
	}
}

func TestCLIListFilterByPriorityRange(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "-p", "0", "hi")
	mid := strings.TrimSpace(c.run("create", "-p", "2", "mid"))
	c.run("create", "-p", "4", "lo")
	out := c.run("list", "--priority-min", "1", "--priority-max", "3")
	if !strings.Contains(out, mid) || strings.Count(out, "bd-") != 1 {
		t.Fatalf("expected only mid:\n%s", out)
	}
}

func TestCLIListFilterByTitle(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	hit := strings.TrimSpace(c.run("create", "Fix the BUG today"))
	c.run("create", "unrelated")
	out := c.run("list", "--title-contains", "bug")
	if !strings.Contains(out, hit) || strings.Count(out, "bd-") != 1 {
		t.Fatalf("expected only the bug issue:\n%s", out)
	}
}

func TestCLIVersion(t *testing.T) {
	c := newTestCLI(t)
	out := c.run("version")
	if !strings.HasPrefix(out, "bd ") {
		t.Fatalf("expected 'bd <ver>': %q", out)
	}
}

func TestCLIVersionJSON(t *testing.T) {
	c := newTestCLI(t)
	out := c.run("--json", "version")
	if !strings.Contains(out, `"version"`) {
		t.Fatalf("expected JSON with version field: %q", out)
	}
}

func TestCLIQuietSuppressesNotices(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "task"))
	out := c.run("--quiet", "close", id)
	if out != "" {
		t.Fatalf("expected empty stdout under --quiet, got %q", out)
	}
}

func TestCLIQuietKeepsDataOutput(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	// create's data output (the new ID) must still print under --quiet.
	id := strings.TrimSpace(c.run("--quiet", "create", "task"))
	if !strings.HasPrefix(id, "bd-") {
		t.Fatalf("expected ID even under --quiet, got %q", id)
	}
}

func TestCLIJSONListIsValid(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "first")
	c.run("create", "second")
	out := c.run("--json", "list")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if _, ok := rows[0]["id"]; !ok {
		t.Fatalf("missing id field: %+v", rows[0])
	}
}

func TestCLIJSONShowIncludesLabels(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "labeled"))
	c.run("label", "add", id, "x", "y")
	out := c.run("--json", "show", id)
	var row map[string]any
	if err := json.Unmarshal([]byte(out), &row); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	labels, ok := row["labels"].([]any)
	if !ok || len(labels) != 2 {
		t.Fatalf("expected 2 labels: %+v", row)
	}
}

func TestCLICompletionBash(t *testing.T) {
	c := newTestCLI(t)
	out := c.run("completion", "bash")
	if !strings.Contains(out, "complete -F _bd_completions bd") {
		t.Fatalf("bash completion missing complete line:\n%s", out)
	}
}

func TestCLIListBadDate(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("list", "--created-after", "not-a-date")
}
