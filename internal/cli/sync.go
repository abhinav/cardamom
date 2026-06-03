package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/arjia-labs/clu/internal/store"
	"github.com/uptrace/bun"
)

// --- git-ref sync (prototype) ------------------------------------------
//
// Stores the project's issue state on a dedicated git ref —
// refs/clu/store — as JSONL, the way beads-rs keeps everything on
// refs/heads/beads/store. The point: issue state is *branch-independent*
// (any checkout sees the same issues) and never collides with code
// commits, because writes only ever touch this ref, never your branch or
// the working tree.
//
// SQLite stays the working copy / query engine; the ref is the durable,
// shareable log. `clu sync push` serializes the DB onto the ref via pure
// git plumbing (hash-object → mktree → commit-tree → update-ref — no
// checkout). `clu sync pull` reads it back and reconciles into SQLite
// with last-writer-wins by `updated`, plus tombstones so deletes
// propagate. With --remote, push/pull also push/fetch the ref.
//
// This is a spike. Known simplifications are flagged inline and in the
// command help; see the parent tracking issue for the full list.

const (
	syncRef       = "refs/clu/store"
	syncStateFile = "state.jsonl"
	syncTombFile  = "tombstones.jsonl"
)

type SyncCmd struct {
	Push   SyncPushCmd   `cmd:"" help:"Serialize the DB onto refs/clu/store (and optionally push it)."`
	Pull   SyncPullCmd   `cmd:"" help:"Reconcile refs/clu/store back into the DB (LWW + tombstones)."`
	Status SyncStatusCmd `cmd:"" help:"Show the synced ref and how it compares to the local DB."`
	Flush  SyncFlushCmd  `cmd:"" help:"pull then push — the full round-trip (beads-rs 'sync')."`
}

// tombstone is a deletion marker. beads-rs keeps an explicit tombstone
// stream merged as a CRDT (keep later deletion stamp); we derive ours by
// diffing the previous snapshot's issue IDs against the current DB on each
// push, and carry them forward. Resurrection wins: if an issue with the
// same ID reappears (present in the DB, or with updated > Deleted), its
// tombstone is dropped on push / ignored on pull.
type tombstone struct {
	ID      string `json:"id"`
	Deleted int64  `json:"deleted"`
	Actor   string `json:"actor,omitempty"`
}

// snapshot is a parsed view of one ref commit's two files.
type snapshot struct {
	issues   map[string]issueExport // by ID, for LWW
	deps     []depExport
	comments []store.Comment
	kvs      []store.KV
	crons    []store.CronJob
	tombs    map[string]tombstone // by ID
}

func newSnapshot() *snapshot {
	return &snapshot{issues: map[string]issueExport{}, tombs: map[string]tombstone{}}
}

// ---- push -------------------------------------------------------------

type SyncPushCmd struct {
	Remote  string `name:"remote" help:"Also push refs/clu/store to this git remote (e.g. origin)."`
	Message string `name:"message" short:"m" help:"Commit subject for the ref commit." default:"clu sync"`
}

func (c *SyncPushCmd) Run(r *runCtx) error {
	if err := requireGitRepo(r); err != nil {
		return err
	}
	return withStore(r, func(s *store.Store) error {
		// Previous snapshot (the commit we're extending) — gives us the
		// prior issue-ID set to diff for tombstones, and the prior
		// tombstones to carry forward.
		parent, _ := gitResolve(r, syncRef)
		prev := newSnapshot()
		if parent != "" {
			var err error
			prev, err = readSnapshot(r, parent)
			if err != nil {
				return fmt.Errorf("read parent ref: %w", err)
			}
		}

		// Serialize current DB state.
		var state bytes.Buffer
		counts, err := writeExportJSONL(r.ctx, s, &state)
		if err != nil {
			return err
		}

		// Which issue IDs exist now?
		live := map[string]bool{}
		for _, line := range splitLines(state.Bytes()) {
			var hdr exportLine
			if json.Unmarshal(line, &hdr) != nil || hdr.Kind != "issue" {
				continue
			}
			var ie issueExport
			if json.Unmarshal(hdr.Data, &ie) == nil {
				live[ie.ID] = true
			}
		}

		// Tombstones: carry forward prior ones (minus any resurrected),
		// then add a fresh tombstone for every ID that was in the parent
		// snapshot but is gone from the DB now.
		tombs := map[string]tombstone{}
		for id, t := range prev.tombs {
			if !live[id] {
				tombs[id] = t
			}
		}
		for id := range prev.issues {
			if !live[id] {
				tombs[id] = tombstone{ID: id, Deleted: time.Now().Unix(), Actor: r.actor}
			}
		}

		var tombBuf bytes.Buffer
		writeTombstones(&tombBuf, tombs)

		// Plumbing: blobs → tree → commit → ref. Never touches the index
		// or working tree.
		stateBlob, err := gitHashObject(r, state.Bytes())
		if err != nil {
			return err
		}
		tombBlob, err := gitHashObject(r, tombBuf.Bytes())
		if err != nil {
			return err
		}
		tree, err := gitMkTree(r, []treeEntry{
			{name: syncStateFile, sha: stateBlob},
			{name: syncTombFile, sha: tombBlob},
		})
		if err != nil {
			return err
		}
		commit, err := gitCommitTree(r, tree, parent, c.Message)
		if err != nil {
			return err
		}
		if err := gitUpdateRef(r, syncRef, commit, parent); err != nil {
			return err
		}

		if c.Remote != "" {
			if err := gitRun(r, nil, "push", c.Remote, syncRef+":"+syncRef); err != nil {
				return fmt.Errorf("push to %s: %w", c.Remote, err)
			}
		}

		if r.json {
			return r.emitJSON(map[string]any{
				"ref": syncRef, "commit": commit, "tombstones": len(tombs),
				"issues": counts.Issues, "deps": counts.Deps,
				"comments": counts.Comments, "kv": counts.KV, "cron": counts.Cron,
			})
		}
		r.notice("pushed %s @ %s — %d issues, %d deps, %d tombstones\n",
			syncRef, short(commit), counts.Issues, counts.Deps, len(tombs))
		return nil
	})
}

