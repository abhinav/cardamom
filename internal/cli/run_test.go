package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rovak/clu/internal/workflow"
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
	dir := workflow.TemplatesPath(c.dir)
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

func TestCLIRunByPath(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")

	// Write a template *outside* .db/templates/ — only reachable by path.
	tmp := t.TempDir()
	path := filepath.Join(tmp, "ad-hoc.yaml")
	if err := os.WriteFile(path, []byte(releaseYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Name-lookup fails (template not in .db/templates/).
	c.runFail("run", "ad-hoc", "--var", "version=1.0.0", "--dry-run")

	// Path-lookup succeeds.
	out := c.run("run", path, "--var", "version=1.0.0", "--dry-run")
	if !strings.Contains(out, "Build 1.0.0") {
		t.Fatalf("path-overloaded run failed:\n%s", out)
	}

	// `template validate` accepts the same path form.
	c.run("template", "validate", path)
}

func TestCLIRunRejectsBadVar(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.writeTemplate("release.yaml", releaseYAML)
	c.runFail("run", "release", "--var", "version=notsemver")
	c.runFail("run", "release") // version required
	c.runFail("run", "nope", "--var", "version=1.0.0")
}

func TestCLIRunNoPromptFailsOnMissingVar(t *testing.T) {
	// --no-prompt should error rather than prompting/hanging when a
	// required var is missing. (Tests run with non-TTY stdin so the
	// flag is mostly belt-and-braces, but explicit is better.)
	c := newTestCLI(t)
	c.run("init")
	c.writeTemplate("release.yaml", releaseYAML)
	c.runFail("run", "release", "--no-prompt")
}

const releaseWithSpecYAML = `
name: release-with-spec
spec: |
  ## Project context
  Repo: {{repo}}
  Conventions: kebab-case files, no force-pushes.
vars:
  repo:
    required: true
  version:
    default: "1.0.0"
steps:
  - id: build
    title: "Build {{version}}"
    description: |
      Run the project's build command. Acceptance: artefact in dist/.
  - id: deploy
    title: "Deploy {{version}}"
    needs: [build]
`

func TestCLIRunThreadsSpecAndStepDescription(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.writeTemplate("rspec.yaml", releaseWithSpecYAML)
	c.run("run", "release-with-spec", "-v", "repo=acme/widget")

	// Find the build step's issue ID by step:build label.
	listOut := c.run("list", "--json", "-l", "step:build")
	var issues []map[string]any
	if err := json.Unmarshal([]byte(listOut), &issues); err != nil {
		t.Fatalf("list --json: %v: %s", err, listOut)
	}
	if len(issues) != 1 {
		t.Fatalf("expected one build step, got %d", len(issues))
	}
	id, _ := issues[0]["id"].(string)
	show := c.run("show", id)
	for _, want := range []string{
		"Repo: acme/widget",             // spec interpolated
		"Acceptance: artefact in dist/", // per-step description
		"---",                           // separator between spec and step body
	} {
		if !strings.Contains(show, want) {
			t.Fatalf("show missing %q:\n%s", want, show)
		}
	}
}
