package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testCLI bundles the args base ("--dir <tmp>/.clu") with stdio buffers.
type testCLI struct {
	t   *testing.T
	dir string
	ctx context.Context
	out bytes.Buffer
	err bytes.Buffer
}

func newTestCLI(t *testing.T) *testCLI {
	t.Helper()
	return &testCLI{t: t, dir: filepath.Join(t.TempDir(), ".clu"), ctx: context.Background()}
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
	if !strings.HasPrefix(id, "clu-") {
		t.Fatalf("expected id, got %q", id)
	}
	show := c.run("show", id)
	if !strings.Contains(show, "first task") {
		t.Fatalf("show output missing title:\n%s", show)
	}
}

func TestCLINoArgsShowsHelp(t *testing.T) {
	// Bare `clu` with no args should print usage (exit 0), not an
	// "expected one of …" error.
	out := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := Run(context.Background(), out, stderr, []string{})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}
	body := out.String() + stderr.String()
	// Grouped help replaces the literal "Commands:" heading with the
	// group titles; assert on the usage line and a stable group title
	// instead.
	if !strings.Contains(body, "Usage:") || !strings.Contains(body, "Working with issues") {
		t.Fatalf("expected help output:\nstdout:%s\nstderr:%s", out.String(), stderr.String())
	}
}

func TestCLIInitScaffoldsConfigAndExample(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	for _, sub := range []string{"config.yaml", "data.sqlite", "templates/example.yaml"} {
		if _, err := os.Stat(filepath.Join(c.dir, sub)); err != nil {
			t.Fatalf("init should create %s: %v", sub, err)
		}
	}
}

func TestCLIInitIdempotent(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	// Second run is a no-op summary; should not fail or duplicate files.
	out := c.run("init")
	if !strings.Contains(out, "already initialized") {
		t.Fatalf("expected idempotent notice, got:\n%s", out)
	}
}

func TestCLIInitWithPrefix(t *testing.T) {
	c := newTestCLI(t)
	c.run("init", "--prefix", "acme-")
	id := strings.TrimSpace(c.run("create", "use the new prefix"))
	if !strings.HasPrefix(id, "acme-") {
		t.Fatalf("expected acme- prefix, got %q", id)
	}
	// info reflects the configured prefix.
	out := c.run("info")
	if !strings.Contains(out, "ID prefix:        acme-") {
		t.Fatalf("info missing prefix:\n%s", out)
	}
}

func TestCLIInitPrefixRejectedAfterFirstInit(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("init", "--prefix", "acme-")
}

func TestCLIInitRejectsBadPrefix(t *testing.T) {
	c := newTestCLI(t)
	c.runFail("init", "--prefix", "BadPrefix")
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

	claimed := c.run("claim")
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
	claimed := c.run("claim", id, "--agent", "bob")
	if !strings.Contains(claimed, id) {
		t.Fatalf("expected claim of %s, got %q", id, claimed)
	}
}

func TestCLIClaimNoneReady(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("claim", "--agent", "alice")
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

	// `clu ready` with no agent — only the shared (unassigned) task.
	out := c.run("ready")
	if !strings.Contains(out, idShared) || strings.Contains(out, idCR) {
		t.Fatalf("default-lane ready wrong:\n%s", out)
	}
	// `clu ready -a code-reviewer` — cr's pre-assigned + the shared
	// pool (post-v13 collapse: lane = assignee=me OR assignee IS NULL).
	out = c.run("ready", "-a", "code-reviewer")
	if !strings.Contains(out, idCR) || !strings.Contains(out, idShared) {
		t.Fatalf("code-reviewer-lane ready should show both pre-assigned and shared pool:\n%s", out)
	}
	// `clu list -a code-reviewer` — only the exact-assignee match.
	// List is a directory view, not a claim preview; matching the
	// shared pool here would make `-a X` useless for "show me X's
	// work".
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
	out := c.run("claim", "-a", "writer", "--wait", "--interval", "20ms")
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

func TestCLIListWatchRendersAndExitsOnCancel(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "first"))

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	c.ctx = ctx

	var wg sync.WaitGroup
	wg.Add(1)
	var code int
	go func() {
		defer wg.Done()
		full := []string{"--dir", c.dir, "list", "--watch", "--interval", "20ms"}
		code = Run(c.ctx, &c.out, &c.err, full)
	}()
	wg.Wait()
	if code != 130 {
		t.Fatalf("expected exit 130 on cancellation, got %d (stderr: %s)", code, c.err.String())
	}
	if !strings.Contains(c.out.String(), a) {
		t.Fatalf("expected initial render to include %s:\n%s", a, c.out.String())
	}
}

func TestCLIListWatchRedrawsOnChange(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "first"))

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	c.ctx = ctx

	// Create a second issue partway through the watch window — the
	// running --watch should pick it up on its next tick.
	go func() {
		time.Sleep(60 * time.Millisecond)
		c2 := &testCLI{t: t, dir: c.dir, ctx: context.Background()}
		c2.run("create", "second")
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		full := []string{"--dir", c.dir, "list", "--watch", "--interval", "20ms"}
		_ = Run(c.ctx, &c.out, &c.err, full)
	}()
	wg.Wait()

	out := c.out.String()
	if !strings.Contains(out, a) {
		t.Fatalf("first issue should appear:\n%s", out)
	}
	if !strings.Contains(out, "second") {
		t.Fatalf("second issue should appear after redraw:\n%s", out)
	}
}

func TestCLIListWatchOnlyEmitsOnChange(t *testing.T) {
	// Key invariant for downstream consumers: a pipe sees output ONLY
	// when matched issues actually change. Polling ticks without state
	// changes must be silent.
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "stable")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	c.ctx = ctx

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// 10ms interval × 200ms window = ~20 ticks; with one initial
		// render and no changes, output must contain exactly one block.
		full := []string{"--dir", c.dir, "list", "--watch", "--interval", "10ms"}
		_ = Run(c.ctx, &c.out, &c.err, full)
	}()
	wg.Wait()

	out := c.out.String()
	// Count occurrences of the issue title — must be 1, not 20.
	if got := strings.Count(out, "stable"); got != 1 {
		t.Fatalf("unchanged ticks should not re-emit; saw %d copies in:\n%s", got, out)
	}
}

func TestCLIListWatchRejectsJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("--json", "list", "--watch", "--interval", "10ms")
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
	if !strings.Contains(out, mid) || strings.Count(out, "clu-") != 1 {
		t.Fatalf("expected only mid:\n%s", out)
	}
}

func TestCLIListFilterByTitle(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	hit := strings.TrimSpace(c.run("create", "Fix the BUG today"))
	c.run("create", "unrelated")
	out := c.run("list", "--title-contains", "bug")
	if !strings.Contains(out, hit) || strings.Count(out, "clu-") != 1 {
		t.Fatalf("expected only the bug issue:\n%s", out)
	}
}

func TestCLIBlocked(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "a"))
	b := strings.TrimSpace(c.run("create", "b"))
	c.run("dep", "add", b, a)
	out := c.run("blocked")
	if !strings.Contains(out, b) || strings.Contains(out, a) {
		t.Fatalf("blocked should show only b:\n%s", out)
	}
}

func TestCLIListShowsBlockedAsDerivedStatus(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "parent"))
	b := strings.TrimSpace(c.run("create", "child"))
	c.run("dep", "add", b, a)
	out := c.run("list")
	// a (no open deps) renders as "open".
	if !strings.Contains(out, a+"  p2  open") {
		t.Fatalf("expected %s open in list:\n%s", a, out)
	}
	// b (has open parent) renders as "blocked".
	if !strings.Contains(out, b+"  p2  blocked") {
		t.Fatalf("expected %s blocked in list:\n%s", b, out)
	}
	// Closing the parent flips b back to open.
	c.run("close", a)
	out = c.run("list")
	if strings.Contains(out, b+"  p2  blocked") {
		t.Fatalf("after parent closed, %s should no longer be blocked:\n%s", b, out)
	}
	if !strings.Contains(out, b+"  p2  open") {
		t.Fatalf("expected %s open after parent closed:\n%s", b, out)
	}
}

func TestCLIListBlockedJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "parent"))
	b := strings.TrimSpace(c.run("create", "child"))
	c.run("dep", "add", b, a)
	out := c.run("--json", "list")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err)
	}
	// JSON's stored status field stays "open"; the derived field "blocked" carries the truth.
	for _, row := range rows {
		if row["id"] == a {
			if row["blocked"] == true {
				t.Fatalf("parent should not be blocked: %+v", row)
			}
		}
		if row["id"] == b {
			if row["status"] != "open" {
				t.Fatalf("child's stored status should stay 'open', got %q", row["status"])
			}
			if row["blocked"] != true {
				t.Fatalf("child should be blocked=true: %+v", row)
			}
		}
	}
}

func TestCLIShowDisplaysBlocked(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "parent"))
	b := strings.TrimSpace(c.run("create", "child"))
	c.run("dep", "add", b, a)
	out := c.run("show", b)
	if !strings.Contains(out, "Status:   blocked") {
		t.Fatalf("show should derive blocked:\n%s", out)
	}
}

func TestCLIInProgressNotFlippedToBlocked(t *testing.T) {
	// A claimed task with an open parent stays "in_progress" — the
	// blocked overlay is only for status=open.
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "parent"))
	b := strings.TrimSpace(c.run("create", "child"))
	c.run("dep", "add", b, a)
	// ClaimByID lets us claim a blocked issue directly (intentional —
	// we don't auto-prevent overrides).
	c.run("claim", b, "--agent", "alice")
	out := c.run("list")
	if !strings.Contains(out, b+"  p2  in_progress") {
		t.Fatalf("expected in_progress, not flipped to blocked:\n%s", out)
	}
}

func TestCLICount(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "one")
	c.run("create", "two")
	c.run("create", "three")
	out := strings.TrimSpace(c.run("count"))
	if out != "3" {
		t.Fatalf("expected 3, got %q", out)
	}
}

func TestCLICountJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "one")
	out := c.run("--json", "count")
	if !strings.Contains(out, `"count":1`) {
		t.Fatalf("expected JSON count: %s", out)
	}
}

func TestCLIStats(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "a")
	c.run("create", "-a", "code-reviewer", "b")
	out := c.run("stats")
	if !strings.Contains(out, "Status:") || !strings.Contains(out, "Assignees:") || !strings.Contains(out, "Types:") {
		t.Fatalf("stats missing sections:\n%s", out)
	}
	if !strings.Contains(out, "<none>") || !strings.Contains(out, "code-reviewer") {
		t.Fatalf("stats missing assignee grouping:\n%s", out)
	}
}