// ---- pull -------------------------------------------------------------

type SyncPullCmd struct {
	Remote string `name:"remote" help:"Fetch refs/clu/store from this remote first (force; the DB is the real local state)."`
}

func (c *SyncPullCmd) Run(r *runCtx) error {
	if err := requireGitRepo(r); err != nil {
		return err
	}
	return withStore(r, func(s *store.Store) error {
		if c.Remote != "" {
			// Force-fetch is safe: the ref is only transport. Anything
			// local that isn't on the remote yet still lives in the DB
			// and gets re-derived on the next push.
			if err := gitRun(r, nil, "fetch", "--force", c.Remote, syncRef+":"+syncRef); err != nil {
				return fmt.Errorf("fetch from %s: %w", c.Remote, err)
			}
		}
		ref, ok := gitResolve(r, syncRef)
		if !ok || ref == "" {
			return fmt.Errorf("%s does not exist — run `clu sync push` first (or pull --remote)", syncRef)
		}
		snap, err := readSnapshot(r, ref)
		if err != nil {
			return err
		}
		res, err := reconcile(r.ctx, s, snap)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(res)
		}
		r.notice("pulled %s @ %s — %d applied, %d skipped (older), %d deleted via tombstone\n",
			syncRef, short(ref), res.Applied, res.SkippedOlder, res.Deleted)
		return nil
	})
}

// ---- status -----------------------------------------------------------

type SyncStatusCmd struct{}

func (c *SyncStatusCmd) Run(r *runCtx) error {
	if err := requireGitRepo(r); err != nil {
		return err
	}
	return withStore(r, func(s *store.Store) error {
		local, err := s.List(r.ctx, store.ListFilter{})
		if err != nil {
			return err
		}
		ref, ok := gitResolve(r, syncRef)
		out := map[string]any{"ref": syncRef, "exists": ok, "local_issues": len(local)}
		if ok {
			snap, err := readSnapshot(r, ref)
			if err != nil {
				return err
			}
			// Count how many local issues differ from / are missing on the ref.
			ahead := 0
			localIDs := map[string]bool{}
			for _, li := range local {
				localIDs[li.ID] = true
				ie, present := snap.issues[li.ID]
				if !present || li.Updated > ie.Updated {
					ahead++
				}
			}
			behind := 0
			for id, ie := range snap.issues {
				li, present := findIssue(local, id)
				if !present || ie.Updated > li.Updated {
					behind++
				}
			}
			out["commit"] = ref
			out["ref_issues"] = len(snap.issues)
			out["tombstones"] = len(snap.tombs)
			out["local_ahead"] = ahead // local newer/extra vs ref
			out["ref_ahead"] = behind  // ref newer/extra vs local
		}
		if r.json {
			return r.emitJSON(out)
		}
		if !ok {
			r.notice("%s: not yet created (%d local issues). Run `clu sync push`.\n", syncRef, len(local))
			return nil
		}
		r.notice("%s @ %s\n  ref:   %d issues, %d tombstones\n  local: %d issues\n  diff:  %d local-ahead, %d ref-ahead\n",
			syncRef, short(ref), out["ref_issues"], out["tombstones"], len(local), out["local_ahead"], out["ref_ahead"])
		return nil
	})
}

// ---- flush ------------------------------------------------------------

type SyncFlushCmd struct {
	Remote string `name:"remote" help:"Fetch before / push after against this remote."`
}

