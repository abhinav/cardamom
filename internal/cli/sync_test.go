package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitInit initializes a git repo at dir with a usable identity, so the
// sync plumbing (commit-tree) has an author/committer. Returns dir.
func gitInit(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@clu.local")
	runGit(t, dir, "config", "user.name", "clu-test")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

// cluAt builds a testCLI rooted at a specific project dir (its store lives
// in <projectDir>/.clu). Used to drive two independent clones in one test.
func cluAt(t *testing.T, projectDir string) *testCLI {
	t.Helper()
	return &testCLI{t: t, dir: filepath.Join(projectDir, ".clu"), ctx: context.Background()}
}

// id is a tiny helper: the create command prints the new ID on stdout.
func id(s string) string { return strings.TrimSpace(s) }

func TestSyncRequiresGitRepo(t *testing.T) {
	// newTestCLI's project dir (parent of .clu) is a bare TempDir with no
	// git repo, so sync must refuse rather than wander up to some ancestor
	// repo and scribble a ref there.
	c := newTestCLI(t)
	c.run("init")
	c.run("create", "lonely task")
	c.runFail("sync", "push")
}

func TestSyncRoundTripAndBranchIndependence(t *testing.T) {
	c := newTestCLI(t)
	gitInit(t, filepath.Dir(c.dir))
	c.run("init")
	a := id(c.run("create", "-p", "1", "alpha"))
	b := id(c.run("create", "-p", "2", "beta"))
	c.run("dep", "add", b, a) // beta depends on alpha
	c.run("comment", "add", a, "a note", "-a", "tester")

	c.run("sync", "push")

	// The push must not dirty the working tree or index — it only writes
	// refs/clu/store via plumbing.
	if st := runGit(t, filepath.Dir(c.dir), "status", "--porcelain", "--", ".", ":!.clu"); strings.TrimSpace(st) != "" {
		t.Fatalf("sync push dirtied the work tree:\n%s", st)
	}

	// The ref exists and is visible after switching branches.
	runGit(t, filepath.Dir(c.dir), "checkout", "-q", "-b", "feature/x")
	if !strings.Contains(c.run("sync", "status"), "refs/clu/store") {
		t.Fatal("ref not visible from feature branch")
	}

	// Wipe local state, then pull it back out of the ref.
	c.run("sql", "--write", "DELETE FROM issues")
	if out := c.run("list", "--status", "all"); !strings.Contains(out, "(none)") {
		t.Fatalf("expected empty after wipe, got:\n%s", out)
	}
	c.run("sync", "pull")

	out := c.run("show", a)
	if !strings.Contains(out, "alpha") {
		t.Fatalf("alpha not restored:\n%s", out)
	}
	// Dependency edge restored: beta should still be blocked by alpha.
	if blk := c.run("show", b); !strings.Contains(blk, "Depends:") || !strings.Contains(blk, a) {
		t.Fatalf("dep edge not restored on pull:\n%s", blk)
	}
	// Comment restored.
	if !strings.Contains(out, "a note") {
		t.Fatalf("comment not restored:\n%s", out)
	}
}

func TestSyncLWWSkipsOlderSnapshot(t *testing.T) {
	c := newTestCLI(t)
	gitInit(t, filepath.Dir(c.dir))
	c.run("init")
	a := id(c.run("create", "original title"))
	c.run("sync", "push") // snapshot holds "original title"

	// Edit locally so the DB row is strictly newer than the snapshot.
	c.run("sql", "--write", "UPDATE issues SET title='edited title', updated=updated+100 WHERE id='"+a+"'")

	// Pulling the older snapshot must NOT clobber the newer local edit.
	out := c.run("sync", "pull")
	if !strings.Contains(out, "skipped") {
		t.Fatalf("expected a skipped-older report, got:\n%s", out)
	}
	if show := c.run("show", a); !strings.Contains(show, "edited title") {
		t.Fatalf("LWW failed: older snapshot overwrote newer local edit:\n%s", show)
	}
}

func TestSyncTombstoneDerivationInRef(t *testing.T) {
	c := newTestCLI(t)
	root := gitInit(t, filepath.Dir(c.dir))
	c.run("init")
	a := id(c.run("create", "keep me"))
	doomed := id(c.run("create", "delete me"))
	c.run("sync", "push") // both present, no tombstones yet

	// Hard-delete one issue, then push: the diff against the parent
	// snapshot should derive exactly one tombstone for it.
	c.run("sql", "--write", "DELETE FROM issues WHERE id='"+doomed+"'")
	out := c.run("sync", "push")
	if !strings.Contains(out, "1 tombstones") {
		t.Fatalf("expected 1 derived tombstone, got:\n%s", out)
	}
	// Inspect the ref's tombstones.jsonl directly.
	tombs := runGit(t, root, "cat-file", "-p", syncRef+":"+syncTombFile)
	if !strings.Contains(tombs, doomed) {
		t.Fatalf("tombstone file missing deleted id %s:\n%s", doomed, tombs)
	}
	if strings.Contains(tombs, a) {
		t.Fatalf("live issue %s should not be tombstoned:\n%s", a, tombs)
	}
}

// TestSyncRemotePropagation drives two independent clones through a bare
// remote: create + push from A, pull into B (forward apply + LWW), then
// delete in A and confirm the tombstone removes it from B on the next
// pull — all without a daemon.
func TestSyncRemotePropagation(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, remote, "init", "--bare", "-q")

	aDir := gitInit(t, filepath.Join(root, "A"))
	runGit(t, aDir, "remote", "add", "origin", remote)
	bDir := gitInit(t, filepath.Join(root, "B"))
	runGit(t, bDir, "remote", "add", "origin", remote)

	A := cluAt(t, aDir)
	B := cluAt(t, bDir)

	A.run("init")
	keep := id(A.run("create", "shared task"))
	doomed := id(A.run("create", "temporary task"))
	A.run("sync", "push", "--remote", "origin")

	// B starts empty, pulls from the remote, and sees both issues.
	B.run("init")
	B.run("sync", "pull", "--remote", "origin")
	if out := B.run("show", keep); !strings.Contains(out, "shared task") {
		t.Fatalf("B did not receive issue from A:\n%s", out)
	}
	if out := B.run("show", doomed); !strings.Contains(out, "temporary task") {
		t.Fatalf("B did not receive second issue:\n%s", out)
	}

	// Forward LWW: A edits, pushes; B pulls and converges to A's version.
	// Bump `updated` explicitly: the column is unix *seconds*, and the
	// whole test can run inside one second, which would tie A's edit with
	// B's local copy (ties go to local — see reconcile's known-gaps note).
	A.run("update", keep, "--title", "shared task v2")
	A.run("sql", "--write", "UPDATE issues SET updated=updated+100 WHERE id='"+keep+"'")
	A.run("sync", "push", "--remote", "origin")
	B.run("sync", "pull", "--remote", "origin")
	if out := B.run("show", keep); !strings.Contains(out, "shared task v2") {
		t.Fatalf("B did not converge to A's newer title:\n%s", out)
	}

	// Tombstone propagation: A deletes, pushes; B's pull removes it locally
	// while leaving the live issue intact.
	A.run("sql", "--write", "DELETE FROM issues WHERE id='"+doomed+"'")
	A.run("sync", "push", "--remote", "origin")
	out := B.run("sync", "pull", "--remote", "origin")
	if !strings.Contains(out, "1 deleted via tombstone") {
		t.Fatalf("expected tombstone delete on B, got:\n%s", out)
	}
	B.runFail("show", doomed) // gone
	if alive := B.run("show", keep); !strings.Contains(alive, "shared task v2") {
		t.Fatalf("tombstone collaterally removed the live issue:\n%s", alive)
	}
}