func TestCLISugarAssign(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "task"))
	c.run("assign", id, "alice")
	out := c.run("show", id)
	if !strings.Contains(out, "Assignee: alice") {
		t.Fatalf("assign didn't apply:\n%s", out)
	}
	c.run("assign", id) // clear
	out = c.run("show", id)
	if strings.Contains(out, "Assignee:") {
		t.Fatalf("clear didn't apply:\n%s", out)
	}
}

func TestCLISugarPriority(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "task"))
	c.run("priority", id, "0")
	out := c.run("show", id)
	if !strings.Contains(out, "Priority: 0") {
		t.Fatalf("priority didn't apply:\n%s", out)
	}
}

func TestCLISugarTag(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "task"))
	c.run("tag", id, "security", "p0")
	out := c.run("label", "ls", id)
	if !strings.Contains(out, "security") || !strings.Contains(out, "p0") {
		t.Fatalf("tag didn't apply:\n%s", out)
	}
}

func TestCLISugarLink(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "a"))
	b := strings.TrimSpace(c.run("create", "b"))
	c.run("link", b, a)
	out := c.run("show", b)
	if !strings.Contains(out, "Depends:") || !strings.Contains(out, a) {
		t.Fatalf("link didn't apply:\n%s", out)
	}
}

func TestCLIReopen(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "task"))
	c.run("close", id)
	c.run("reopen", id)
	out := c.run("show", id)
	if !strings.Contains(out, "Status:   open") {
		t.Fatalf("expected status open after reopen:\n%s", out)
	}
}

func TestCLIExportImportRoundTrip(t *testing.T) {
	// Build the dataset.
	src := newTestCLI(t)
	src.run("init")
	a := strings.TrimSpace(src.run("create", "-p", "0", "first task"))
	b := strings.TrimSpace(src.run("create", "-p", "2", "second"))
	src.run("dep", "add", b, a)
	src.run("label", "add", a, "security", "p0")
	src.run("defer", b, "+1h")

	// Export to a temp file.
	dump := filepath.Join(t.TempDir(), "dump.jsonl")
	src.run("export", "-o", dump)

	// Import into a fresh DB.
	dst := newTestCLI(t)
	dst.run("init")
	dst.run("import", dump)

	// Compare: all three issues survived with priorities + agent + deps + labels.
	for _, id := range []string{a, b} {
		out := dst.run("show", id)
		if !strings.Contains(out, id) {
			t.Fatalf("missing %s after import:\n%s", id, out)
		}
	}
	out := dst.run("show", a)
	if !strings.Contains(out, "security") || !strings.Contains(out, "p0") {
		t.Fatalf("labels lost on roundtrip:\n%s", out)
	}
	out = dst.run("show", b)
	if !strings.Contains(out, "Deferred:") {
		t.Fatalf("defer_until lost on roundtrip:\n%s", out)
	}
	if !strings.Contains(out, "Depends:") || !strings.Contains(out, a) {
		t.Fatalf("dep lost on roundtrip:\n%s", out)
	}
}

func TestCLIImportLenientSkipsBadLines(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	// Write a file with one valid line and one garbage line.
	path := filepath.Join(t.TempDir(), "mixed.jsonl")
	good := `{"kind":"issue","data":{"id":"bd-aaaa","title":"hello","type":"task","status":"open","priority":2,"created":1,"updated":1}}` + "\n"
	bad := "{not valid json}\n"
	if err := os.WriteFile(path, []byte(good+bad), 0o644); err != nil {
		t.Fatal(err)
	}
	c.run("import", "--lenient", path)
	out := c.run("show", "bd-aaaa")
	if !strings.Contains(out, "hello") {
		t.Fatalf("good line not imported:\n%s", out)
	}
}

func TestCLIImportStrictRejectsBadLines(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	path := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := os.WriteFile(path, []byte("{not valid json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.runFail("import", path)
}

func TestCLINoteSetAppendClearShow(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.run("note", "set", id, "first", "thought")
	if out := c.run("note", "show", id); !strings.Contains(out, "first thought") {
		t.Fatalf("set/show wrong:\n%s", out)
	}
	c.run("note", "append", id, "later", "thought")
	if out := c.run("note", "show", id); !strings.Contains(out, "first thought") || !strings.Contains(out, "later thought") {
		t.Fatalf("append didn't accumulate:\n%s", out)
	}
	// bd show includes the notes block.
	if out := c.run("show", id); !strings.Contains(out, "Notes:") {
		t.Fatalf("show missing Notes section:\n%s", out)
	}
	c.run("note", "clear", id)
	if out := c.run("note", "show", id); !strings.Contains(out, "(no notes)") {
		t.Fatalf("clear didn't clear:\n%s", out)
	}
}

func TestCLIVersionShort(t *testing.T) {
	c := newTestCLI(t)
	out := c.run("-V")
	if !strings.HasPrefix(out, "clu ") {
		t.Fatalf("expected -V to print version: %q", out)
	}
}

func TestCLICommentRoundTrip(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.run("comment", "add", id, "first", "thought", "--agent", "alice")
	c.run("comment", "add", id, "second", "thought", "--agent", "bob")
	out := c.run("comment", "ls", id)
	if !strings.Contains(out, "alice") || !strings.Contains(out, "bob") ||
		!strings.Contains(out, "first thought") || !strings.Contains(out, "second thought") {
		t.Fatalf("ls missing content:\n%s", out)
	}
	// bd show includes comments at the bottom.
	out = c.run("show", id)
	if !strings.Contains(out, "Comments (2)") {
		t.Fatalf("show missing comments header:\n%s", out)
	}
}

func TestCLICommentRm(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.run("comment", "add", id, "delete me", "--agent", "a")
	// Pull the numeric ID via JSON.
	out := c.run("--json", "comment", "ls", id)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 comment: %+v", rows)
	}
	cid := fmt.Sprintf("%v", rows[0]["id"])
	c.run("comment", "rm", cid)
	out = c.run("comment", "ls", id)
	if !strings.Contains(out, "(no comments)") {
		t.Fatalf("expected empty after rm: %s", out)
	}
}

func TestCLICommentExportImportRoundTrip(t *testing.T) {
	// Build dataset with comments.
	src := newTestCLI(t)
	src.run("init")
	id := strings.TrimSpace(src.run("create", "issue"))
	src.run("comment", "add", id, "first", "--agent", "alice")
	src.run("comment", "add", id, "second", "--agent", "bob")

	dump := filepath.Join(t.TempDir(), "dump.jsonl")
	src.run("export", "-o", dump)

	dst := newTestCLI(t)
	dst.run("init")
	dst.run("import", dump)

	out := dst.run("comment", "ls", id)
	if !strings.Contains(out, "alice") || !strings.Contains(out, "bob") ||
		!strings.Contains(out, "first") || !strings.Contains(out, "second") {
		t.Fatalf("comments lost on roundtrip:\n%s", out)
	}
}

func TestCLIDescribeRoundTrip(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.run("describe", id, "this is the long form")
	out := c.run("show", id)
	if !strings.Contains(out, "Description:") || !strings.Contains(out, "long form") {
		t.Fatalf("description not shown:\n%s", out)
	}
	c.run("describe", id) // clear
	out = c.run("show", id)
	if strings.Contains(out, "Description:") {
		t.Fatalf("clear didn't apply:\n%s", out)
	}
}

func TestCLIListEmptyDescription(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "with desc"))
	b := strings.TrimSpace(c.run("create", "bare"))
	c.run("describe", a, "explained")
	out := c.run("list", "--empty-description")
	if !strings.Contains(out, b) || strings.Contains(out, a) {
		t.Fatalf("empty-description filter wrong:\n%s", out)
	}
}

func TestCLIListLabelPattern(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "a"))
	b := strings.TrimSpace(c.run("create", "b"))
	c.run("create", "c")
	c.run("label", "add", a, "tech-debt")
	c.run("label", "add", b, "tech-legacy")
	out := c.run("list", "--label-pattern", "tech-*")
	if !strings.Contains(out, a) || !strings.Contains(out, b) {
		t.Fatalf("expected a and b in tech-* match:\n%s", out)
	}
}

func TestCLIListExcludeLabel(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "skip me"))
	b := strings.TrimSpace(c.run("create", "keep me"))
	c.run("label", "add", a, "wip")
	out := c.run("list", "--exclude-label", "wip")
	if strings.Contains(out, a) || !strings.Contains(out, b) {
		t.Fatalf("exclude-label wrong:\n%s", out)
	}
}

func TestCLIListSortReverse(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "-p", "0", "hi")
	c.run("create", "-p", "4", "lo")
	out := c.run("list", "-r")
	// Reverse default puts lower priority first.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.Contains(lines[0], "lo") {
		t.Fatalf("reverse default sort wrong:\n%s", out)
	}
}

func TestCLIInfo(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "a")
	c.run("create", "b")
	out := c.run("info")
	if !strings.Contains(out, "Schema version:") || !strings.Contains(out, "Total issues:     2") {
		t.Fatalf("info missing expected fields:\n%s", out)
	}
}

func TestCLIStatusesAndTypes(t *testing.T) {
	c := newTestCLI(t)
	if !strings.Contains(c.run("statuses"), "open") {
		t.Fatal("statuses missing 'open'")
	}
	if !strings.Contains(c.run("types"), "task") {
		t.Fatal("types missing 'task'")
	}
}

func TestCLIDoctorClean(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "a")
	out := c.run("doctor")
	for _, line := range []string{"Schema version", "Foreign keys", "Orphaned labels", "Orphaned deps"} {
		if !strings.Contains(out, line) {
			t.Fatalf("doctor missing %q:\n%s", line, out)
		}
	}
	if !strings.Contains(out, "✓") {
		t.Fatalf("doctor should mark clean DB with checkmarks:\n%s", out)
	}
}

func TestCLIDoctorJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	out := c.run("--json", "doctor")
	var r map[string]any
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if v, ok := r["foreign_key_ok"]; !ok || v != true {
		t.Fatalf("expected foreign_key_ok=true: %+v", r)
	}
}

