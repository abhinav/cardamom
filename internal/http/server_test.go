package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rovak/clu/internal/store"
	"github.com/rovak/clu/internal/workflow"
)

var ctx = context.Background()

// newTestServer spins up an httptest.Server backed by a fresh on-disk
// store. The store directory is t.TempDir()-scoped, so each test gets a
// clean DB and everything is cleaned up at test end.
func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	ts := httptest.NewServer(New(s).Mux())
	t.Cleanup(ts.Close)
	return ts, s
}

// do is a thin fetch helper: builds the request, optionally sets the
// X-Clu-Agent header, optionally JSON-encodes the body, and returns
// the response + decoded body bytes.
func do(t *testing.T, ts *httptest.Server, method, path, agent string, body any) (*stdhttp.Response, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := stdhttp.NewRequest(method, ts.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if agent != "" {
		req.Header.Set(agentHeader, agent)
	}
	resp, err := stdhttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func mustJSON(t *testing.T, data []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal %s: %v", string(data), err)
	}
}

func TestHealthz(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, _ := do(t, ts, "GET", "/api/healthz", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestMeta(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, data := do(t, ts, "GET", "/api/meta", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, data)
	}
	var out metaOut
	mustJSON(t, data, &out)
	if len(out.Statuses) == 0 || len(out.Types) == 0 {
		t.Fatalf("empty meta: %+v", out)
	}
	if out.IDPrefix == "" {
		t.Fatalf("missing id_prefix")
	}
}

func TestCreateGetPatch(t *testing.T) {
	ts, _ := newTestServer(t)

	// Create
	resp, data := do(t, ts, "POST", "/api/issues", "", map[string]any{
		"title":    "first",
		"priority": 1,
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create status = %d, body = %s", resp.StatusCode, data)
	}
	var created issueOut
	mustJSON(t, data, &created)
	if created.ID == "" || created.Title != "first" {
		t.Fatalf("unexpected created: %+v", created)
	}

	// Get
	resp, data = do(t, ts, "GET", "/api/issues/"+created.ID, "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("get status = %d", resp.StatusCode)
	}
	var got issueDetailOut
	mustJSON(t, data, &got)
	if got.Title != "first" {
		t.Fatalf("got.Title = %q", got.Title)
	}

	// Patch — change title + clear assignee (it was unset; this exercises
	// the jsonOpt absent path) + set description + add a tag.
	resp, data = do(t, ts, "PATCH", "/api/issues/"+created.ID, "", map[string]any{
		"title":       "renamed",
		"description": "now with body",
		"tags":        []string{"backend", "urgent"},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("patch status = %d, body = %s", resp.StatusCode, data)
	}
	var patched issueOut
	mustJSON(t, data, &patched)
	if patched.Title != "renamed" {
		t.Fatalf("not renamed: %+v", patched)
	}
	if patched.Description == nil || *patched.Description != "now with body" {
		t.Fatalf("description not set: %+v", patched.Description)
	}
	if len(patched.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %v", patched.Labels)
	}
}

func TestPatchClearsDescription(t *testing.T) {
	ts, s := newTestServer(t)
	i, err := s.Create(ctx, "x", "task", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, i.ID, store.UpdateFields{
		Description: ptrPtr("temp"),
	}); err != nil {
		t.Fatal(err)
	}

	// Send {"description": null} — should clear.
	resp, data := do(t, ts, "PATCH", "/api/issues/"+i.ID, "", map[string]any{
		"description": nil,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("patch status = %d, body = %s", resp.StatusCode, data)
	}
	var out issueOut
	mustJSON(t, data, &out)
	if out.Description != nil {
		t.Fatalf("description not cleared: %v", *out.Description)
	}
}

func TestClaimRequiresAgent(t *testing.T) {
	ts, s := newTestServer(t)
	i, _ := s.Create(ctx, "x", "task", 1, nil)

	resp, _ := do(t, ts, "POST", "/api/issues/"+i.ID+"/claim", "", nil)
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	resp, data := do(t, ts, "POST", "/api/issues/"+i.ID+"/claim", "alice", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("claim status = %d, body = %s", resp.StatusCode, data)
	}
	var out issueOut
	mustJSON(t, data, &out)
	if out.Assignee == nil || *out.Assignee != "alice" {
		t.Fatalf("assignee not set: %+v", out.Assignee)
	}
	if out.Status != "in_progress" {
		t.Fatalf("status = %q", out.Status)
	}
}

func TestCloseReopen(t *testing.T) {
	ts, s := newTestServer(t)
	i, _ := s.Create(ctx, "x", "task", 1, nil)

	resp, _ := do(t, ts, "POST", "/api/issues/"+i.ID+"/close", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("close status = %d", resp.StatusCode)
	}
	resp, _ = do(t, ts, "POST", "/api/issues/"+i.ID+"/reopen", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("reopen status = %d", resp.StatusCode)
	}
}

func TestListWithFilter(t *testing.T) {
	ts, s := newTestServer(t)
	a, _ := s.Create(ctx, "alpha", "task", 1, nil)
	_, _ = s.Create(ctx, "beta", "bug", 2, nil)
	_, _ = s.AddLabels(ctx, a.ID, []string{"frontend"})

	resp, data := do(t, ts, "GET", "/api/issues?type=task", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("list status = %d", resp.StatusCode)
	}
	var out []issueOut
	mustJSON(t, data, &out)
	if len(out) != 1 || out[0].Type != "task" {
		t.Fatalf("unexpected: %+v", out)
	}

	// Tag filter (label_any).
	_, data = do(t, ts, "GET", "/api/issues?tag=frontend", "", nil)
	mustJSON(t, data, &out)
	if len(out) != 1 || out[0].ID != a.ID {
		t.Fatalf("tag filter wrong: %+v", out)
	}
}

func TestCommentsRoundtrip(t *testing.T) {
	ts, s := newTestServer(t)
	i, _ := s.Create(ctx, "x", "task", 1, nil)

	resp, data := do(t, ts, "POST", "/api/issues/"+i.ID+"/comments", "alice", map[string]any{
		"body": "looks good",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("post comment status = %d, body = %s", resp.StatusCode, data)
	}
	resp, data = do(t, ts, "GET", "/api/issues/"+i.ID+"/comments", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("list comments status = %d", resp.StatusCode)
	}
	var cs []store.Comment
	mustJSON(t, data, &cs)
	if len(cs) != 1 || cs[0].Author != "alice" || cs[0].Body != "looks good" {
		t.Fatalf("unexpected comments: %+v", cs)
	}
}

func TestDepsRoundtrip(t *testing.T) {
	ts, s := newTestServer(t)
	a, _ := s.Create(ctx, "a", "task", 1, nil)
	b, _ := s.Create(ctx, "b", "task", 1, nil)

	// b depends on a
	resp, data := do(t, ts, "POST", "/api/issues/"+b.ID+"/deps", "", map[string]any{
		"parent": a.ID,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("add dep status = %d, body = %s", resp.StatusCode, data)
	}

	// Cycle: a → b should now be rejected.
	resp, data = do(t, ts, "POST", "/api/issues/"+a.ID+"/deps", "", map[string]any{
		"parent": b.ID,
	})
	if resp.StatusCode != 400 {
		t.Fatalf("cycle should 400, got %d (%s)", resp.StatusCode, data)
	}
}

func TestCheckpointApprove(t *testing.T) {
	ts, s := newTestServer(t)
	// Create a checkpoint issue and prime the KV payload by hand.
	cp, err := s.Create(ctx, "review gate", "checkpoint", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.AddLabels(ctx, cp.ID, []string{"checkpoint:pending"})
	_ = s.KVSet(ctx, "cp:"+cp.ID, `{"kind":"approval","approvers":["alice"]}`)

	// Approver-list enforcement was dropped (single-user local tool;
	// the list is informational, not a gate). Any agent on the
	// X-Clu-Agent header can approve, regardless of membership.
	// Identity itself is still required — confirm the missing-header
	// case still 400s.
	resp, _ := do(t, ts, "POST", "/api/checkpoints/"+cp.ID+"/approve", "", map[string]any{})
	if resp.StatusCode != 400 {
		t.Fatalf("missing X-Clu-Agent should be 400, got %d", resp.StatusCode)
	}

	// Off-list approver → 200. This is the case that used to 400.
	resp, data := do(t, ts, "POST", "/api/checkpoints/"+cp.ID+"/approve", "bob", map[string]any{
		"reason": "lgtm",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("off-list approver should now pass; status = %d, body = %s", resp.StatusCode, data)
	}
	var out store.CheckpointResult
	mustJSON(t, data, &out)
	if !out.Pass || out.Closed.Status != "closed" {
		t.Fatalf("checkpoint not passed/closed: %+v", out)
	}
	hasPassedLabel := false
	for _, l := range out.Labels {
		if l == "checkpoint:passed" {
			hasPassedLabel = true
		}
	}
	if !hasPassedLabel {
		t.Fatalf("missing checkpoint:passed label: %v", out.Labels)
	}
}

// TestListCheckpoints exercises /api/checkpoints — the feed for the
// /approvals UI. Tests: a pending checkpoint with approvers shows
// up, a non-pending checkpoint (no checkpoint:pending label) does
// not, and the response is an empty array (not null) when none.
func TestListCheckpoints(t *testing.T) {
	ts, s := newTestServer(t)

	// Empty case first.
	resp, data := do(t, ts, "GET", "/api/checkpoints", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if string(data) != "[]\n" {
		t.Fatalf("empty checkpoints body = %q (want %q)", string(data), "[]\n")
	}

	// Pending checkpoint with approvers.
	pending, _ := s.Create(ctx, "Review release", "checkpoint", 1, nil)
	_, _ = s.AddLabels(ctx, pending.ID, []string{"checkpoint:pending"})
	_ = s.KVSet(ctx, "cp:"+pending.ID,
		`{"kind":"approval","approvers":["alice","bob"]}`)

	// A second issue that the checkpoint blocks (so .blocks is non-empty).
	blocked, _ := s.Create(ctx, "Roll out the change", "task", 1, nil)
	if err := s.AddDep(ctx, blocked.ID, pending.ID); err != nil {
		t.Fatal(err)
	}

	// A non-pending checkpoint (already approved) — should be excluded.
	approved, _ := s.Create(ctx, "Old gate", "checkpoint", 1, nil)
	_, _ = s.AddLabels(ctx, approved.ID, []string{"checkpoint:passed"})
	_ = s.KVSet(ctx, "cp:"+approved.ID, `{"kind":"manual"}`)

	resp, data = do(t, ts, "GET", "/api/checkpoints", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, data)
	}
	var out []pendingCheckpointOut
	mustJSON(t, data, &out)
	if len(out) != 1 {
		t.Fatalf("expected 1 pending checkpoint, got %d (%+v)", len(out), out)
	}
	got := out[0]
	if got.ID != pending.ID {
		t.Fatalf("wrong issue: %s", got.ID)
	}
	if got.Kind != "approval" {
		t.Fatalf("wrong kind: %q", got.Kind)
	}
	if len(got.Approvers) != 2 || got.Approvers[0] != "alice" {
		t.Fatalf("approvers wrong: %v", got.Approvers)
	}
	if len(got.Blocks) != 1 || got.Blocks[0] != blocked.ID {
		t.Fatalf("blocks wrong: %v", got.Blocks)
	}
}

// TestTemplatesRoundtrip exercises the workflow templates endpoints:
// list returns the seeded template, plan dry-runs without writing,
// run instantiates and we can find the parent + steps in the store.
func TestTemplatesRoundtrip(t *testing.T) {
	dir := t.TempDir()
	tdir := workflow.TemplatesPath(dir)
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		t.Fatal(err)
	}
	yml := `name: spike
description: Three-step spike workflow
vars:
  scope:
    required: true
    label: "Short scope tag"
steps:
  - id: investigate
    title: "Investigate: {{scope}}"
  - id: implement
    title: "Implement: {{scope}}"
    needs: [investigate]
  - id: review
    title: "Review: {{scope}}"
    type: checkpoint
    needs: [implement]
    wait:
      approval: [alice]
`
	if err := os.WriteFile(filepath.Join(tdir, "spike.yaml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	srv := New(s).WithTemplatesDir(tdir)
	ts := httptest.NewServer(srv.Mux())
	t.Cleanup(ts.Close)

	// List
	resp, data := do(t, ts, "GET", "/api/templates", "", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("list status=%d body=%s", resp.StatusCode, data)
	}
	var list templatesListOut
	mustJSON(t, data, &list)
	if len(list.Templates) != 1 || list.Templates[0].Name != "spike" {
		t.Fatalf("unexpected list: %+v", list)
	}
	if list.Templates[0].StepCount != 3 {
		t.Fatalf("step count = %d", list.Templates[0].StepCount)
	}
	if len(list.Templates[0].Vars) != 1 || list.Templates[0].Vars[0].Name != "scope" {
		t.Fatalf("vars wrong: %+v", list.Templates[0].Vars)
	}

	// Plan with missing required var → 400
	resp, _ = do(t, ts, "POST", "/api/templates/spike/plan", "", map[string]any{})
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for missing var, got %d", resp.StatusCode)
	}

	// Plan with var → 200, no DB writes
	resp, data = do(t, ts, "POST", "/api/templates/spike/plan", "", map[string]any{
		"vars": map[string]string{"scope": "auth-fix"},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("plan status=%d body=%s", resp.StatusCode, data)
	}
	var plan planOut
	mustJSON(t, data, &plan)
	if len(plan.Steps) != 3 {
		t.Fatalf("plan steps = %d", len(plan.Steps))
	}
	if plan.Title != "spike auth-fix" {
		t.Fatalf("plan title = %q", plan.Title)
	}
	// Plan didn't write anything.
	issues, _ := s.List(ctx, store.ListFilter{})
	if len(issues) != 0 {
		t.Fatalf("plan wrote issues: %d", len(issues))
	}

	// Run actually instantiates.
	resp, data = do(t, ts, "POST", "/api/templates/spike/run", "", map[string]any{
		"vars": map[string]string{"scope": "auth-fix"},
	})
	if resp.StatusCode != 201 {
		t.Fatalf("run status=%d body=%s", resp.StatusCode, data)
	}
	var runOut struct {
		ParentID string `json:"parent_id"`
	}
	mustJSON(t, data, &runOut)
	if runOut.ParentID == "" {
		t.Fatalf("missing parent_id in %s", data)
	}
	issues, _ = s.List(ctx, store.ListFilter{})
	if len(issues) != 4 { // parent + 3 steps
		t.Fatalf("after run, issues = %d (want 4)", len(issues))
	}
	// Checkpoint step should have a cp:<id> KV row with approvers.
	var checkpointID string
	for _, i := range issues {
		if i.Type == "checkpoint" {
			checkpointID = i.ID
		}
	}
	if checkpointID == "" {
		t.Fatalf("no checkpoint step created")
	}
	if _, err := s.GetCheckpointPayload(ctx, checkpointID); err != nil {
		t.Fatalf("checkpoint payload missing: %v", err)
	}
}

func TestTemplatesNotConfigured(t *testing.T) {
	// Server with no templates dir set → 503 on the templates endpoints.
	ts, _ := newTestServer(t)
	resp, _ := do(t, ts, "GET", "/api/templates", "", nil)
	if resp.StatusCode != 503 {
		t.Fatalf("expected 503 when templates dir unset, got %d", resp.StatusCode)
	}
}

func TestNotFoundIsJSON(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, data := do(t, ts, "GET", "/api/issues/nope", "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body errorBody
	mustJSON(t, data, &body)
	if !strings.Contains(body.Error, "not found") {
		t.Fatalf("error body = %q", body.Error)
	}
}

func ptrPtr[T any](v T) **T {
	p := &v
	return &p
}

// TestBrokerFanout exercises the SSE broker without going through an
// HTTP roundtrip: subscribe twice, tick the watermark, both
// subscribers should receive an issues-changed event.
func TestBrokerFanout(t *testing.T) {
	b := newBroker()
	b.pollInterval = 5 * time.Millisecond
	var watermark int64
	lookup := func(_ context.Context) (int64, error) {
		return atomic.LoadInt64(&watermark), nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	b.start(ctx, lookup)
	// Let start() seed lastSeen against the zero watermark.
	time.Sleep(20 * time.Millisecond)

	s1, unsub1 := b.subscribe()
	defer unsub1()
	s2, unsub2 := b.subscribe()
	defer unsub2()

	atomic.StoreInt64(&watermark, 42)

	for _, ch := range []chan event{s1, s2} {
		select {
		case e := <-ch:
			if e.Type != "issues-changed" {
				t.Fatalf("got event type %q, want issues-changed", e.Type)
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("subscriber did not receive event")
		}
	}
}

// TestEventsEndpoint exercises the SSE HTTP handler: subscribe via a
// real request, mutate the store, verify the byte stream carries an
// issues-changed event. Drives the broker by speeding up its poll
// interval.
func TestEventsEndpoint(t *testing.T) {
	ts, s := newTestServer(t)
	// We need to dig into the server's broker to bump poll speed —
	// the default 1s would make this test too slow.
	// (Test-only access: package-private fields are visible here.)
	// Re-create the server with a fast broker.
	// Reuse the store from newTestServer but ignore its TS.
	ts.Close()

	srv := New(s)
	srv.broker.pollInterval = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	srv.Start(ctx)
	ts2 := httptest.NewServer(srv.Mux())
	defer ts2.Close()

	req, _ := stdhttp.NewRequest("GET", ts2.URL+"/api/events", nil)
	req.Header.Set("Accept", "text/event-stream")
	cctx, ccancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer ccancel()
	req = req.WithContext(cctx)

	resp, err := stdhttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("wrong content-type: %q", resp.Header.Get("Content-Type"))
	}

	// Cause a state change. The poll loop should pick it up within
	// ~10ms and push an issues-changed frame.
	if _, err := s.Create(ctx, "watch me", "task", 1, nil); err != nil {
		t.Fatal(err)
	}

	// Read frames until we see issues-changed or hit the deadline.
	buf := make([]byte, 1024)
	deadline := time.Now().Add(2 * time.Second)
	var seen string
	for time.Now().Before(deadline) {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			seen += string(buf[:n])
			if strings.Contains(seen, "event: issues-changed") {
				return
			}
		}
		if err != nil {
			break
		}
	}
	t.Fatalf("did not see issues-changed; saw: %q", seen)
}
