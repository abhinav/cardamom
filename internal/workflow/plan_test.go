package workflow

import (
	"os"
	"strings"
	"testing"
)

func contains(s, sub string) bool   { return strings.Contains(s, sub) }
func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}

func TestMakePlan(t *testing.T) {
	p := writeTemplate(t, "release.yaml", goodYAML)
	tmpl, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	plan, err := MakePlan(tmpl, map[string]string{"version": "1.2.3"})
	if err != nil {
		t.Fatalf("MakePlan: %v", err)
	}

	if plan.Parent.Title != "release stable 1.2.3" {
		t.Errorf("parent title = %q, want %q", plan.Parent.Title, "release stable 1.2.3")
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(plan.Steps))
	}

	byID := map[string]StepSpec{}
	for _, s := range plan.Steps {
		byID[s.StepID] = s
	}
	if byID["build"].Title != "Build 1.2.3" {
		t.Errorf("build title = %q", byID["build"].Title)
	}
	if byID["test"].Title != "Test 1.2.3 on stable" {
		t.Errorf("test title = %q", byID["test"].Title)
	}
	if byID["build"].Priority != 1 {
		t.Errorf("build priority = %d, want 1", byID["build"].Priority)
	}
	if byID["test"].Priority != 2 {
		t.Errorf("test priority = %d, want 2 (default)", byID["test"].Priority)
	}

	// IsLeaf: deploy has nothing depending on it.
	if !byID["deploy"].IsLeaf {
		t.Errorf("deploy should be a leaf")
	}
	if byID["build"].IsLeaf {
		t.Errorf("build should not be a leaf (test needs it)")
	}
}

func TestMakePlanSpecAndStepDescription(t *testing.T) {
	yaml := `
name: scaffold
spec: |
  Project: {{project}}
  Conventions: kebab-case files, tailwind v4, react 19.
vars:
  project:
    required: true
steps:
  - id: install
    title: install deps
    description: |
      Run pnpm install. Acceptance: lockfile committed.
  - id: build
    title: build
    needs: [install]
`
	p := writeTemplate(t, "scaffold.yaml", yaml)
	tmpl, _ := Load(p)
	plan, err := MakePlan(tmpl, map[string]string{"project": "demo"})
	if err != nil {
		t.Fatalf("MakePlan: %v", err)
	}
	if !contains(plan.Spec, "Project: demo") {
		t.Errorf("spec not interpolated: %q", plan.Spec)
	}
	if plan.Parent.Description != plan.Spec {
		t.Errorf("parent description should equal spec, got %q", plan.Parent.Description)
	}
	byID := map[string]StepSpec{}
	for _, s := range plan.Steps {
		byID[s.StepID] = s
	}
	install := byID["install"]
	// Description should contain spec + the step's own body.
	if !contains(install.Description, "Project: demo") {
		t.Errorf("install missing spec: %q", install.Description)
	}
	if !contains(install.Description, "Acceptance: lockfile committed") {
		t.Errorf("install missing step-specific body: %q", install.Description)
	}
	if !contains(install.Description, "---") {
		t.Errorf("install missing separator between spec and step body: %q", install.Description)
	}
	// A step with no description should still get the spec alone.
	build := byID["build"]
	if !contains(build.Description, "Project: demo") {
		t.Errorf("build missing spec: %q", build.Description)
	}
	if contains(build.Description, "---") {
		t.Errorf("build has separator but no step body: %q", build.Description)
	}
}

func TestLoadSpecFromExternalFile(t *testing.T) {
	dir := t.TempDir()
	specPath := dir + "/spec.md"
	if err := writeFile(specPath, "# Project spec\nUse foo."); err != nil {
		t.Fatal(err)
	}
	yaml := `
name: external
spec: "@spec.md"
steps:
  - id: a
    title: only step
`
	p := dir + "/external.yaml"
	if err := writeFile(p, yaml); err != nil {
		t.Fatal(err)
	}
	tmpl, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !contains(tmpl.Spec, "# Project spec") {
		t.Errorf("spec not loaded from file: %q", tmpl.Spec)
	}
}

func TestMakePlanCheckpoint(t *testing.T) {
	yaml := `
name: gate
steps:
  - id: build
    title: build
  - id: approve
    type: checkpoint
    title: approve
    wait: { approval: [alice] }
    needs: [build]
`
	p := writeTemplate(t, "gate.yaml", yaml)
	tmpl, _ := Load(p)
	plan, err := MakePlan(tmpl, nil)
	if err != nil {
		t.Fatalf("MakePlan: %v", err)
	}
	byID := map[string]StepSpec{}
	for _, s := range plan.Steps {
		byID[s.StepID] = s
	}
	cp := byID["approve"]
	if cp.Type != "checkpoint" {
		t.Errorf("type = %q, want checkpoint", cp.Type)
	}
	if cp.Wait == nil || len(cp.Wait.Approval) != 1 || cp.Wait.Approval[0] != "alice" {
		t.Errorf("wait = %+v", cp.Wait)
	}
}