func TestCLIDeferUndefer(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "later"))
	c.run("defer", id, "+1h")

	// Excluded from ready.
	out := c.run("ready")
	if strings.Contains(out, id) {
		t.Fatalf("deferred should not be ready:\n%s", out)
	}
	// Shown by --deferred filter.
	out = c.run("list", "--deferred")
	if !strings.Contains(out, id) {
		t.Fatalf("deferred should appear in --deferred list:\n%s", out)
	}
	// Show includes Deferred line.
	out = c.run("show", id)
	if !strings.Contains(out, "Deferred:") {
		t.Fatalf("show missing Deferred line:\n%s", out)
	}
	// Undefer restores readiness.
	c.run("undefer", id)
	out = c.run("ready")
	if !strings.Contains(out, id) {
		t.Fatalf("undeferred should be ready again:\n%s", out)
	}
}

func TestCLIDeferBadDuration(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.runFail("defer", id, "+nope")
}

func TestCLIKVRoundTrip(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("kv", "set", "feature_flag", "true")
	c.run("kv", "set", "api_endpoint", "https://api.example.com")
	c.run("kv", "set", "max_retries", "3")

	// get prints just the value.
	if got := strings.TrimSpace(c.run("kv", "get", "feature_flag")); got != "true" {
		t.Fatalf("expected 'true', got %q", got)
	}
	if got := strings.TrimSpace(c.run("kv", "get", "api_endpoint")); got != "https://api.example.com" {
		t.Fatalf("expected URL, got %q", got)
	}

	// set overwrites.
	c.run("kv", "set", "feature_flag", "false")
	if got := strings.TrimSpace(c.run("kv", "get", "feature_flag")); got != "false" {
		t.Fatalf("expected 'false' after overwrite, got %q", got)
	}

	// list shows all three, alphabetised.
	out := c.run("kv", "list")
	want := "api_endpoint=https://api.example.com\nfeature_flag=false\nmax_retries=3\n"
	if out != want {
		t.Fatalf("list mismatch:\nwant:\n%s\ngot:\n%s", want, out)
	}

	// clear removes one; the rest stay.
	c.run("kv", "clear", "api_endpoint")
	out = c.run("kv", "list")
	if strings.Contains(out, "api_endpoint") {
		t.Fatalf("clear didn't remove api_endpoint:\n%s", out)
	}
	if !strings.Contains(out, "feature_flag") {
		t.Fatalf("clear removed too much:\n%s", out)
	}
}

func TestCLIKVGetMissingFails(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("kv", "get", "absent_key")
}

func TestCLIKVListEmpty(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	if got := c.run("kv", "list"); !strings.Contains(got, "(empty)") {
		t.Fatalf("expected '(empty)' on fresh store, got %q", got)
	}
}

func TestCLIKVGetJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("kv", "set", "k", "v")
	out := c.run("--json", "kv", "get", "k")
	var row map[string]any
	if err := json.Unmarshal([]byte(out), &row); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if row["key"] != "k" || row["value"] != "v" {
		t.Fatalf("unexpected JSON: %+v", row)
	}
}

func TestCLIKVValueWithSpaces(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("kv", "set", "msg", "hello", "world", "and", "everyone")
	if got := strings.TrimSpace(c.run("kv", "get", "msg")); got != "hello world and everyone" {
		t.Fatalf("expected joined value, got %q", got)
	}
}

func TestCLIKVExportImportRoundTrip(t *testing.T) {
	src := newTestCLI(t)
	src.run("init")
	src.run("kv", "set", "feature_flag", "on")
	src.run("kv", "set", "max_retries", "5")
	src.run("create", "an issue") // mix with regular data to make sure both kinds export

	dump := filepath.Join(t.TempDir(), "dump.jsonl")
	src.run("export", "-o", dump)

	dst := newTestCLI(t)
	dst.run("init")
	dst.run("import", dump)

	for k, want := range map[string]string{"feature_flag": "on", "max_retries": "5"} {
		got := strings.TrimSpace(dst.run("kv", "get", k))
		if got != want {
			t.Fatalf("kv %s: expected %q, got %q", k, want, got)
		}
	}
}

func TestCLICloseMultiple(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "a"))
	b := strings.TrimSpace(c.run("create", "b"))
	cd := strings.TrimSpace(c.run("create", "c"))
	out := c.run("close", a, b, cd)
	for _, id := range []string{a, b, cd} {
		if !strings.Contains(out, "closed "+id) {
			t.Fatalf("missing close notice for %s:\n%s", id, out)
		}
	}
}

func TestCLICloseMultipleJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "a"))
	b := strings.TrimSpace(c.run("create", "b"))
	out := c.run("--json", "close", a, b)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	for _, row := range rows {
		if row["status"] != "closed" {
			t.Fatalf("row not closed: %+v", row)
		}
	}
}

func TestCLICloseContinuesPastErrors(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "a"))
	cd := strings.TrimSpace(c.run("create", "c"))
	// Middle ID is bogus.
	c.runFail("close", a, "bd-zzzz", cd)
	// Both real ones should still be closed.
	for _, id := range []string{a, cd} {
		out := c.run("show", id)
		if !strings.Contains(out, "Status:   closed") {
			t.Fatalf("expected %s closed after partial-failure batch:\n%s", id, out)
		}
	}
}

func TestCLIReopenMultiple(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "a"))
	b := strings.TrimSpace(c.run("create", "b"))
	c.run("close", a, b)
	c.run("reopen", a, b)
	for _, id := range []string{a, b} {
		out := c.run("show", id)
		if !strings.Contains(out, "Status:   open") {
			t.Fatalf("expected %s open after batch reopen:\n%s", id, out)
		}
	}
}

func TestCLIUndeferMultiple(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "a"))
	b := strings.TrimSpace(c.run("create", "b"))
	c.run("defer", a, "+1h")
	c.run("defer", b, "+1h")
	c.run("undefer", a, b)
	for _, id := range []string{a, b} {
		out := c.run("show", id)
		if strings.Contains(out, "Deferred:") {
			t.Fatalf("expected %s undeferred:\n%s", id, out)
		}
	}
}

func TestCLIListDefaultIncludesInProgress(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	open := strings.TrimSpace(c.run("create", "still open"))
	wip := strings.TrimSpace(c.run("create", "in progress"))
	closed := strings.TrimSpace(c.run("create", "done"))
	c.run("close", closed)
	c.run("claim", wip, "--agent", "alice")

	out := c.run("list") // default --status open,in_progress
	if !strings.Contains(out, open) {
		t.Fatalf("expected open issue in default list:\n%s", out)
	}
	if !strings.Contains(out, wip) {
		t.Fatalf("expected in_progress issue in default list:\n%s", out)
	}
	if strings.Contains(out, closed) {
		t.Fatalf("closed issue should be hidden in default list:\n%s", out)
	}
}

func TestCLIListStatusAll(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "a"))
	b := strings.TrimSpace(c.run("create", "b"))
	c.run("close", b)
	out := c.run("list", "--status", "all")
	if !strings.Contains(out, a) || !strings.Contains(out, b) {
		t.Fatalf("--status all should include closed:\n%s", out)
	}
}

func TestCLIListStatusCommaSeparated(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "a"))
	b := strings.TrimSpace(c.run("create", "b"))
	c.run("close", b)
	out := c.run("list", "--status", "closed")
	if strings.Contains(out, a) || !strings.Contains(out, b) {
		t.Fatalf("--status closed should show only closed:\n%s", out)
	}
}

func TestCLIClaimEmitsFullIssue(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "-p", "0", "claim me"))
	c.run("describe", id, "the long form")
	out := c.run("claim")
	// Human mode includes the notice header and a full show-style block.
	if !strings.Contains(out, "claimed "+id) {
		t.Fatalf("expected notice header:\n%s", out)
	}
	if !strings.Contains(out, "Description:") || !strings.Contains(out, "the long form") {
		t.Fatalf("expected full issue body:\n%s", out)
	}
}

func TestCLIClaimJSONIsCleanJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "claim me")
	out := c.run("--json", "claim")
	if strings.HasPrefix(out, "claimed ") {
		t.Fatalf("notice leaked into --json output: %q", out)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(out), &row); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	// Without --agent, assignee defaults to $USER. The point of this
	// test is that status flips and *some* assignee is recorded.
	if row["status"] != "in_progress" || row["assignee"] == nil || row["assignee"] == "" {
		t.Fatalf("missing claim mutations in JSON: %+v", row)
	}
}

func TestCLIVersion(t *testing.T) {
	c := newTestCLI(t)
	out := c.run("version")
	if !strings.HasPrefix(out, "clu ") {
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
	if !strings.HasPrefix(id, "clu-") {
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
	if !strings.Contains(out, "complete -F _clu_completions clu") {
		t.Fatalf("bash completion missing complete line:\n%s", out)
	}
}

func TestCLIClaimMissingSaysNotFound(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("claim", "bd-zzzz", "--agent", "alice")
	if !strings.Contains(c.err.String(), "not found") {
		t.Fatalf("expected 'not found' for missing issue, got: %s", c.err.String())
	}
}

func TestCLICommentRmMissingSaysCommentNotFound(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("comment", "rm", "99999")
	if !strings.Contains(c.err.String(), "comment not found") {
		t.Fatalf("expected 'comment not found', got: %s", c.err.String())
	}
}

func TestCLIDepRmNoEdgeErrors(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "a"))
	b := strings.TrimSpace(c.run("create", "b"))
	// No edge between them yet.
	c.runFail("dep", "rm", a, b)
}

func TestCLIUndeferNotDeferredErrors(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.runFail("undefer", id)
}

func TestCLIInitTwiceSaysAlreadyInitialized(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	out := c.run("init")
	if !strings.Contains(out, "already initialized") {
		t.Fatalf("expected 'already initialized', got: %s", out)
	}
}

