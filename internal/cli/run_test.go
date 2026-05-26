package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const releaseYAML = `
name: release
description: Standard release
vars:
  version:
    required: true
    pattern: '^\d+\.\d+\.\d+$'
steps:
  - id: build
    title: "Build {{version}}"
  - id: test
    title: "Test {{version}}"
    needs: [build]
  - id: deploy
    title: "Deploy {{version}}"
    needs: [test]
`

func (c *testCLI) writeTemplate(name, body string) {
	c.t.Helper()
	dir := filepath.Join(c.dir, "templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		c.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		c.t.Fatal(err)
	}
}

func TestCLITemplateList(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.writeTemplate("release.yaml", releaseYAML)
	out := c.run("template", "ls")
	if !strings.Contains(out, "release") {
		t.Fatalf("template ls missing 'release': %q", out)
	}
}

func TestCLITemplateValidate(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.writeTemplate("release.yaml", releaseYAML)
	c.run("template", "validate", "release")

	c.writeTemplate("broken.yaml", "name: broken\nsteps:\n  - id: a\n    title: t\n    needs: [ghost]\n")
	c.runFail("template", "validate", "broken")
}

func TestCLIRunDryRun(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.writeTemplate("release.yaml", releaseYAML)
	out := c.run("run", "release", "--var", "version=1.2.3", "--dry-run")
	if !strings.Contains(out, "Build 1.2.3") {
		t.Fatalf("dry-run output missing interpolated title:\n%s", out)
	}
	// No issues should have been created.
	list := c.run("list", "--json")
	if !strings.HasPrefix(strings.TrimSpace(list), "[]") && !strings.HasPrefix(strings.TrimSpace(list), "null") {
		t.Fatalf("dry-run leaked issues: %s", list)
	}
}

func TestCLIRunCreatesIssues(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.writeTemplate("release.yaml", releaseYAML)
	c.run("run", "release", "--var", "version=1.2.3")

	// Should now have 4 issues: parent + 3 steps.
	listOut := c.run("list", "--json")
	var issues []map[string]any
	if err := json.Unmarshal([]byte(listOut), &issues); err != nil {
		t.Fatalf("list --json: %v: %s", err, listOut)
	}
	if len(issues) != 4 {
		t.Fatalf("expected 4 issues, got %d: %s", len(issues), listOut)
	}

	// Only 'build' should be ready (no open deps).
	ready := c.run("ready", "--json")
	var readyIssues []map[string]any
	if err := json.Unmarshal([]byte(ready), &readyIssues); err != nil {
		t.Fatalf("ready --json: %v", err)
	}
	if len(readyIssues) != 1 {
		t.Fatalf("expected 1 ready issue, got %d: %s", len(readyIssues), ready)
	}
	title, _ := readyIssues[0]["title"].(string)
	if title != "Build 1.2.3" {
		t.Errorf("ready title = %q, want Build 1.2.3", title)
	}
}

func TestCLIRunRejectsBadVar(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.writeTemplate("release.yaml", releaseYAML)
	c.runFail("run", "release", "--var", "version=notsemver")
	c.runFail("run", "release") // version required
	c.runFail("run", "nope", "--var", "version=1.0.0")
}
