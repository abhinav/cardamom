package workflow

import "testing"

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