func TestCLIJSONWriteCommandsEmitObject(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")

	parseObj := func(t *testing.T, src string) map[string]any {
		t.Helper()
		var row map[string]any
		if err := json.Unmarshal([]byte(src), &row); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, src)
		}
		return row
	}

	// create
	out := c.run("--json", "create", "first")
	created := parseObj(t, out)
	id, _ := created["id"].(string)
	if !strings.HasPrefix(id, "clu-") {
		t.Fatalf("create --json missing id: %+v", created)
	}

	// update
	out = c.run("--json", "update", id, "-p", "1")
	upd := parseObj(t, out)
	if upd["priority"].(float64) != 1 {
		t.Fatalf("update --json priority wrong: %+v", upd)
	}

	// dep add
	other := strings.TrimSpace(c.run("create", "other"))
	out = c.run("--json", "dep", "add", id, other)
	parseObj(t, out) // just must be valid JSON object

	// label add
	out = c.run("--json", "label", "add", id, "x")
	la := parseObj(t, out)
	labels, _ := la["labels"].([]any)
	if len(labels) != 1 {
		t.Fatalf("label add --json missing labels: %+v", la)
	}

	// defer
	out = c.run("--json", "defer", id, "+1h")
	d := parseObj(t, out)
	if d["defer_until"] == nil {
		t.Fatalf("defer --json missing defer_until: %+v", d)
	}

	// note set
	out = c.run("--json", "note", "set", id, "hi")
	n := parseObj(t, out)
	if n["notes"] != "hi" {
		t.Fatalf("note set --json missing notes: %+v", n)
	}

	// kv set
	out = c.run("--json", "kv", "set", "k", "v")
	kv := parseObj(t, out)
	if kv["key"] != "k" || kv["value"] != "v" {
		t.Fatalf("kv set --json wrong: %+v", kv)
	}
}

func TestCLIStatsJSONNoHTMLEscape(t *testing.T) {
	// `<none>` is the agent grouping for nil; HTML-escaping turns it
	// into `<none>` which is valid but ugly. The fix is
	// SetEscapeHTML(false) across the JSON emitters.
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "x")
	out := c.run("--json", "stats")
	// Without SetEscapeHTML(false) this would be "<none>".
	if !strings.Contains(out, "<none>") {
		t.Fatalf("expected raw <none> in JSON: %s", out)
	}
	if strings.Contains(out, "\\u003c") {
		t.Fatalf("expected SetEscapeHTML(false), got escaped output: %s", out)
	}
}

func TestCLIListBadDate(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("list", "--created-after", "not-a-date")
}

func TestCLILabelAddRejectsEmptyString(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.runFail("label", "add", id, "")
}

func TestCLILabelRmRejectsUnknownIssue(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("label", "rm", "bd-zzzz", "foo")
}

func TestCLILabelLsRejectsUnknownIssue(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("label", "ls", "bd-zzzz")
}

func TestCLICommentLsRejectsUnknownIssue(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("comment", "ls", "bd-zzzz")
}

func TestCLIRejectsInvalidStatus(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.runFail("update", id, "--status", "invalid")
	// Pre-existing status untouched.
	out := c.run("show", id)
	if !strings.Contains(out, "Status:   open") {
		t.Fatalf("status should still be open:\n%s", out)
	}
}

func TestCLIUpdateStatusOutOfClosedClearsTimestamp(t *testing.T) {
	// Regression: `update --status open` on a closed issue left the
	// `closed` timestamp populated, so `show` reported an open issue
	// with a Closed: timestamp.
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.run("close", id)
	out := c.run("--json", "show", id)
	if !strings.Contains(out, `"status":"closed"`) || strings.Contains(out, `"closed":null`) {
		t.Fatalf("setup: expected closed status with non-null timestamp:\n%s", out)
	}

	c.run("update", id, "--status", "open")
	out = c.run("--json", "show", id)
	if !strings.Contains(out, `"status":"open"`) {
		t.Fatalf("expected status=open after update:\n%s", out)
	}
	// `closed` must be cleared — the omitempty JSON tag drops null,
	// so we look for absence of the "closed" key.
	if strings.Contains(out, `"closed":`) {
		t.Fatalf("closed timestamp should be cleared, but JSON still has the key:\n%s", out)
	}
}

func TestCLIRejectsInvalidType(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("create", "x", "-t", "notatype")
}

func TestCLIRejectsInvalidPriorityOnCreate(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("create", "x", "-p", "99")
	c.runFail("create", "x", "-p", "-1")
}

func TestCLIRejectsInvalidPriorityOnSugar(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.runFail("priority", id, "99")
}

func TestCLIHelpDoesNotRunCommand(t *testing.T) {
	// `cli init --help` must print help and NOT initialize the DB.
	c := newTestCLI(t)
	out := c.run("init", "--help")
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("expected help output, got:\n%s", out)
	}
	if strings.Contains(out, "initialized") {
		t.Fatalf("init ran as a side effect of --help:\n%s", out)
	}
	// The DB file must not exist after asking for help.
	if _, err := os.Stat(filepath.Join(c.dir, "data.sqlite")); err == nil {
		t.Fatalf("DB file created by --help (expected nothing)")
	}
}

func TestCLITopLevelHelpExits0(t *testing.T) {
	// `cli --help` should exit 0 and print usage.
	c := newTestCLI(t)
	out := c.run("--help")
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("expected usage on --help, got:\n%s", out)
	}
}

func TestCLIExportStdoutIsCleanJSONL(t *testing.T) {
	// `cli export` to stdout must not interleave a summary line — the
	// output has to be parseable JSONL.
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "first")
	c.run("create", "second")
	out := c.run("export")
	for i, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		var anyVal any
		if err := json.Unmarshal([]byte(line), &anyVal); err != nil {
			t.Fatalf("export line %d not valid JSON: %v\nline: %q\nfull:\n%s", i+1, err, line, out)
		}
	}
	// Summary must show up on stderr (when not --quiet).
	if !strings.Contains(c.err.String(), "exported ") {
		t.Fatalf("expected 'exported …' summary on stderr, got: %q", c.err.String())
	}
}

func TestCLISubcommandHelpExits0(t *testing.T) {
	// `cli show --help` must exit 0 even without a required <id> arg.
	c := newTestCLI(t)
	out := c.run("show", "--help")
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("expected usage on show --help, got:\n%s", out)
	}
}

func TestCLICreateRejectsWhitespaceTitle(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("create", "")
	c.runFail("create", "   ")
	c.runFail("create", " \t\n ")
}

func TestCLIVersionLongFlag(t *testing.T) {
	// --version (long form) should work the same as -V.
	c := newTestCLI(t)
	out := c.run("--version")
	if !strings.HasPrefix(out, "clu ") {
		t.Fatalf("expected version banner, got:\n%s", out)
	}
}

func TestCLIDepLsListsBothDirections(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "parent"))
	b := strings.TrimSpace(c.run("create", "child"))
	d := strings.TrimSpace(c.run("create", "another child"))
	c.run("link", b, a)
	c.run("link", d, a)
	// On a: blocks both children, depends on nothing.
	out := c.run("dep", "ls", a)
	if !strings.Contains(out, b) || !strings.Contains(out, d) {
		t.Fatalf("expected both children listed under blocks:\n%s", out)
	}
	if !strings.Contains(out, "depends on: (none)") {
		t.Fatalf("expected '(none)' for empty parents:\n%s", out)
	}
	// Unknown ID errors.
	c.runFail("dep", "ls", "clu-9999")
}

func TestCLILabelAddHonestCount(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.run("label", "add", id, "alpha", "beta")
	// Re-add: both already present.
	out := c.run("label", "add", id, "alpha", "beta")
	if !strings.Contains(out, "added 0") || !strings.Contains(out, "2 already present") {
		t.Fatalf("expected '0 added, 2 already present', got:\n%s", out)
	}
}

func TestCLICommentEdit(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	add := c.run("comment", "add", id, "first draft", "-a", "alice")
	// Extract comment ID from notice "(#N)".
	var cid string
	for _, tok := range strings.Fields(add) {
		if strings.HasPrefix(tok, "(#") && strings.HasSuffix(tok, ")") {
			cid = tok[2 : len(tok)-1]
		}
	}
	if cid == "" {
		t.Fatalf("could not extract comment id from: %s", add)
	}
	c.run("comment", "edit", cid, "revised")
	ls := c.run("comment", "ls", id)
	if !strings.Contains(ls, "revised") {
		t.Fatalf("edit didn't apply:\n%s", ls)
	}
	if strings.Contains(ls, "first draft") {
		t.Fatalf("old body still present:\n%s", ls)
	}
}

func TestCLIShowJSONShapeIsStable(t *testing.T) {
	// Regression: clu --json show used to omit empty labels/depends/
	// blocks/comments arrays. Consumers want a uniform shape.
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "no deps no labels"))
	out := c.run("--json", "show", id)
	for _, want := range []string{
		`"labels":[]`,
		`"depends_on":[]`,
		`"blocks":[]`,
		`"comments":[]`,
		`"blocked":false`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected stable %q in JSON, got:\n%s", want, out)
		}
	}
}

func TestCLIListShowsDeferred(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.run("defer", id, "+1h")
	out := c.run("list", "--status", "all")
	if !strings.Contains(out, "deferred") {
		t.Fatalf("deferred row should show 'deferred' status:\n%s", out)
	}
}

func TestCLICloseClearsDeferUntil(t *testing.T) {
	// Regression: closing a deferred issue used to leave defer_until
	// set, so doctor's Closed+deferred check fired forever.
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.run("defer", id, "+1h")
	c.run("close", id)
	out := c.run("--json", "show", id)
	if strings.Contains(out, `"defer_until":`) {
		t.Fatalf("close should clear defer_until:\n%s", out)
	}
	// Cancel-cascade should clear it too.
	id2 := strings.TrimSpace(c.run("create", "y"))
	c.run("defer", id2, "+1h")
	c.run("cancel", id2)
	out = c.run("--json", "show", id2)
	if strings.Contains(out, `"defer_until":`) {
		t.Fatalf("cancel should clear defer_until:\n%s", out)
	}
	// `update --status closed` path too.
	id3 := strings.TrimSpace(c.run("create", "z"))
	c.run("defer", id3, "+1h")
	c.run("update", id3, "--status", "closed")
	out = c.run("--json", "show", id3)
	if strings.Contains(out, `"defer_until":`) {
		t.Fatalf("update --status closed should clear defer_until:\n%s", out)
	}
}

func TestCLIInfoOpenCountLabel(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "x")
	out := c.run("info")
	if !strings.Contains(out, "open (incl. blocked)") {
		t.Fatalf("info should label open as 'open (incl. blocked)':\n%s", out)
	}
}

