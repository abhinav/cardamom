package cli

import (
	"encoding/json"
	"os/user"
	"strings"
	"testing"
)

const checkpointYAML = `
name: deploy
steps:
  - id: build
    title: build
  - id: gate-manual
    title: confirm
    type: checkpoint
    wait: { manual: true }
    needs: [build]
  - id: gate-approval
    title: prod-approve
    type: checkpoint
    wait: { approval: [%s] }
    needs: [gate-manual]
`

// currentUserName returns the same value as the CLI's currentUser().
func currentUserName(t *testing.T) string {
	t.Helper()
	u, err := user.Current()
	if err != nil || u.Username == "" {
		return "unknown"
	}
	return u.Username
}

// checkpointIDs returns the issue IDs for each step-id label in a run.
func checkpointIDs(t *testing.T, c *testCLI) map[string]string {
	t.Helper()
	listOut := c.run("list", "--status", "all", "--json")
	var issues []map[string]any
	if err := json.Unmarshal([]byte(listOut), &issues); err != nil {
		t.Fatalf("list: %v", err)
	}
	out := map[string]string{}
	for _, i := range issues {
		id, _ := i["id"].(string)
		labelsAny, _ := i["labels"].([]any)
		for _, la := range labelsAny {
			s, _ := la.(string)
			if strings.HasPrefix(s, "step:") {
				out[strings.TrimPrefix(s, "step:")] = id
			}
		}
	}
	return out
}

func TestCheckpointManual(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	user := currentUserName(t)
	c.writeTemplate("deploy.yaml", strings.Replace(checkpointYAML, "%s", user, 1))
	c.run("run", "deploy")

	ids := checkpointIDs(t, c)

	// Manual checkpoint blocks until passed; not type-restricted at all.
	c.run("close", ids["build"])

	// gate-manual should now be ready and is a checkpoint.
	ready := c.run("ready", "--json")
	if !strings.Contains(ready, ids["gate-manual"]) {
		t.Fatalf("gate-manual not ready: %s", ready)
	}

	// approve the manual gate
	c.run("approve", ids["gate-manual"])

	// after approval, gate-approval becomes ready
	ready = c.run("ready", "--json")
	if !strings.Contains(ready, ids["gate-approval"]) {
		t.Fatalf("gate-approval not ready after manual pass: %s", ready)
	}
}

func TestCheckpointApproval(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	user := currentUserName(t)
	c.writeTemplate("deploy.yaml", strings.Replace(checkpointYAML, "%s", user, 1))
	c.run("run", "deploy")
	ids := checkpointIDs(t, c)
	c.run("close", ids["build"])
	c.run("approve", ids["gate-manual"])
	// Approval gate — current user is the configured approver.
	c.run("checkpoint", "pass", ids["gate-approval"])

	show := c.run("show", ids["gate-approval"])
	if !strings.Contains(show, "Status:   closed") {
		t.Errorf("expected closed, got:\n%s", show)
	}
	if !strings.Contains(show, "checkpoint:passed") {
		t.Errorf("expected checkpoint:passed label, got:\n%s", show)
	}
}

func TestCheckpointPassRejectsBlockedGate(t *testing.T) {
	// Regression for the S1 in pass 4: `checkpoint pass` succeeded on
	// gates whose own predecessors were still open, so downstream
	// went ready without the gated work happening. Must refuse.
	c := newTestCLI(t)
	c.run("init")
	user := currentUserName(t)
	c.writeTemplate("deploy.yaml", strings.Replace(checkpointYAML, "%s", user, 1))
	c.run("run", "deploy")
	ids := checkpointIDs(t, c)
	// Skip closing the build step — gate-manual still has an unmet parent.
	c.runFail("approve", ids["gate-manual"])
	// Closing the predecessor unblocks the path and pass should now work.
	c.run("close", ids["build"])
	c.run("approve", ids["gate-manual"])
}

// TestCheckpointApprovalNoLongerRejectsWrongUser locks in the post-
// gate-removal behavior. Approver-list enforcement was dropped (this
// is a single-user local tool; the declared list is informational).
// Anyone can approve, regardless of whether they're on the list.
// Kept the test under its history-friendly name shape so the
// behavioral change is grep-able from the test name alone.
func TestCheckpointApprovalNoLongerRejectsWrongUser(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.writeTemplate("deploy.yaml", strings.Replace(checkpointYAML, "%s", "not-the-current-user", 1))
	c.run("run", "deploy")
	ids := checkpointIDs(t, c)
	c.run("close", ids["build"])
	c.run("approve", ids["gate-manual"])

	// approval gate's declared approver is "not-the-current-user".
	// Pre-change this would fail; now it succeeds — the list is
	// advisory, not a gate.
	c.run("approve", ids["gate-approval"])
}

func TestCheckpointFail(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.writeTemplate("deploy.yaml", strings.Replace(checkpointYAML, "%s", currentUserName(t), 1))
	c.run("run", "deploy")
	ids := checkpointIDs(t, c)
	c.run("close", ids["build"])
	c.run("checkpoint", "fail", ids["gate-manual"], "--reason", "scope creep")

	show := c.run("show", ids["gate-manual"])
	if !strings.Contains(show, "checkpoint:failed") {
		t.Errorf("expected checkpoint:failed label:\n%s", show)
	}
	if !strings.Contains(show, "scope creep") {
		t.Errorf("expected reason note:\n%s", show)
	}
	// Fail cancels the gate (not closes) so downstream stays blocked.
	// Cancel cascades, so the gate-approval step (which depends on
	// gate-manual) also goes to cancelled, and the deploy step below
	// it too.
	if !strings.Contains(show, "Status:   cancelled") {
		t.Errorf("expected gate to be cancelled after fail:\n%s", show)
	}
	for _, name := range []string{"gate-approval", "deploy"} {
		if id := ids[name]; id != "" {
			s := c.run("show", id)
			if !strings.Contains(s, "Status:   cancelled") {
				t.Errorf("expected %s to cascade-cancel after gate fail:\n%s", name, s)
			}
		}
	}
}

func TestApproveOnNonCheckpoint(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	id := strings.TrimSpace(c.run("create", "regular task"))
	c.runFail("approve", id)
}