func (c *SyncFlushCmd) Run(r *runCtx) error {
	if err := (&SyncPullCmd{Remote: c.Remote}).Run(r); err != nil {
		// A missing ref on first-ever flush is fine — nothing to pull.
		if !strings.Contains(err.Error(), "does not exist") {
			return err
		}
	}
	return (&SyncPushCmd{Remote: c.Remote, Message: "clu sync"}).Run(r)
}

// ---- reconciliation ---------------------------------------------------

type reconcileResult struct {
	Applied      int `json:"applied"`       // issues inserted or updated
	SkippedOlder int `json:"skipped_older"` // incoming was older than local
	Deleted      int `json:"deleted"`       // local issues removed by tombstone
	Deps         int `json:"deps"`
	Comments     int `json:"comments"`
	KV           int `json:"kv"`
	Cron         int `json:"cron"`
}

// reconcile merges a ref snapshot into the DB. Issues use last-writer-wins
// on `updated`; everything else is additively upserted. Tombstones delete
// local issues unless the local copy is newer (resurrection wins). Runs in
// a single transaction so a failure leaves the DB untouched.
//
// Prototype gaps (deliberate): dep/comment *removals* aren't reconciled
// (upserts are additive), and KV has no timestamp so it's last-sync-wins.
// LWW resolves on `updated`, which is unix *seconds* — two edits to the
// same issue within the same second tie, and ties go to the local copy
// (>= below). That's non-commutative across clones; beads-rs avoids it
// with a finer Stamp plus an actor tiebreak. A real implementation should
// widen the timestamp and break ties deterministically (e.g. by actor).
func reconcile(ctx context.Context, s *store.Store, snap *snapshot) (reconcileResult, error) {
	var res reconcileResult
	err := s.RunInTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		// Live issues first, so a resurrected row exists before we weigh
		// its tombstone.
		for id, ie := range snap.issues {
			local, err := getIssueTx(ctx, tx, id)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			if err == nil && local.Updated >= ie.Updated {
				res.SkippedOlder++
				continue
			}
			if err := store.UpsertIssueTx(ctx, tx, ie.Issue); err != nil {
				return fmt.Errorf("upsert %s: %w", id, err)
			}
			if err := store.ReplaceLabelsTx(ctx, tx, id, ie.Labels); err != nil {
				return fmt.Errorf("labels %s: %w", id, err)
			}
			res.Applied++
		}
		for _, d := range snap.deps {
			if err := store.UpsertDepTx(ctx, tx, d.Child, d.Parent); err != nil {
				return err
			}
			res.Deps++
		}
		for _, cm := range snap.comments {
			if err := store.UpsertCommentTx(ctx, tx, cm); err != nil {
				return err
			}
			res.Comments++
		}
		for _, kv := range snap.kvs {
			if err := store.KVSetTx(ctx, tx, kv.Key, kv.Value); err != nil {
				return err
			}
			res.KV++
		}
		for _, cj := range snap.crons {
			if err := store.CronJobUpsertTx(ctx, tx, cj); err != nil {
				return err
			}
			res.Cron++
		}
		// Tombstones last.
		for id, t := range snap.tombs {
			local, err := getIssueTx(ctx, tx, id)
			if errors.Is(err, store.ErrNotFound) {
				continue // already gone
			}
			if err != nil {
				return err
			}
			if local.Updated > t.Deleted {
				continue // resurrection wins
			}
			if err := store.DeleteIssueTx(ctx, tx, id); err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			res.Deleted++
		}
		return nil
	})
	return res, err
}

// getIssueTx fetches one issue inside a transaction.
func getIssueTx(ctx context.Context, tx bun.Tx, id string) (store.Issue, error) {
	var i store.Issue
	err := tx.NewSelect().Model(&i).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return store.Issue{}, store.ErrNotFound
		}
		return store.Issue{}, err
	}
	return i, nil
}

// ---- snapshot parsing -------------------------------------------------