// clu-0880d5: `sync pull` must be able to bootstrap a missing local DB —
// both a fresh clone (never inited) and a clone whose data.sqlite was
// deleted. The ref is transport; the DB is rebuilt from it.
func TestSyncPullBootstrapsMissingDB(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, remote, "init", "--bare", "-q")

	aDir := gitInit(t, filepath.Join(root, "A"))
	runGit(t, aDir, "remote", "add", "origin", remote)
	A := cluAt(t, aDir)
	A.run("init")
	a := id(A.run("create", "shared task"))
	A.run("sync", "push", "--remote", "origin")

	// Fresh clone B: a git repo with the remote, but no `clu init` and so
	// no .clu/data.sqlite. Pull must create it.
	bDir := gitInit(t, filepath.Join(root, "B"))
	runGit(t, bDir, "remote", "add", "origin", remote)
	B := cluAt(t, bDir)
	if _, err := os.Stat(filepath.Join(bDir, ".clu", "data.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("expected no DB before pull, stat err = %v", err)
	}
	B.run("sync", "pull", "--remote", "origin")
	if out := B.run("show", a); !strings.Contains(out, "shared task") {
		t.Fatalf("fresh clone did not bootstrap from ref:\n%s", out)
	}

	// Local rebuild: delete A's DB, pull from the local ref, recover.
	if err := os.Remove(filepath.Join(aDir, ".clu", "data.sqlite")); err != nil {
		t.Fatal(err)
	}
	A.run("sync", "pull")
	if out := A.run("show", a); !strings.Contains(out, "shared task") {
		t.Fatalf("local pull did not rebuild deleted DB:\n%s", out)
	}
}

