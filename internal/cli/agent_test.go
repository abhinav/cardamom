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
	// Plain `claim` (no --as) MUST NOT pick up cap:* labelled issues.
	c := newTestCLI(t)
	c.run("init")
	plain := strings.TrimSpace(c.run("create", "plain task"))
	gated := strings.TrimSpace(c.run("create", "--capability", "go-review", "gated task"))

	out := c.run("claim", "--as", "user1")
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
	out := c.run("claim", "--as", "code-reviewer")
	if !strings.Contains(out, gated) {
		t.Fatalf("expected code-reviewer to claim cap:go-review task:\n%s", out)
	}
}

func TestCLIClaimAgentLanePreservedWithoutCaps(t *testing.T) {
	// An ad-hoc agent (not in config) still claims its named lane.
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "-a", "infra", "spot check"))
	out := c.run("claim", "-a", "infra", "--as", "alice")
	if !strings.Contains(out, id) {
		t.Fatalf("ad-hoc lane claim broke:\n%s", out)
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
	// Run `claim --wait --as code-reviewer --heartbeat` in a goroutine;
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
		full := []string{"--dir", c.dir, "claim", "--wait", "--interval", "20ms", "--as", "code-reviewer", "--heartbeat"}
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
		full := []string{"--dir", c.dir, "claim", "--wait", "--interval", "20ms", "--as", "code-reviewer"}
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
	// `ready --heartbeat` without `-a` should error — ready has no --as
	// default; the lane name doubles as the agent identity.
	c := newTestCLI(t)
	c.run("init")
	c.runFail("ready", "--wait", "--heartbeat", "--interval", "20ms")
}

func TestCLIListHeartbeatRequiresAs(t *testing.T) {
	// `list --watch --heartbeat` without --as should error.
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
