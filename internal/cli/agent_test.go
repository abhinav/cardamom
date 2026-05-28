package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// writeAgentsConfig replaces (or creates) config.yaml with an agents block.
func writeAgentsConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCLICreateWithCapabilityAttachesLabel(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "--capability", "go-review", "review the auth code"))
	out := c.run("label", "ls", id)
	if !strings.Contains(out, "cap:go-review") {
		t.Fatalf("expected cap:go-review label:\n%s", out)
	}
}

func TestCLIClaimDefaultIgnoresCapLabels(t *testing.T) {
	// Plain `claim` (no --agent) MUST NOT pick up cap:* labelled issues.
	c := newTestCLI(t)
	c.run("init")
	plain := strings.TrimSpace(c.run("create", "plain task"))
	gated := strings.TrimSpace(c.run("create", "--capability", "go-review", "gated task"))

	out := c.run("claim")
	if !strings.Contains(out, plain) {
		t.Fatalf("expected default claim to take the plain task:\n%s", out)
	}
	if strings.Contains(out, gated) {
		t.Fatalf("default claim should NOT take cap-labelled task:\n%s", out)
	}
}

func TestCLIClaimWithCapabilityPicksUpCapLabel(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	writeAgentsConfig(t, c.dir, `
id_prefix: clu-
agents:
  code-reviewer:
    description: "Reviews code"
    capabilities: [go-review]
`)
	gated := strings.TrimSpace(c.run("create", "--capability", "go-review", "gated task"))
	out := c.run("claim", "--agent", "code-reviewer")
	if !strings.Contains(out, gated) {
		t.Fatalf("expected code-reviewer to claim cap:go-review task:\n%s", out)
	}
}

func TestCLIClaimAgentLanePreservedWithoutCaps(t *testing.T) {
	// An ad-hoc agent (not declared in config) still works — `-a infra`
	// is both the lane filter AND the identity. The issue gets assignee=infra.
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "-a", "infra", "spot check"))
	out := c.run("claim", "-a", "infra")
	if !strings.Contains(out, id) {
		t.Fatalf("ad-hoc lane claim broke:\n%s", out)
	}
	if !strings.Contains(out, "Assignee: infra") {
		t.Fatalf("expected assignee=infra (the agent identity):\n%s", out)
	}
}

func TestCLIAgentLsShowsDeclaredAndActive(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	writeAgentsConfig(t, c.dir, `
id_prefix: clu-
agents:
  code-reviewer:
    description: "Reviews code"
    capabilities: [go-review]
  writer:
    description: "Writes docs"
    capabilities: [docs]
`)
	out := c.run("agent", "ls")
	// Both declared agents present, none active yet.
	for _, want := range []string{"code-reviewer", "writer", "go-review", "docs", "Reviews code"} {
		if !strings.Contains(out, want) {
			t.Fatalf("agent ls missing %q:\n%s", want, out)
		}
	}
	// Active markers absent.
	if strings.Contains(out, "●") {
		t.Fatalf("expected no active agents yet:\n%s", out)
	}
}

func TestCLIAgentLsShowsLiveHeartbeatFromClaim(t *testing.T) {
	// Run `claim --wait --agent code-reviewer --heartbeat` in a goroutine;
	// while it loops, `agent ls` should show it active.
	c := newTestCLI(t)
	c.run("init")
	writeAgentsConfig(t, c.dir, `
id_prefix: clu-
agents:
  code-reviewer:
    description: "Reviews code"
    capabilities: [go-review]
`)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		full := []string{"--dir", c.dir, "claim", "--wait", "--interval", "20ms", "--agent", "code-reviewer", "--heartbeat"}
		_ = Run(ctx, &c.out, &c.err, full)
	}()

	// Give the heartbeat one tick to land.
	time.Sleep(60 * time.Millisecond)

	checker := newTestCLI(t)
	checker.dir = c.dir // share the same DB
	out := checker.run("agent", "ls")
	cancel()
	wg.Wait()

	if !strings.Contains(out, "●") {
		t.Fatalf("expected code-reviewer marked active:\n%s", out)
	}
}

func TestCLIClaimWaitNoHeartbeatByDefault(t *testing.T) {
	// Without --heartbeat, the claim --wait loop must NOT show up in
	// `agent ls` — that was the surprise we removed.
	c := newTestCLI(t)
	c.run("init")
	writeAgentsConfig(t, c.dir, `
id_prefix: clu-
agents:
  code-reviewer:
    description: "Reviews code"
    capabilities: [go-review]
`)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		full := []string{"--dir", c.dir, "claim", "--wait", "--interval", "20ms", "--agent", "code-reviewer"}
		_ = Run(ctx, &c.out, &c.err, full)
	}()

	time.Sleep(80 * time.Millisecond)

	checker := newTestCLI(t)
	checker.dir = c.dir
	out := checker.run("agent", "ls")
	cancel()
	wg.Wait()

	if strings.Contains(out, "●") {
		t.Fatalf("default claim --wait must not heartbeat; agent ls shouldn't show active:\n%s", out)
	}
}

func TestCLIHeartbeatRequiresIdentity(t *testing.T) {
	// `ready --heartbeat` without `-a` should error — ready has no --agent
	// default; the lane name doubles as the agent identity.
	c := newTestCLI(t)
	c.run("init")
	c.runFail("ready", "--wait", "--heartbeat", "--interval", "20ms")
}

func TestCLIListWatchHeartbeatRequiresAgent(t *testing.T) {
	// `list --watch --heartbeat` without -a should error.
	c := newTestCLI(t)
	c.run("init")
	c.runFail("list", "--watch", "--heartbeat", "--interval", "20ms")
}