func readSnapshot(r *runCtx, commit string) (*snapshot, error) {
	snap := newSnapshot()
	state, err := gitCatFile(r, commit+":"+syncStateFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", syncStateFile, err)
	}
	for _, line := range splitLines(state) {
		var hdr exportLine
		if err := json.Unmarshal(line, &hdr); err != nil {
			continue
		}
		switch hdr.Kind {
		case "issue":
			var ie issueExport
			if json.Unmarshal(hdr.Data, &ie) == nil {
				snap.issues[ie.ID] = ie
			}
		case "dep":
			var d depExport
			if json.Unmarshal(hdr.Data, &d) == nil {
				snap.deps = append(snap.deps, d)
			}
		case "comment":
			var cm store.Comment
			if json.Unmarshal(hdr.Data, &cm) == nil {
				snap.comments = append(snap.comments, cm)
			}
		case "kv":
			var kv store.KV
			if json.Unmarshal(hdr.Data, &kv) == nil {
				snap.kvs = append(snap.kvs, kv)
			}
		case "cron":
			var cj store.CronJob
			if json.Unmarshal(hdr.Data, &cj) == nil {
				snap.crons = append(snap.crons, cj)
			}
		}
	}
	// Tombstones file is optional (absent on pre-tombstone commits).
	if tombs, err := gitCatFile(r, commit+":"+syncTombFile); err == nil {
		for _, line := range splitLines(tombs) {
			var t tombstone
			if json.Unmarshal(line, &t) == nil && t.ID != "" {
				snap.tombs[t.ID] = t
			}
		}
	}
	return snap, nil
}

func writeTombstones(buf *bytes.Buffer, tombs map[string]tombstone) {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	// Stable order keeps the blob deterministic across runs with the same set.
	for _, id := range sortedKeys(tombs) {
		_ = enc.Encode(tombs[id])
	}
}

// ---- git plumbing -----------------------------------------------------

type treeEntry struct {
	name string
	sha  string
}

func requireGitRepo(r *runCtx) error {
	if err := gitRun(r, nil, "rev-parse", "--git-dir"); err != nil {
		return errors.New("clu sync requires a git repository (run from inside one)")
	}
	return nil
}

// gitRun runs git for side effects, discarding stdout. stdin is optional.
func gitRun(r *runCtx, stdin []byte, args ...string) error {
	_, err := gitOut(r, stdin, args...)
	return err
}

// gitOut runs git and returns trimmed stdout. Author/committer identity is
// pinned to the clu actor so ref commits are attributable to the agent
// that made them, regardless of the repo's git config.
func gitOut(r *runCtx, stdin []byte, args ...string) (string, error) {
	cmd := exec.CommandContext(r.ctx, "git", args...)
	// Anchor git to the project dir that owns the .clu/ store, not the
	// process cwd. With the default --dir=.clu this is "." (unchanged);
	// with an explicit/absolute --dir it pins the ref to that repo (and
	// keeps tests from writing into whatever repo the test binary runs
	// in). git itself ascends from here to find the .git dir.
	cmd.Dir = filepath.Dir(r.dir)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	actor := r.actor
	if actor == "" {
		actor = "clu"
	}
	cmd.Env = append(cmd.Environ(),
		"GIT_AUTHOR_NAME="+actor, "GIT_AUTHOR_EMAIL="+actor+"@clu.local",
		"GIT_COMMITTER_NAME="+actor, "GIT_COMMITTER_EMAIL="+actor+"@clu.local",
	)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg != "" {
			return "", fmt.Errorf("git %s: %s", args[0], msg)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimSpace(out.String()), nil
}

// gitResolve returns the SHA a ref points at, and whether it exists.
func gitResolve(r *runCtx, ref string) (string, bool) {
	sha, err := gitOut(r, nil, "rev-parse", "--verify", "--quiet", ref)
	if err != nil || sha == "" {
		return "", false
	}
	return sha, true
}

func gitCatFile(r *runCtx, spec string) ([]byte, error) {
	out, err := gitOut(r, nil, "cat-file", "-p", spec)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

func gitHashObject(r *runCtx, content []byte) (string, error) {
	return gitOut(r, content, "hash-object", "-w", "--stdin")
}

func gitMkTree(r *runCtx, entries []treeEntry) (string, error) {
	var in bytes.Buffer
	for _, e := range entries {
		fmt.Fprintf(&in, "100644 blob %s\t%s\n", e.sha, e.name)
	}
	return gitOut(r, in.Bytes(), "mktree")
}

func gitCommitTree(r *runCtx, tree, parent, msg string) (string, error) {
	args := []string{"commit-tree", tree, "-m", msg}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	return gitOut(r, nil, args...)
}

func gitUpdateRef(r *runCtx, ref, newSHA, oldSHA string) error {
	if oldSHA == "" {
		return gitRun(r, nil, "update-ref", ref, newSHA)
	}
	// CAS: fail if someone moved the ref under us since we read it.
	return gitRun(r, nil, "update-ref", ref, newSHA, oldSHA)
}

// ---- small helpers ----------------------------------------------------

func splitLines(b []byte) [][]byte {
	var out [][]byte
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		cp := make([]byte, len(line))
		copy(cp, line)
		out = append(out, cp)
	}
	return out
}

func sortedKeys(m map[string]tombstone) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort — tombstone sets are tiny.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func findIssue(issues []store.Issue, id string) (store.Issue, bool) {
	for _, i := range issues {
		if i.ID == id {
			return i, true
		}
	}
	return store.Issue{}, false
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
