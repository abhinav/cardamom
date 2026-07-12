package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodYAML = `
name: release
description: Standard release
vars:
  version:
    required: true
    pattern: '^\d+\.\d+\.\d+$'
  channel:
    default: stable
steps:
  - id: build
    title: "Build {{version}}"
    priority: 1
  - id: test
    title: "Test {{version}} on {{channel}}"
    needs: [build]
  - id: deploy
    title: "Deploy {{version}}"
    needs: [test]
`

func writeTemplate(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadAndValidate(t *testing.T) {
	p := writeTemplate(t, "release.yaml", goodYAML)
	tmpl, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tmpl.Name != "release" {
		t.Errorf("name = %q, want release", tmpl.Name)
	}
	if err := tmpl.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(tmpl.Steps) != 3 {
		t.Errorf("steps = %d, want 3", len(tmpl.Steps))
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{
			"no steps",
			`name: x`,
			"no steps",
		},
		{
			"bad step id",
			"name: x\nsteps:\n  - id: BadID\n    title: t\n",
			"invalid step id",
		},
		{
			"duplicate id",
			"name: x\nsteps:\n  - id: a\n    title: t\n  - id: a\n    title: t\n",
			"duplicate step id",
		},
		{
			"missing needs",
			"name: x\nsteps:\n  - id: a\n    title: t\n    needs: [ghost]\n",
			"unknown step",
		},
		{
			"self dep",
			"name: x\nsteps:\n  - id: a\n    title: t\n    needs: [a]\n",
			"cannot depend on itself",
		},
		{
			"cycle",
			"name: x\nsteps:\n  - id: a\n    title: t\n    needs: [b]\n  - id: b\n    title: t\n    needs: [a]\n",
			"cycle",
		},
		{
			"checkpoint without wait",
			"name: x\nsteps:\n  - id: a\n    title: t\n    type: checkpoint\n",
			"requires a 'wait'",
		},
		{
			"task with wait",
			"name: x\nsteps:\n  - id: a\n    title: t\n    wait:\n      manual: true\n",
			"only valid for checkpoint",
		},
		{
			"checkpoint with empty wait",
			"name: x\nsteps:\n  - id: a\n    title: t\n    type: checkpoint\n    wait: {}\n",
			"manual: true",
		},
		{
			"unknown type",
			"name: x\nsteps:\n  - id: a\n    title: t\n    type: dance\n",
			"unknown type",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeTemplate(t, "t.yaml", tc.yaml)
			tmpl, err := Load(p)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			err = tmpl.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestResolveVars(t *testing.T) {
	p := writeTemplate(t, "x.yaml", goodYAML)
	tmpl, _ := Load(p)

	vars, err := tmpl.ResolveVars(map[string]string{"version": "1.2.3"})
	if err != nil {
		t.Fatalf("ResolveVars: %v", err)
	}
	if vars["version"] != "1.2.3" || vars["channel"] != "stable" {
		t.Errorf("got %v", vars)
	}

	if _, err := tmpl.ResolveVars(nil); err == nil {
		t.Errorf("expected required-var error")
	}
	if _, err := tmpl.ResolveVars(map[string]string{"version": "bad"}); err == nil {
		t.Errorf("expected pattern mismatch error")
	}
	if _, err := tmpl.ResolveVars(map[string]string{"version": "1.0.0", "unknown": "x"}); err == nil {
		t.Errorf("expected unknown-var error")
	}
}

func TestInterpolate(t *testing.T) {
	out, err := Interpolate("Deploy {{version}} to {{channel}}", map[string]string{
		"version": "1.0.0",
		"channel": "stable",
	})
	if err != nil || out != "Deploy 1.0.0 to stable" {
		t.Errorf("got %q err=%v", out, err)
	}
	if _, err := Interpolate("{{missing}}", nil); err == nil {
		t.Errorf("expected unknown-var error")
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("name: a\nsteps:\n  - id: x\n    title: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yml"), []byte("name: b\nsteps:\n  - id: x\n    title: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, loadErrs, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loadErrs) != 0 {
		t.Errorf("unexpected load errors: %v", loadErrs)
	}
	if len(m) != 2 {
		t.Errorf("got %d templates, want 2", len(m))
	}

	// Missing dir → empty map, no error.
	m2, loadErrs2, err := LoadDir(filepath.Join(dir, "does-not-exist"))
	if err != nil || len(m2) != 0 || len(loadErrs2) != 0 {
		t.Errorf("missing dir: m=%v err=%v loadErrs=%v", m2, err, loadErrs2)
	}
}

func TestLoadDirIsolatesBrokenTemplates(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "good.yaml"), []byte("name: good\nsteps:\n  - id: x\n    title: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A template with an @-referenced spec that doesn't exist.
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("name: bad\nspec: \"@nope.md\"\nsteps: [{id: x, title: t}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, loadErrs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir should not return a fatal error when one file is bad: %v", err)
	}
	if _, ok := m["good"]; !ok {
		t.Errorf("good template should still load: %v", m)
	}
	if _, ok := m["bad"]; ok {
		t.Errorf("bad template should not be in the map: %v", m)
	}
	if len(loadErrs) != 1 || loadErrs[0].File != "bad.yaml" {
		t.Errorf("expected one LoadError for bad.yaml, got %v", loadErrs)
	}
}