func TestCLICancelCascadesToDependents(t *testing.T) {
	// Shape:  A ← B ← C   and   A ← D
	// Cancel A → everything in this graph should be cancelled.
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "root"))
	b := strings.TrimSpace(c.run("create", "child of a"))
	cc := strings.TrimSpace(c.run("create", "grandchild via b"))
	d := strings.TrimSpace(c.run("create", "sibling child of a"))
	unrelated := strings.TrimSpace(c.run("create", "unrelated"))
	c.run("link", b, a)
	c.run("link", cc, b)
	c.run("link", d, a)

	out := c.run("cancel", a)
	for _, want := range []string{a, b, cc, d} {
		if !strings.Contains(out, want) {
			t.Fatalf("cancel output missing %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, unrelated) {
		t.Fatalf("unrelated issue %s should not be cancelled:\n%s", unrelated, out)
	}

	// Verify status persisted for each cancelled issue and unchanged for unrelated.
	for _, id := range []string{a, b, cc, d} {
		s := c.run("show", id)
		if !strings.Contains(s, "cancelled") {
			t.Fatalf("show %s should report cancelled status:\n%s", id, s)
		}
	}
	if s := c.run("show", unrelated); !strings.Contains(s, "open") {
		t.Fatalf("unrelated should still be open:\n%s", s)
	}
}

func TestCLICancelSkipsAlreadyTerminal(t *testing.T) {
	// Closing one descendant first: cancel should skip it (not re-mark) and
	// only cancel the still-non-terminal ones.
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "root"))
	b := strings.TrimSpace(c.run("create", "child"))
	cc := strings.TrimSpace(c.run("create", "grandchild"))
	c.run("link", b, a)
	c.run("link", cc, b)
	c.run("close", b) // b is already terminal

	out := c.run("cancel", a)
	// a and cc should be cancelled; b should NOT appear (already closed).
	if !strings.Contains(out, a) || !strings.Contains(out, cc) {
		t.Fatalf("expected a and cc in output:\n%s", out)
	}
	if strings.Contains(out, "cancelled "+b) {
		t.Fatalf("already-closed b should not be re-cancelled:\n%s", out)
	}
	if s := c.run("show", b); !strings.Contains(s, "closed") {
		t.Fatalf("b should remain closed:\n%s", s)
	}
}

func TestCLICancelNothingToDo(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "root"))
	c.run("close", a)
	out := c.run("cancel", a)
	if !strings.Contains(out, "nothing to cancel") {
		t.Fatalf("expected 'nothing to cancel' notice:\n%s", out)
	}
}

func TestCLICancelUnknownIDFails(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("cancel", "clu-deadbeef")
}

func TestCLICancelReopenRestoresOpen(t *testing.T) {
	// cancelled → reopen should round-trip to open.
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "root"))
	c.run("cancel", a)
	c.run("reopen", a)
	if s := c.run("show", a); !strings.Contains(s, "open") {
		t.Fatalf("expected reopen to restore status=open:\n%s", s)
	}
}

func TestCLICancelJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "root"))
	b := strings.TrimSpace(c.run("create", "child"))
	c.run("link", b, a)

	out := c.run("--json", "cancel", a)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 cancelled rows, got %d:\n%s", len(rows), out)
	}
	for _, r := range rows {
		if r["status"] != "cancelled" {
			t.Fatalf("expected status=cancelled, got %v", r["status"])
		}
	}
}

func TestCLIBriefIncludesManualAndDeclaredAgents(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	writeAgentsConfig(t, c.dir, `
id_prefix: clu-
agents:
  code-reviewer:
    description: "Reviews Go code"
    capabilities: [go-review]
`)
	out := c.run("brief")
	for _, want := range []string{
		"Using `clu` as a Claude Code agent", // from AGENTS.md
		"This project's agents",
		"code-reviewer",
		"Reviews Go code",
		"go-review",
		"Currently active",
		"Persisted memories",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("brief output missing %q:\n%s", want, out)
		}
	}
}

func TestCLIBriefWorksWithoutInit(t *testing.T) {
	// brief should work pre-init so a fresh agent can read it before
	// the project has a database.
	c := newTestCLI(t)
	out := c.run("brief")
	if !strings.Contains(out, "Using `clu` as a Claude Code agent") {
		t.Fatalf("brief should include the manual even without init:\n%s", out)
	}
	if !strings.Contains(out, "No agents declared") {
		t.Fatalf("expected 'No agents declared' notice:\n%s", out)
	}
}

func TestCLINoteSetRejectsEmpty(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.runFail("note", "set", id, "")
	c.runFail("note", "set", id, "   ") // whitespace-only also rejected
	// `note clear` is the explicit way to wipe notes.
	c.run("note", "set", id, "real content")
	c.run("note", "clear", id)
	out := c.run("note", "show", id)
	if !strings.Contains(out, "(no notes)") {
		t.Fatalf("clear should wipe notes:\n%s", out)
	}
}

func TestCLIUpdateRejectsNoFlags(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.runFail("update", id) // no flags
}

func TestCLIReopenWarnsOnStuckParent(t *testing.T) {
	// Cancel-cascade then reopen a child: the parent is still
	// cancelled, so the reopened child will stay blocked forever
	// unless the user takes action. Reopen must warn (not block).
	c := newTestCLI(t)
	c.run("init")
	parent := strings.TrimSpace(c.run("create", "parent"))
	child := strings.TrimSpace(c.run("create", "child"))
	c.run("link", child, parent)
	c.run("cancel", parent) // cascades to child
	c.out.Reset()
	c.err.Reset()
	c.run("reopen", child)
	if !strings.Contains(c.err.String(), "unresolved parents") {
		t.Fatalf("expected stuck-parent warning on stderr:\n%s", c.err.String())
	}
	if !strings.Contains(c.err.String(), "cancelled") {
		t.Fatalf("warning should name the parent status:\n%s", c.err.String())
	}
	// Reopen of a child with a closed parent → no warning.
	p2 := strings.TrimSpace(c.run("create", "p2"))
	c2 := strings.TrimSpace(c.run("create", "c2"))
	c.run("link", c2, p2)
	c.run("close", p2)
	c.run("close", c2)
	c.out.Reset()
	c.err.Reset()
	c.run("reopen", c2)
	if strings.Contains(c.err.String(), "unresolved parents") {
		t.Fatalf("closed parent shouldn't trigger a warning:\n%s", c.err.String())
	}
}

func TestCLICreateWithDepWiresEdges(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	a := strings.TrimSpace(c.run("create", "parent A"))
	b := strings.TrimSpace(c.run("create", "parent B"))
	child := strings.TrimSpace(c.run("create", "--dep", a, "--dep", b, "child"))
	// Child is blocked by both parents until either closes.
	out := c.run("blocked")
	if !strings.Contains(out, child) {
		t.Fatalf("child should appear in blocked list:\n%s", out)
	}
	// Comma-separated also accepted.
	a2 := strings.TrimSpace(c.run("create", "p1"))
	b2 := strings.TrimSpace(c.run("create", "p2"))
	c2 := strings.TrimSpace(c.run("create", "-d", a2+","+b2, "c2"))
	out = c.run("show", c2)
	if !strings.Contains(out, "Depends:") || !strings.Contains(out, a2) || !strings.Contains(out, b2) {
		t.Fatalf("show should list both parents:\n%s", out)
	}
}

func TestCLICreateWithDepRejectsMissingParent(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	// No issue was ever created; --dep <unknown> must fail and not
	// leave a half-linked child behind.
	c.runFail("create", "--dep", "clu-ffff", "orphan")
	out := c.run("list", "--status", "all")
	if strings.Contains(out, "orphan") {
		t.Fatalf("failed create should not have inserted the issue:\n%s", out)
	}
}

func TestCLICreateWithDescriptionAndNotes(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create",
		"--description", "Long-form context goes here.",
		"--notes", "Working theory: X.",
		"do the thing"))
	out := c.run("show", id)
	for _, want := range []string{
		"Long-form context goes here.",
		"Working theory: X.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("show missing %q:\n%s", want, out)
		}
	}
}

func TestCLICreateWithDepIsAtomicVsClaimWatch(t *testing.T) {
	// The race the --dep flag is meant to close: a watching claim
	// loop must NOT see a parent-less version of the new issue. After
	// create completes, the issue must already be blocked.
	//
	// We can't reliably observe the intermediate state in a unit test
	// (it's a sub-millisecond window the tx now closes), but we can
	// at least assert the post-condition: immediately after create,
	// `ready` does not include the new child.
	c := newTestCLI(t)
	c.run("init")
	parent := strings.TrimSpace(c.run("create", "parent"))
	child := strings.TrimSpace(c.run("create", "--dep", parent, "child"))
	out := c.run("ready")
	if strings.Contains(out, child) {
		t.Fatalf("child must not appear in `ready` after --dep create:\n%s", out)
	}
	if !strings.Contains(out, parent) {
		t.Fatalf("parent should still be ready:\n%s", out)
	}
}

func TestCLICreateWarnsOnUndeclaredAgent(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	writeAgentsConfig(t, c.dir, `
id_prefix: clu-
agents:
  code-reviewer:
    description: "reviews"
`)
	// Declared agent → no warning.
	c.out.Reset()
	c.err.Reset()
	c.run("create", "-a", "code-reviewer", "ok")
	if strings.Contains(c.err.String(), "warning") {
		t.Fatalf("declared agent should not warn:\n%s", c.err.String())
	}
	// Undeclared agent → stderr warning, but command still succeeds.
	c.out.Reset()
	c.err.Reset()
	id := strings.TrimSpace(c.run("create", "-a", "ghost", "spook"))
	if !strings.HasPrefix(id, "clu-") {
		t.Fatalf("create should still succeed:\n%s", id)
	}
	if !strings.Contains(c.err.String(), "ghost") || !strings.Contains(c.err.String(), "not declared") {
		t.Fatalf("expected warning on stderr:\n%s", c.err.String())
	}
	// --quiet suppresses the warning.
	c.out.Reset()
	c.err.Reset()
	c.run("--quiet", "create", "-a", "ghost", "quiet spook")
	if strings.Contains(c.err.String(), "warning") {
		t.Fatalf("--quiet should suppress warning:\n%s", c.err.String())
	}
}

func TestCLIListByAgentMatchesAssigneeToo(t *testing.T) {
	// Regression (pre-v13): with the old agent/assignee split, `clu
	// assign <id> X` set only assignee and `clu list -a X` missed it.
	// Post-v13 there's one assignee column and this is naturally the
	// case; the test guards against re-splitting.
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "needs a fix"))
	c.run("assign", id, "bug-fixer")
	out := c.run("list", "-a", "bug-fixer")
	if !strings.Contains(out, id) {
		t.Fatalf("expected -a to match assignee=bug-fixer too:\n%s", out)
	}
	// And the bare-lane case (-a not set) still works for non-bug-fixer work.
	other := strings.TrimSpace(c.run("create", "in default lane"))
	out = c.run("list")
	if !strings.Contains(out, other) {
		t.Fatalf("bare list should still show default-lane issues:\n%s", out)
	}
}