func TestCLIListAliasLs(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "a task"))
	out := c.run("ls")
	if !strings.Contains(out, id) {
		t.Fatalf("`ls` should behave like `list`:\n%s", out)
	}
}

func TestCLIAgentShowReportsPendingWork(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	writeAgentsConfig(t, c.dir, `
id_prefix: clu-
agents:
  code-reviewer:
    capabilities: [go-review]
`)
	c.run("create", "--capability", "go-review", "needs review")
	c.run("create", "--capability", "go-review", "also needs review")
	c.run("create", "unrelated work")

	out := c.run("agent", "show", "code-reviewer")
	if !strings.Contains(out, "Pending work: 2") {
		t.Fatalf("expected 2 pending for code-reviewer:\n%s", out)
	}
}

func TestCLIAgentShowJSONShape(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	writeAgentsConfig(t, c.dir, `
id_prefix: clu-
agents:
  code-reviewer:
    description: "Reviews code"
    capabilities: [go-review]
`)
	out := c.run("--json", "agent", "show", "code-reviewer")
	var row map[string]any
	if err := json.Unmarshal([]byte(out), &row); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if row["name"] != "code-reviewer" {
		t.Fatalf("wrong name: %+v", row)
	}
	caps, _ := row["capabilities"].([]any)
	if len(caps) != 1 || caps[0] != "go-review" {
		t.Fatalf("wrong capabilities: %+v", row)
	}
}

func TestCLIAgentShowFailsWhenAgentUnknown(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("agent", "show", "ghost")
}

func TestCLIAgentGcDropsStaleRows(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	// No heartbeat → no rows → gc reports nothing dropped, exits 0.
	out := c.run("agent", "gc")
	if !strings.Contains(out, "no stale rows") {
		t.Fatalf("expected 'no stale rows':\n%s", out)
	}
}

func TestCLIAgentStartPrint(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	writeAgentsConfig(t, c.dir, `
id_prefix: clu-
agents:
  reviewer:
    description: "Reviews code"
    capabilities: [go-review]
    command: claude
    prompts: [AGENTS.md, SOUL.md]
`)
	// Prompt files must exist for assembly to succeed.
	pdir := filepath.Join(c.dir, "agents", "reviewer")
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"AGENTS.md", "SOUL.md"} {
		if err := os.WriteFile(filepath.Join(pdir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out := c.run("agent", "start", "--print", "reviewer")
	if !strings.Contains(out, "claude --append-system-prompt") {
		t.Fatalf("expected claude default prompt flag:\n%s", out)
	}
	if !strings.Contains(out, "AGENTS.md") || !strings.Contains(out, "SOUL.md") {
		t.Fatalf("expected both prompt files in command:\n%s", out)
	}

	// JSON form carries the full argv.
	jout := c.run("--json", "agent", "start", "--print", "reviewer")
	var got struct {
		Command string   `json:"command"`
		Argv    []string `json:"argv"`
	}
	if err := json.Unmarshal([]byte(jout), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, jout)
	}
	if got.Command != "claude" || len(got.Argv) != 5 {
		t.Fatalf("unexpected argv: %+v", got)
	}
}

func TestCLIAgentStartErrors(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	writeAgentsConfig(t, c.dir, `
id_prefix: clu-
agents:
  nocmd:
    description: "no command"
`)
	c.runFail("agent", "start", "--print", "ghost")  // not declared
	c.runFail("agent", "start", "--print", "nocmd")  // no command set
}

func TestCLIAgentStartGlobsPromptDir(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	writeAgentsConfig(t, c.dir, `
id_prefix: clu-
agents:
  reviewer:
    command: claude
`)
	pdir := filepath.Join(c.dir, "agents", "reviewer")
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No `prompts:` in config → every *.md is picked up, sorted.
	for _, f := range []string{"02-second.md", "01-first.md", "ignore.txt"} {
		if err := os.WriteFile(filepath.Join(pdir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := c.run("agent", "start", "--print", "reviewer")
	if !strings.Contains(out, "01-first.md") || !strings.Contains(out, "02-second.md") {
		t.Fatalf("expected globbed md files:\n%s", out)
	}
	if strings.Contains(out, "ignore.txt") {
		t.Fatalf("non-md file leaked into command:\n%s", out)
	}
	if strings.Index(out, "01-first.md") > strings.Index(out, "02-second.md") {
		t.Fatalf("globbed prompts not sorted:\n%s", out)
	}
}

func TestCLIAgentStartSharedLayer(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	writeAgentsConfig(t, c.dir, `
id_prefix: clu-
agents:
  reviewer:
    command: claude
    prompts: [SOUL.md]
`)
	// Shared base files.
	shared := filepath.Join(c.dir, "agents", "_shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"AGENTS.md", "AUTONOMY.md"} {
		if err := os.WriteFile(filepath.Join(shared, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Per-agent persona.
	pdir := filepath.Join(c.dir, "agents", "reviewer")
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pdir, "SOUL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := c.run("agent", "start", "--print", "reviewer")
	// Shared files come first (sorted), persona last.
	iA := strings.Index(out, "_shared/AGENTS.md")
	iU := strings.Index(out, "_shared/AUTONOMY.md")
	iS := strings.Index(out, "reviewer/SOUL.md")
	if iA < 0 || iU < 0 || iS < 0 {
		t.Fatalf("missing a prompt file in command:\n%s", out)
	}
	if !(iA < iU && iU < iS) {
		t.Fatalf("expected shared-first, sorted, persona-last ordering:\n%s", out)
	}
}
