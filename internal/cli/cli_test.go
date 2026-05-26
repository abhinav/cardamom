package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testCLI bundles the args base ("--dir <tmp>/.db") with stdio buffers.
type testCLI struct {
	t   *testing.T
	dir string
	ctx context.Context
	out bytes.Buffer
	err bytes.Buffer
}

func newTestCLI(t *testing.T) *testCLI {
	t.Helper()
	return &testCLI{t: t, dir: filepath.Join(t.TempDir(), ".db"), ctx: context.Background()}
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
	if !strings.Contains(out, "Status:") || !strings.Contains(out, "Agents:") || !strings.Contains(out, "Types:") {
		t.Fatalf("stats missing sections:\n%s", out)
	}
	if !strings.Contains(out, "<none>") || !strings.Contains(out, "code-reviewer") {
		t.Fatalf("stats missing agent grouping:\n%s", out)
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
	if !strings.HasPrefix(out, "cli ") {
		t.Fatalf("expected -V to print version: %q", out)
	}
}

func TestCLICommentRoundTrip(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "x"))
	c.run("comment", "add", id, "first", "thought", "--as", "alice")
	c.run("comment", "add", id, "second", "thought", "--as", "bob")
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
	c.run("comment", "add", id, "delete me", "--as", "a")
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
	src.run("comment", "add", id, "first", "--as", "alice")
	src.run("comment", "add", id, "second", "--as", "bob")

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
	c.run("create", "-p", "5", "lo")
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
	c.run("claim", wip, "--as", "alice")

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
	out := c.run("claim", "--as", "alice")
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
	out := c.run("--json", "claim", "--as", "alice")
	if strings.HasPrefix(out, "claimed ") {
		t.Fatalf("notice leaked into --json output: %q", out)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(out), &row); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if row["status"] != "in_progress" || row["assignee"] != "alice" {
		t.Fatalf("missing claim mutations in JSON: %+v", row)
	}
}

func TestCLIVersion(t *testing.T) {
	c := newTestCLI(t)
	out := c.run("version")
	if !strings.HasPrefix(out, "cli ") {
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
	if !strings.Contains(out, "complete -F _cli_completions cli") {
		t.Fatalf("bash completion missing complete line:\n%s", out)
	}
}

func TestCLIListBadDate(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("list", "--created-after", "not-a-date")
}