func TestCLIReadyWatchEmitsUnblockedOnly(t *testing.T) {
	// `ready --watch` must show only unblocked, unassigned issues.
	// A blocked child must NOT appear; closing the parent should make
	// it appear on the next tick.
	c := newTestCLI(t)
	c.run("init")
	parent := strings.TrimSpace(c.run("create", "parent"))
	child := strings.TrimSpace(c.run("create", "child"))
	c.run("link", child, parent)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	c.ctx = ctx

	go func() {
		time.Sleep(80 * time.Millisecond)
		c2 := &testCLI{t: t, dir: c.dir, ctx: context.Background()}
		c2.run("close", parent)
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		full := []string{"--dir", c.dir, "ready", "--watch", "--interval", "20ms"}
		_ = Run(c.ctx, &c.out, &c.err, full)
	}()
	wg.Wait()

	out := c.out.String()
	// Parent was open and unblocked initially → present.
	if !strings.Contains(out, parent) {
		t.Fatalf("parent should appear initially:\n%s", out)
	}
	// Child appears only after parent closes.
	if !strings.Contains(out, child) {
		t.Fatalf("child should appear after parent closes:\n%s", out)
	}
}

func TestCLIReadyWatchRejectsJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("--json", "ready", "--watch", "--interval", "10ms")
}

func TestCLIReadyWaitAndWatchMutuallyExclusive(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("ready", "--wait", "--watch", "--interval", "10ms")
}

func TestCLIBriefJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	writeAgentsConfig(t, c.dir, `
id_prefix: clu-
agents:
  doc-writer:
    description: "Writes docs"
    capabilities: [docs]
`)
	out := c.run("--json", "brief")
	var b struct {
		Manual string `json:"manual"`
		Agents []struct {
			Name         string   `json:"name"`
			Description  string   `json:"description"`
			Capabilities []string `json:"capabilities"`
		} `json:"agents"`
		Active   []any `json:"active"`
		Memories []any `json:"memories"`
	}
	if err := json.Unmarshal([]byte(out), &b); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if !strings.Contains(b.Manual, "claim --agent") {
		t.Fatalf("manual missing key content: %s", b.Manual[:200])
	}
	if len(b.Agents) != 1 || b.Agents[0].Name != "doc-writer" {
		t.Fatalf("expected one declared agent doc-writer, got %+v", b.Agents)
	}
}

func TestCLILabelPropagateDirect(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	parent := strings.TrimSpace(c.run("create", "epic"))
	a := strings.TrimSpace(c.run("create", "subtask a", "--dep", parent))
	b := strings.TrimSpace(c.run("create", "subtask b", "--dep", parent))
	grandchild := strings.TrimSpace(c.run("create", "deep", "--dep", b))

	// Pre-seed: a already has branch:foo so the propagate must skip it.
	c.run("label", "add", a, "branch:foo")

	out := c.run("label", "propagate", parent, "branch:foo", "perf")
	if !strings.Contains(out, "2 direct child(ren)") {
		t.Fatalf("expected 2 direct child summary:\n%s", out)
	}
	if !strings.Contains(out, "branch:foo: 1 added, 1 already present") {
		t.Fatalf("expected per-label skip count:\n%s", out)
	}

	// a got perf (had branch:foo); b got both.
	out = c.run("label", "ls", a)
	if !strings.Contains(out, "branch:foo") || !strings.Contains(out, "perf") {
		t.Fatalf("a should have both labels:\n%s", out)
	}
	out = c.run("label", "ls", b)
	if !strings.Contains(out, "branch:foo") || !strings.Contains(out, "perf") {
		t.Fatalf("b should have both labels:\n%s", out)
	}
	// Direct mode does NOT touch the grandchild.
	out = c.run("label", "ls", grandchild)
	if strings.Contains(out, "branch:foo") || strings.Contains(out, "perf") {
		t.Fatalf("direct propagate must not reach grandchild:\n%s", out)
	}
}

func TestCLILabelPropagateDeep(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	parent := strings.TrimSpace(c.run("create", "epic"))
	mid := strings.TrimSpace(c.run("create", "mid", "--dep", parent))
	leaf := strings.TrimSpace(c.run("create", "leaf", "--dep", mid))

	out := c.run("label", "propagate", "--deep", parent, "branch:foo")
	if !strings.Contains(out, "2 descendant(s)") {
		t.Fatalf("expected 2 descendant summary:\n%s", out)
	}
	for _, id := range []string{mid, leaf} {
		out := c.run("label", "ls", id)
		if !strings.Contains(out, "branch:foo") {
			t.Fatalf("expected %s to have branch:foo:\n%s", id, out)
		}
	}
}

func TestCLILabelPropagateJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	parent := strings.TrimSpace(c.run("create", "epic"))
	child := strings.TrimSpace(c.run("create", "child", "--dep", parent))

	out := c.run("--json", "label", "propagate", parent, "branch:foo")
	var body struct {
		Parent   string   `json:"parent"`
		Deep     bool     `json:"deep"`
		Labels   []string `json:"labels"`
		Children []string `json:"children"`
		Results  []struct {
			ID      string   `json:"id"`
			Added   []string `json:"added"`
			Skipped []string `json:"skipped"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if body.Parent != parent || body.Deep {
		t.Fatalf("wrong header: %+v", body)
	}
	if len(body.Results) != 1 || body.Results[0].ID != child {
		t.Fatalf("expected one result for child %s: %+v", child, body.Results)
	}
	if len(body.Results[0].Added) != 1 || body.Results[0].Added[0] != "branch:foo" {
		t.Fatalf("expected branch:foo in added: %+v", body.Results[0])
	}
	// Empty `skipped` must serialize as [] not null.
	if body.Results[0].Skipped == nil {
		t.Fatalf("skipped should be empty []; got null")
	}
}

func TestCLILabelPropagateNoChildren(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "lonely"))
	out := c.run("label", "propagate", id, "branch:foo")
	if !strings.Contains(out, "no direct child(ren)") {
		t.Fatalf("expected no-children notice:\n%s", out)
	}
}

func TestCLISqlReadDefault(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "-p", "1", "hello"))
	out := c.run("sql", "SELECT id, priority FROM issues")
	if !strings.Contains(out, id) || !strings.Contains(out, "1") {
		t.Fatalf("expected id and priority in output:\n%s", out)
	}
	if !strings.Contains(out, "id") || !strings.Contains(out, "priority") {
		t.Fatalf("expected header columns:\n%s", out)
	}
}

func TestCLISqlReadOnlyRefusesWrite(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "hello")
	c.runFail("sql", "UPDATE issues SET priority=5")
	c.runFail("sql", "DROP TABLE issues")
	// After failed writes, the row is still untouched.
	out := c.run("sql", "SELECT COUNT(*) FROM issues")
	if !strings.Contains(out, "1") {
		t.Fatalf("expected count=1 after refused writes:\n%s", out)
	}
}

func TestCLISqlWriteFlag(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "hello")
	out := c.run("sql", "--write", "UPDATE issues SET priority=9")
	if !strings.Contains(out, "1 row(s) affected") {
		t.Fatalf("expected affected-rows summary:\n%s", out)
	}
	out = c.run("sql", "SELECT priority FROM issues")
	if !strings.Contains(out, "9") {
		t.Fatalf("expected priority=9 after --write:\n%s", out)
	}
}

func TestCLISqlCSV(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "first")
	c.run("create", "-a", "foo", "second")
	out := c.run("sql", "--csv", "SELECT priority, assignee, title FROM issues ORDER BY title")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 CSV lines (header + 2 rows), got %d:\n%s", len(lines), out)
	}
	if lines[0] != "priority,assignee,title" {
		t.Fatalf("expected CSV header, got %q", lines[0])
	}
	// NULL renders as empty in CSV.
	if !strings.Contains(lines[1], ",,first") {
		t.Fatalf("expected empty-string for NULL assignee:\n%s", out)
	}
}

func TestCLISqlJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "-p", "0", "a")
	out := c.run("--json", "sql", "SELECT id, priority FROM issues")
	// Top-level is a JSON array.
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Fatalf("expected JSON array:\n%s", out)
	}
	if !strings.Contains(out, `"priority":0`) {
		t.Fatalf("expected priority field:\n%s", out)
	}
}

func TestCLISqlPragmaAllowed(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	out := c.run("sql", "PRAGMA user_version")
	if !strings.Contains(out, "user_version") {
		t.Fatalf("expected PRAGMA result:\n%s", out)
	}
}

func TestCLISqlCSVAndJSONExclusive(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("--json", "sql", "--csv", "SELECT 1")
}

// initGitRepo creates a fresh git repo with a tiny initial commit at
// the given path. Used by worktree tests to set up a realistic source.
func initGitRepo(t *testing.T, path string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@t.t"},
		{"config", "user.name", "t"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = path
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(path, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, ".gitignore"), []byte(".env\n.clu/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = path
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// runInDir executes one clu invocation with cwd set, since worktree
// behavior depends on cwd-relative git discovery.
func runInDir(t *testing.T, dir string, args ...string) string {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(prev) }()
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	code := Run(context.Background(), out, errBuf, args)
	if code != 0 {
		t.Fatalf("clu %v in %s: exit %d\nstdout:%s\nstderr:%s", args, dir, code, out.String(), errBuf.String())
	}
	return out.String() + errBuf.String()
}

func TestCLIWorktreeAddBootstrap(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	main := t.TempDir()
	initGitRepo(t, main)

	// Set up gitignored files + a recipe that copies them and runs a
	// command that writes a sentinel file in the new worktree.
	if err := os.WriteFile(filepath.Join(main, ".env"), []byte("LOCAL=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runInDir(t, main, "init")
	if err := os.WriteFile(filepath.Join(main, ".clu", "config.yaml"), []byte(`id_prefix: clu-
worktree:
  copy:
    - .env
  commands:
    - 'touch BOOTSTRAPPED'
`), 0o644); err != nil {
		t.Fatal(err)
	}

	wt := filepath.Join(filepath.Dir(main), "wt-"+filepath.Base(main))
	defer os.RemoveAll(wt)
	runInDir(t, main, "worktree", "add", wt, "-b", "feat/x", "--bootstrap")

	// Copied file lands at the right path.
	if data, err := os.ReadFile(filepath.Join(wt, ".env")); err != nil {
		t.Fatalf("expected .env in worktree: %v", err)
	} else if string(data) != "LOCAL=1\n" {
		t.Fatalf("bad .env contents: %q", data)
	}
	// Command ran (sentinel created in the worktree dir).
	if _, err := os.Stat(filepath.Join(wt, "BOOTSTRAPPED")); err != nil {
		t.Fatalf("expected BOOTSTRAPPED sentinel: %v", err)
	}
}

func TestCLIWorktreeAddNoBootstrap(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	main := t.TempDir()
	initGitRepo(t, main)
	runInDir(t, main, "init")
	if err := os.WriteFile(filepath.Join(main, ".clu", "config.yaml"), []byte(`id_prefix: clu-
worktree:
  commands:
    - 'touch BOOTSTRAPPED'
`), 0o644); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(filepath.Dir(main), "no-bootstrap-"+filepath.Base(main))
	defer os.RemoveAll(wt)
	runInDir(t, main, "worktree", "add", wt, "-b", "feat/y")
	// Without --bootstrap, the recipe should NOT have run.
	if _, err := os.Stat(filepath.Join(wt, "BOOTSTRAPPED")); err == nil {
		t.Fatalf("BOOTSTRAPPED sentinel exists; bootstrap ran without flag")
	}
}

func TestCLIWorktreeSharedDB(t *testing.T) {
	// Issues created from a secondary worktree must land in the main
	// worktree's DB — that's the auto-resolveCluDir contract.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	main := t.TempDir()
	initGitRepo(t, main)
	runInDir(t, main, "init")
	idMain := strings.TrimSpace(runInDir(t, main, "create", "from main"))

	wt := filepath.Join(filepath.Dir(main), "shared-"+filepath.Base(main))
	defer os.RemoveAll(wt)
	runInDir(t, main, "worktree", "add", wt, "-b", "feat/share")

	// From the worktree (no .clu/ locally), creating an issue must
	// write to main's DB.
	idWt := strings.TrimSpace(runInDir(t, wt, "create", "from worktree"))
	if idWt == "" || idWt == idMain {
		t.Fatalf("expected a new id from worktree, got %q (main was %q)", idWt, idMain)
	}
	// And listing from main sees both.
	out := runInDir(t, main, "list")
	if !strings.Contains(out, idMain) || !strings.Contains(out, idWt) {
		t.Fatalf("main should see both ids %s, %s:\n%s", idMain, idWt, out)
	}
}

func TestCLIWorktreeBootstrapNoConfig(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	main := t.TempDir()
	initGitRepo(t, main)
	runInDir(t, main, "init") // default config — no worktree section
	wt := filepath.Join(filepath.Dir(main), "empty-"+filepath.Base(main))
	defer os.RemoveAll(wt)
	runInDir(t, main, "worktree", "add", wt, "-b", "feat/z")
	out := runInDir(t, main, "worktree", "bootstrap", wt)
	if !strings.Contains(out, "nothing to do") {
		t.Fatalf("expected 'nothing to do' notice, got:\n%s", out)
	}
}

func TestCLIWorktreeRemoveBlocksUncommitted(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	main := t.TempDir()
	initGitRepo(t, main)
	runInDir(t, main, "init")
	wt := filepath.Join(filepath.Dir(main), "rm-dirty-"+filepath.Base(main))
	defer os.RemoveAll(wt)
	runInDir(t, main, "worktree", "add", wt, "-b", "feat/dirty")
	if err := os.WriteFile(filepath.Join(wt, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	prev, _ := os.Getwd()
	defer func() { _ = os.Chdir(prev) }()
	_ = os.Chdir(main)
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	code := Run(context.Background(), out, errBuf, []string{"worktree", "remove", wt})
	if code == 0 {
		t.Fatalf("expected failure with uncommitted changes; got 0")
	}
	if !strings.Contains(errBuf.String(), "uncommitted changes") {
		t.Fatalf("expected uncommitted-changes error; got:\n%s", errBuf.String())
	}
	// Worktree still exists.
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("worktree should still exist after refused remove: %v", err)
	}
}

func TestCLIWorktreeRemoveNoUpstreamIsNotice(t *testing.T) {
	// No-upstream is informational, not blocking — the branch ref
	// survives the worktree removal, so there's nothing actually lost.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	main := t.TempDir()
	initGitRepo(t, main)
	runInDir(t, main, "init")
	wt := filepath.Join(filepath.Dir(main), "rm-noup-"+filepath.Base(main))
	defer os.RemoveAll(wt)
	runInDir(t, main, "worktree", "add", wt, "-b", "feat/noup")
	out := runInDir(t, main, "worktree", "remove", wt)
	if !strings.Contains(out, "no upstream") {
		t.Fatalf("expected no-upstream notice; got:\n%s", out)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree should be gone; stat: %v", err)
	}
}

func TestCLIWorktreeRemoveForce(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	main := t.TempDir()
	initGitRepo(t, main)
	runInDir(t, main, "init")
	wt := filepath.Join(filepath.Dir(main), "rm-force-"+filepath.Base(main))
	defer os.RemoveAll(wt)
	runInDir(t, main, "worktree", "add", wt, "-b", "feat/force")
	// Make it dirty so the regular path would refuse.
	if err := os.WriteFile(filepath.Join(wt, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runInDir(t, main, "worktree", "remove", "--force", wt)
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree should be gone after --force; stat: %v", err)
	}
}

func TestCLIWorktreeRemoveRefusesMain(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	main := t.TempDir()
	initGitRepo(t, main)
	runInDir(t, main, "init")
	prev, _ := os.Getwd()
	defer func() { _ = os.Chdir(prev) }()
	_ = os.Chdir(main)
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	code := Run(context.Background(), out, errBuf, []string{"worktree", "remove", main})
	if code == 0 {
		t.Fatalf("expected failure removing main worktree; got 0")
	}
	if !strings.Contains(errBuf.String(), "main worktree") {
		t.Fatalf("expected main-worktree error; got:\n%s", errBuf.String())
	}
}

func TestCLIHistoryAndLog(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "-p", "2", "trackme", "task"))
	c.run("claim", "-a", "worker", id)
	c.run("close", id)

	// history: per-issue timeline, oldest first.
	hist := c.run("history", id)
	for _, want := range []string{"created", "claimed", "closed"} {
		if !strings.Contains(hist, want) {
			t.Fatalf("history missing %q:\n%s", want, hist)
		}
	}
	if idx0, idxC := strings.Index(hist, "created"), strings.Index(hist, "closed"); idx0 > idxC {
		t.Fatalf("history not oldest-first:\n%s", hist)
	}
	// The claimer is recorded as the actor on the claim event.
	if !strings.Contains(hist, "worker") {
		t.Fatalf("history missing claimer actor:\n%s", hist)
	}

	// log --json: one array, payload is a nested object not a string.
	out := c.run("--json", "log", "--issue", id)
	var evs []map[string]any
	if err := json.Unmarshal([]byte(out), &evs); err != nil {
		t.Fatalf("log --json not valid array: %v\n%s", err, out)
	}
	if len(evs) != 3 {
		t.Fatalf("expected 3 events, got %d:\n%s", len(evs), out)
	}
	// Newest-first: closed leads.
	if evs[0]["kind"] != "closed" {
		t.Fatalf("expected newest-first (closed leads), got %v", evs[0]["kind"])
	}

	// log --kind filter.
	claimed := c.run("log", "--kind", "claimed")
	if !strings.Contains(claimed, "claimed") || strings.Contains(claimed, "closed") {
		t.Fatalf("kind filter leaked other kinds:\n%s", claimed)
	}
}

func TestCLILabelJSONContract(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "json labels", "task"))

	// label ls --json on an empty set → [] (not human "(none)").
	out := c.run("--json", "label", "ls", id)
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("empty label ls --json = %q, want []", strings.TrimSpace(out))
	}

	c.run("label", "add", id, "one")
	// label ls --json → JSON array.
	var labels []string
	if err := json.Unmarshal([]byte(c.run("--json", "label", "ls", id)), &labels); err != nil {
		t.Fatalf("label ls --json not an array: %v", err)
	}
	if len(labels) != 1 || labels[0] != "one" {
		t.Fatalf("label ls --json = %v", labels)
	}

	// label rm --json → an issue object (not empty output).
	rmOut := c.run("--json", "label", "rm", id, "one")
	var obj map[string]any
	if err := json.Unmarshal([]byte(rmOut), &obj); err != nil {
		t.Fatalf("label rm --json not an object: %v\n%q", err, rmOut)
	}
	if obj["id"] != id {
		t.Fatalf("label rm --json missing id: %v", obj)
	}
}

func TestCLICommentLsJSONEmptyIsArray(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "no comments", "task"))
	out := strings.TrimSpace(c.run("--json", "comment", "ls", id))
	if out != "[]" {
		t.Fatalf("empty comment ls --json = %q, want []", out)
	}
}

// assertOneJSON fails unless s is exactly one valid JSON value.
func assertOneJSON(t *testing.T, label, s string) {
	t.Helper()
	s = strings.TrimSpace(s)
	if s == "" {
		t.Fatalf("%s: --json emitted nothing", label)
	}
	var v any
	dec := json.NewDecoder(strings.NewReader(s))
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%s: not valid JSON: %v\n%q", label, err, s)
	}
	if dec.More() {
		t.Fatalf("%s: more than one JSON value\n%q", label, s)
	}
}

func TestCLIJSONContractSweep(t *testing.T) {
	c := newTestCLI(t)

	// init --json
	assertOneJSON(t, "init", c.run("--json", "init"))

	id := strings.TrimSpace(c.run("create", "sweep target", "task"))
	id2 := strings.TrimSpace(c.run("create", "sweep parent", "task"))

	// sugar verbs → affected issue
	assertOneJSON(t, "assign", c.run("--json", "assign", id, "alice"))
	assertOneJSON(t, "priority", c.run("--json", "priority", id, "1"))
	assertOneJSON(t, "describe", c.run("--json", "describe", id, "some desc"))
	assertOneJSON(t, "tag", c.run("--json", "tag", id, "blue"))
	assertOneJSON(t, "link", c.run("--json", "link", id, id2))
	assertOneJSON(t, "note-append", c.run("--json", "note", "append", id, "a note"))

	// deletes / mutations → result objects
	assertOneJSON(t, "dep-rm", c.run("--json", "dep", "rm", id, id2))
	cid := strings.TrimSpace(c.run("--json", "comment", "add", id, "hi"))
	_ = cid
	cmid := func() int64 {
		var cm map[string]any
		_ = json.Unmarshal([]byte(c.run("--json", "comment", "add", id, "x")), &cm)
		return int64(cm["id"].(float64))
	}()
	assertOneJSON(t, "comment-rm", c.run("--json", "comment", "rm", "-a", "alice", fmt.Sprintf("%d", cmid)))

	c.run("kv", "set", "k", "v")
	assertOneJSON(t, "kv-clear", c.run("--json", "kv", "clear", "k"))

	c.run("cron", "add", "nightly", "--schedule", "@daily", "--", "create", "x")
	assertOneJSON(t, "cron-disable", c.run("--json", "cron", "disable", "nightly"))
	assertOneJSON(t, "cron-enable", c.run("--json", "cron", "enable", "nightly"))
	assertOneJSON(t, "cron-rm", c.run("--json", "cron", "rm", "nightly"))

	c.run("lock", "deploy", "--ttl", "1m", "-a", "alice")
	assertOneJSON(t, "unlock", c.run("--json", "unlock", "deploy", "-a", "alice"))

	assertOneJSON(t, "completion", c.run("--json", "completion", "bash"))

	// import --json (round-trips an export)
	exp := filepath.Join(t.TempDir(), "e.jsonl")
	c.run("export", "-o", exp)
	assertOneJSON(t, "import", c.run("--json", "import", exp))
}

func TestCLIJSONRejectedWhereStreaming(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	// export is JSONL by design → --json rejected.
	c.runFail("--json", "export")
	// lock with a trailing command streams child stdout → --json rejected.
	c.runFail("--json", "lock", "deploy", "--ttl", "1m", "--", "echo", "hi")
}

// writeBatchFile writes a graph JSON doc to a temp file and returns its path.
func writeBatchFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCLIBatchArrayForm(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	graph := writeBatchFile(t, `[
	  {"alias":"a","title":"Task A","priority":1},
	  {"alias":"b","title":"Task B","needs":["a"],"capabilities":["go"]},
	  {"alias":"c","title":"Task C","needs":["a","b"]}
	]`)
	out := c.run("--json", "batch", graph)
	var res struct {
		Count   int               `json:"count"`
		Edges   int               `json:"edges"`
		Created map[string]string `json:"created"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("batch --json: %v\n%s", err, out)
	}
	if res.Count != 3 || res.Edges != 3 {
		t.Fatalf("count/edges = %d/%d, want 3/3", res.Count, res.Edges)
	}
	if len(res.Created) != 3 {
		t.Fatalf("created map = %v", res.Created)
	}
	// The graph really exists: c depends on a and b.
	show := c.run("--json", "show", res.Created["c"])
	for _, id := range []string{res.Created["a"], res.Created["b"]} {
		if !strings.Contains(show, id) {
			t.Fatalf("c should depend on %s:\n%s", id, show)
		}
	}
	// cap label routed onto b.
	labels := c.run("--json", "label", "ls", res.Created["b"])
	if !strings.Contains(labels, "cap:go") {
		t.Fatalf("b should carry cap:go:\n%s", labels)
	}
}

