package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func newTestEnv(t *testing.T) (*Env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	out := &bytes.Buffer{}
	errb := &bytes.Buffer{}
	env := &Env{
		Dir:    filepath.Join(dir, ".beads"),
		Stdout: out,
		Stderr: errb,
	}
	return env, out, errb
}

func run(t *testing.T, env *Env, out *bytes.Buffer, args ...string) string {
	t.Helper()
	out.Reset()
	code := Run(env, args)
	if code != 0 {
		t.Fatalf("bd %v exit %d", args, code)
	}
	return out.String()
}

func runFail(t *testing.T, env *Env, args ...string) {
	t.Helper()
	if code := Run(env, args); code == 0 {
		t.Fatalf("bd %v unexpectedly succeeded", args)
	}
}

func TestCLIInitAndCreate(t *testing.T) {
	env, out, _ := newTestEnv(t)
	run(t, env, out, "init")
	id := strings.TrimSpace(run(t, env, out, "create", "-p", "1", "first", "task"))
	if !strings.HasPrefix(id, "bd-") {
		t.Fatalf("expected id, got %q", id)
	}
	show := run(t, env, out, "show", id)
	if !strings.Contains(show, "first task") {
		t.Fatalf("show output missing title:\n%s", show)
	}
}

func TestCLIWithoutInitFails(t *testing.T) {
	env, _, _ := newTestEnv(t)
	runFail(t, env, "create", "untitled")
}

func TestCLIReadyAndClaim(t *testing.T) {
	env, out, _ := newTestEnv(t)
	run(t, env, out, "init")
	a := strings.TrimSpace(run(t, env, out, "create", "-p", "1", "task a"))
	b := strings.TrimSpace(run(t, env, out, "create", "-p", "1", "task b"))
	run(t, env, out, "dep", "add", b, a)

	ready := run(t, env, out, "ready")
	if !strings.Contains(ready, a) {
		t.Fatalf("a should be ready: %s", ready)
	}
	if strings.Contains(ready, b) {
		t.Fatalf("b should NOT be ready: %s", ready)
	}

	claimed := run(t, env, out, "claim", "--as", "alice")
	if !strings.Contains(claimed, a) {
		t.Fatalf("expected claim of %s, got %q", a, claimed)
	}

	run(t, env, out, "close", a)
	ready = run(t, env, out, "ready")
	if !strings.Contains(ready, b) {
		t.Fatalf("b should be ready after closing a: %s", ready)
	}
}

func TestCLIClaimNoneReady(t *testing.T) {
	env, out, _ := newTestEnv(t)
	run(t, env, out, "init")
	runFail(t, env, "claim", "--as", "alice")
}

func TestCLIUpdate(t *testing.T) {
	env, out, _ := newTestEnv(t)
	run(t, env, out, "init")
	id := strings.TrimSpace(run(t, env, out, "create", "todo"))
	run(t, env, out, "update", id, "-p", "0", "--title", "urgent thing")
	show := run(t, env, out, "show", id)
	if !strings.Contains(show, "urgent thing") || !strings.Contains(show, "Priority: 0") {
		t.Fatalf("update did not apply: %s", show)
	}
}

func TestCLIDepCycleRejected(t *testing.T) {
	env, out, _ := newTestEnv(t)
	run(t, env, out, "init")
	a := strings.TrimSpace(run(t, env, out, "create", "a"))
	b := strings.TrimSpace(run(t, env, out, "create", "b"))
	run(t, env, out, "dep", "add", b, a)
	runFail(t, env, "dep", "add", a, b)
}

func TestCLIList(t *testing.T) {
	env, out, _ := newTestEnv(t)
	run(t, env, out, "init")
	a := strings.TrimSpace(run(t, env, out, "create", "open one"))
	b := strings.TrimSpace(run(t, env, out, "create", "to close"))
	run(t, env, out, "close", b)
	openList := run(t, env, out, "list")
	if !strings.Contains(openList, a) || strings.Contains(openList, b) {
		t.Fatalf("list (open) wrong:\n%s", openList)
	}
	all := run(t, env, out, "list", "--status", "all")
	if !strings.Contains(all, a) || !strings.Contains(all, b) {
		t.Fatalf("list (all) wrong:\n%s", all)
	}
}

func TestCLIUnknownCommand(t *testing.T) {
	env, _, _ := newTestEnv(t)
	runFail(t, env, "frobnicate")
}