// clu-6eb080: first-ever `sync flush --remote` must tolerate a remote that
// doesn't have refs/clu/store yet (nothing to pull), then push.
func TestSyncFlushRemoteFirstUse(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, remote, "init", "--bare", "-q")

	aDir := gitInit(t, filepath.Join(root, "A"))
	runGit(t, aDir, "remote", "add", "origin", remote)
	A := cluAt(t, aDir)
	A.run("init")
	a := id(A.run("create", "shared task"))

	// No prior push — the remote ref is missing. flush must not treat the
	// failed fetch as fatal.
	A.run("sync", "flush", "--remote", "origin")

	// The ref now exists on the remote: a second clone can pull it.
	bDir := gitInit(t, filepath.Join(root, "B"))
	runGit(t, bDir, "remote", "add", "origin", remote)
	B := cluAt(t, bDir)
	B.run("sync", "pull", "--remote", "origin")
	if out := B.run("show", a); !strings.Contains(out, "shared task") {
		t.Fatalf("flush --remote did not publish the ref:\n%s", out)
	}
}

// clu-6c0c1c: `sync flush --json` must emit exactly one JSON value, with
// the pull and push results nested under one wrapper object.
func TestSyncFlushJSONSingleValue(t *testing.T) {
	c := newTestCLI(t)
	gitInit(t, filepath.Dir(c.dir))
	c.run("init")
	c.run("create", "first")
	c.run("sync", "push") // ref now exists, so flush's pull will also emit

	c.run("create", "second")
	out := c.run("--json", "sync", "flush")
	dec := json.NewDecoder(strings.NewReader(out))
	var wrapper struct {
		Pull map[string]any `json:"pull"`
		Push map[string]any `json:"push"`
	}
	if err := dec.Decode(&wrapper); err != nil {
		t.Fatalf("first JSON value did not decode: %v\nout: %s", err, out)
	}
	if dec.More() {
		t.Fatalf("flush --json emitted more than one JSON value:\n%s", out)
	}
	if wrapper.Pull == nil || wrapper.Push == nil {
		t.Fatalf("flush --json missing pull/push keys:\n%s", out)
	}
}

// clu-326774: a push whose serialized tree matches the ref's current tree
// must be a no-op — no new commit, ref unchanged.
func TestSyncPushNoOpKeepsRef(t *testing.T) {
	c := newTestCLI(t)
	root := gitInit(t, filepath.Dir(c.dir))
	c.run("init")
	c.run("create", "only task")
	c.run("sync", "push")

	before := strings.TrimSpace(runGit(t, root, "rev-parse", syncRef))
	out := c.run("--json", "sync", "push") // no data changed
	after := strings.TrimSpace(runGit(t, root, "rev-parse", syncRef))
	if before != after {
		t.Fatalf("no-op push advanced the ref: %s -> %s", before, after)
	}
	if !strings.Contains(out, `"noop":true`) {
		t.Fatalf("expected noop flag in push output:\n%s", out)
	}
}

// clu-118bac: `sync status` must report unsynced comments/KV (anything in
// the serialized tree), not just issue-row differences.
func TestSyncStatusFullTreeDirty(t *testing.T) {
	c := newTestCLI(t)
	gitInit(t, filepath.Dir(c.dir))
	c.run("init")
	a := id(c.run("create", "task one"))
	c.run("sync", "push")

	if st := c.run("--json", "sync", "status"); !strings.Contains(st, `"local_dirty":false`) {
		t.Fatalf("expected clean status right after push:\n%s", st)
	}

	// Add records that don't change any issue row.
	c.run("comment", "add", a, "unsynced note", "-a", "alice")
	c.run("kv", "set", "unsynced-key", "unsynced-value")

	st := c.run("--json", "sync", "status")
	if !strings.Contains(st, `"local_dirty":true`) {
		t.Fatalf("status ignored unsynced comment/KV:\n%s", st)
	}

	c.run("sync", "push")
	if st := c.run("--json", "sync", "status"); !strings.Contains(st, `"local_dirty":false`) {
		t.Fatalf("status still dirty after pushing the changes:\n%s", st)
	}
}