func TestCLIBatchDocFormAndDryRun(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	graph := writeBatchFile(t, `{"issues":[{"alias":"x","title":"X"},{"alias":"y","title":"Y","needs":["x"]}]}`)

	// Dry-run writes nothing.
	c.run("batch", "--dry-run", graph)
	listOut := c.run("--json", "list", "--status", "all")
	if !strings.Contains(listOut, "[]") {
		t.Fatalf("dry-run should have created nothing:\n%s", listOut)
	}

	// Real run creates them.
	c.run("batch", graph)
	listOut = c.run("--json", "list", "--status", "all")
	if strings.Contains(strings.TrimSpace(listOut), "[]") {
		t.Fatalf("batch should have created issues:\n%s", listOut)
	}
}

func TestCLIBatchRejectsBadInput(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	// Unknown field (typo) must fail loudly.
	c.runFail("batch", writeBatchFile(t, `[{"alias":"a","title":"A","capabilites":["go"]}]`))
	// Cycle must fail.
	c.runFail("batch", writeBatchFile(t, `[{"alias":"a","title":"A","needs":["b"]},{"alias":"b","title":"B","needs":["a"]}]`))
	// Invalid capability charset must fail.
	c.runFail("batch", writeBatchFile(t, `[{"alias":"a","title":"A","capabilities":["Go Lang"]}]`))
}

func TestCLIBatchGroup(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")

	// --group flag creates a parent + run: label on everything.
	graph := writeBatchFile(t, `[{"alias":"a","title":"A"},{"alias":"b","title":"B","needs":["a"]}]`)
	out := c.run("--json", "batch", "--group", "Sprint 7", graph)
	var res struct {
		Count   int               `json:"count"`
		Group   string            `json:"group"`
		Created map[string]string `json:"created"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("batch --json: %v\n%s", err, out)
	}
	if res.Group == "" {
		t.Fatalf("expected group parent id:\n%s", out)
	}
	// run:<parent> matches parent + 2 issues = 3.
	list := c.run("--json", "list", "--status", "all", "-l", "run:"+res.Group)
	var items []map[string]any
	if err := json.Unmarshal([]byte(list), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("run label should match 3 (parent+2), got %d:\n%s", len(items), list)
	}

	// Document "group" string form also works.
	graph2 := writeBatchFile(t, `{"group":"From Doc","issues":[{"alias":"x","title":"X"}]}`)
	out2 := c.run("--json", "batch", graph2)
	if !strings.Contains(out2, `"group"`) {
		t.Fatalf("doc group should produce a parent:\n%s", out2)
	}
}

func TestCLIBatchRejectsTrailingContent(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	// Valid array followed by extra JSON must be rejected, not committed.
	c.runFail("batch", writeBatchFile(t, `[{"alias":"a","title":"A"}] {"extra":true}`))
	// And nothing was written.
	out := c.run("--json", "list", "--status", "all")
	if !strings.Contains(strings.TrimSpace(out), "[]") {
		t.Fatalf("trailing-content batch should have created nothing:\n%s", out)
	}
}

func TestCLIBatchIdempotentImport(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	graph := writeBatchFile(t, `[
	  {"alias":"a","title":"Ticket A","key":"linear:ENG-1"},
	  {"alias":"b","title":"Ticket B","key":"linear:ENG-2"}
	]`)

	// First import: both new.
	out1 := c.run("--json", "batch", graph)
	var r1 struct{ New, Existing int }
	if err := json.Unmarshal([]byte(out1), &r1); err != nil {
		t.Fatal(err)
	}
	if r1.New != 2 || r1.Existing != 0 {
		t.Fatalf("run1 new/existing = %d/%d, want 2/0", r1.New, r1.Existing)
	}

	// Re-run (skip default): nothing new, no duplicates.
	out2 := c.run("--json", "batch", graph)
	var r2 struct{ New, Existing int }
	if err := json.Unmarshal([]byte(out2), &r2); err != nil {
		t.Fatal(err)
	}
	if r2.New != 0 || r2.Existing != 2 {
		t.Fatalf("run2 new/existing = %d/%d, want 0/2", r2.New, r2.Existing)
	}
	all := c.run("--json", "list", "--status", "all")
	var items []map[string]any
	if err := json.Unmarshal([]byte(all), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("re-run duplicated: %d issues, want 2", len(items))
	}

	// Re-run with --on-existing update: title change syncs.
	graph2 := writeBatchFile(t, `[{"alias":"a","title":"Ticket A (renamed)","key":"linear:ENG-1"}]`)
	out3 := c.run("--json", "batch", "--on-existing", "update", graph2)
	var r3 struct{ Updated int }
	if err := json.Unmarshal([]byte(out3), &r3); err != nil {
		t.Fatal(err)
	}
	if r3.Updated != 1 {
		t.Fatalf("update run updated = %d, want 1", r3.Updated)
	}
}

func TestCLIContextBundle(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	// upstream → downstream chain via batch (b needs a).
	graph := writeBatchFile(t, `[
	  {"alias":"a","title":"Design auth","description":"design the endpoint","notes":"chose JWT"},
	  {"alias":"b","title":"Implement auth","needs":["a"]}
	]`)
	out := c.run("--json", "batch", graph)
	var res struct{ Created map[string]string }
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	aID, bID := res.Created["a"], res.Created["b"]

	// Put a comment + close on the upstream task.
	c.run("comment", "add", aID, "-a", "alice", "endpoint shipped, edge case X handled")
	c.run("close", aID)

	// show b --context surfaces a's description/notes/comment.
	human := c.run("show", bID, "--context")
	for _, want := range []string{"Context", aID, "design the endpoint", "chose JWT", "endpoint shipped, edge case X handled"} {
		if !strings.Contains(human, want) {
			t.Fatalf("context output missing %q:\n%s", want, human)
		}
	}

	// --json wraps {issue, context}; context carries a's comments.
	j := c.run("--json", "show", bID, "--context")
	var doc struct {
		Issue   map[string]any `json:"issue"`
		Context []struct {
			ID       string `json:"id"`
			Comments []struct {
				Body string `json:"body"`
			} `json:"comments"`
		} `json:"context"`
	}
	if err := json.Unmarshal([]byte(j), &doc); err != nil {
		t.Fatalf("context json: %v\n%s", err, j)
	}
	if doc.Issue["id"] != bID {
		t.Fatalf("issue wrapper wrong: %v", doc.Issue["id"])
	}
	if len(doc.Context) != 1 || doc.Context[0].ID != aID {
		t.Fatalf("context should have 1 entry (a): %+v", doc.Context)
	}
	if len(doc.Context[0].Comments) != 1 || !strings.Contains(doc.Context[0].Comments[0].Body, "edge case X") {
		t.Fatalf("ancestor comment missing: %+v", doc.Context[0].Comments)
	}
}

func TestCLIBatchDocs(t *testing.T) {
	c := newTestCLI(t)
	// --docs works WITHOUT init (no DB needed) and covers the key surface.
	out := c.run("batch", "--docs")
	for _, want := range []string{
		"alias", "needs", "checkpoint", "key", "--on-existing", "--group",
		"--dry-run", "milestone", "EXAMPLE", "idempotent",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("--docs missing %q:\n%s", want, out)
		}
	}
	// The example should itself be valid JSON the parser accepts. Extract the
	// array between the first '[' and its matching trailing ']' line.
	start := strings.Index(out, "[\n")
	if start < 0 {
		t.Fatal("no example array found in --docs")
	}
}
