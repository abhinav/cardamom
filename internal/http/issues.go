package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"strconv"
	"strings"

	"github.com/Rovak/agents-clu/internal/store"
)

// issueOut wraps store.Issue with derived fields (labels, blocked) the
// UI cares about. Mirrors internal/cli's issueOut so JSON shapes are
// identical between `clu list --json` and `GET /api/issues`.
type issueOut struct {
	store.Issue
	Labels  []string `json:"labels"`
	Blocked bool     `json:"blocked"`
}

// issueDetailOut adds deps + comments for the single-issue endpoint.
type issueDetailOut struct {
	store.Issue
	Labels   []string        `json:"labels"`
	Depends  []string        `json:"depends_on"`
	Blocks   []string        `json:"blocks"`
	Comments []store.Comment `json:"comments"`
	Blocked  bool            `json:"blocked"`
}

// handleListIssues — GET /api/issues
//
// Query params (all optional, all repeatable for the multi-value ones):
//
//	status         one or more (?status=open&status=in_progress)
//	priority_min   int
//	priority_max   int
//	type           exact match
//	agent          exact match on assignee (mirrors CLI -a)
//	label          AND of multiple — issue must have all
//	label_any      OR of multiple
//	tag            UI-friendly alias for label_any
//	no_labels      "1" / "true"
//	no_assignee    "1" / "true"
//	q              title substring
//	desc           description substring
//	limit          int (default 0 = no limit)
//	sort           one of validSortKeys
//	reverse        "1" / "true"
func (s *Server) handleListIssues(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	f, err := parseListFilter(r)
	if err != nil {
		writeError(w, stdhttp.StatusBadRequest, err.Error())
		return
	}
	issues, err := s.store.List(ctxOf(r), f)
	if err != nil {
		respondErr(w, err)
		return
	}
	ids := issueIDs(issues)
	labels, err := s.store.LoadLabels(ctxOf(r), ids)
	if err != nil {
		respondErr(w, err)
		return
	}
	blocked, err := s.store.IDsBlocked(ctxOf(r), ids)
	if err != nil {
		respondErr(w, err)
		return
	}
	out := make([]issueOut, len(issues))
	for i, is := range issues {
		out[i] = issueOut{Issue: is, Labels: nilToEmpty(labels[is.ID]), Blocked: blocked[is.ID]}
	}
	writeJSON(w, stdhttp.StatusOK, out)
}

// handleGetIssue — GET /api/issues/{id}
func (s *Server) handleGetIssue(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	issue, err := s.store.Get(ctxOf(r), id)
	if err != nil {
		respondErr(w, err)
		return
	}
	parents, blocks, err := s.store.Deps(ctxOf(r), id)
	if err != nil {
		respondErr(w, err)
		return
	}
	labels, err := s.store.LabelsForIssue(ctxOf(r), id)
	if err != nil {
		respondErr(w, err)
		return
	}
	comments, err := s.store.Comments(ctxOf(r), id)
	if err != nil {
		respondErr(w, err)
		return
	}
	blocked, err := s.store.IDsBlocked(ctxOf(r), []string{id})
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, issueDetailOut{
		Issue:    issue,
		Labels:   nilToEmpty(labels),
		Depends:  nilToEmpty(parents),
		Blocks:   nilToEmpty(blocks),
		Comments: nilToEmptyComments(comments),
		Blocked:  blocked[id],
	})
}

// createIssueReq is the POST /api/issues body.
//
// Priority is a *int (not int) so we can distinguish "omitted, apply
// default" from "explicitly set to 0". The CLI's --priority defaults
// to 2; the HTTP surface used to decode missing priority as the zero
// value, silently routing every API-created issue to P0.
type createIssueReq struct {
	Title    string `json:"title"`
	Type     string `json:"type,omitempty"`     // default "task"
	Priority *int   `json:"priority,omitempty"` // default 2 (matches CLI)
	// "agent" mirrors the CLI flag; the value lands in the assignee
	// column. nil = the shared unassigned pool.
	Agent        *string  `json:"agent,omitempty"`
	Labels       []string `json:"labels,omitempty"`       // attached after create
	Capabilities []string `json:"capabilities,omitempty"` // cap:<name> labels, expanded server-side (matches CLI --capability)
	Parents      []string `json:"parents,omitempty"`      // dep edges to add
	Description  string   `json:"description,omitempty"`
	Notes        string   `json:"notes,omitempty"`
}

// handleCreateIssue — POST /api/issues
//
// Uses store.CreateWithLinks so labels + parents land atomically with
// the insert.
func (s *Server) handleCreateIssue(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var body createIssueReq
	if err := readJSON(r, &body); err != nil {
		writeError(w, stdhttp.StatusBadRequest, err.Error())
		return
	}
	priority := 2
	if body.Priority != nil {
		priority = *body.Priority
	}
	issue, err := s.store.CreateWithLinks(ctxOf(r), body.Title, body.Type, priority, body.Agent, store.CreateOpts{
		Caps:        body.Capabilities,
		Parents:     body.Parents,
		Description: body.Description,
		Notes:       body.Notes,
	})
	if err != nil {
		respondErr(w, err)
		return
	}
	if len(body.Labels) > 0 {
		if _, err := s.store.AddLabels(ctxOf(r), issue.ID, body.Labels); err != nil {
			respondErr(w, err)
			return
		}
	}
	labels, _ := s.store.LabelsForIssue(ctxOf(r), issue.ID)
	writeJSON(w, stdhttp.StatusCreated, issueOut{Issue: issue, Labels: nilToEmpty(labels)})
}

// patchIssueReq mirrors store.UpdateFields for JSON. jsonOpt encodes
// the JSON tri-state (absent / null / value) so we can distinguish
// "leave alone" from "clear". `tags`, when present, replaces the
// issue's label set wholesale.
type patchIssueReq struct {
	Title       *string   `json:"title,omitempty"`
	Type        *string   `json:"type,omitempty"`
	Status      *string   `json:"status,omitempty"`
	Priority    *int      `json:"priority,omitempty"`
	Assignee    jsonOpt   `json:"assignee,omitempty"`
	Description jsonOpt   `json:"description,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
}

// handlePatchIssue — PATCH /api/issues/{id}
func (s *Server) handlePatchIssue(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	var body patchIssueReq
	if err := readJSON(r, &body); err != nil {
		writeError(w, stdhttp.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.store.Update(ctxOf(r), id, store.UpdateFields{
		Title:       body.Title,
		Type:        body.Type,
		Status:      body.Status,
		Priority:    body.Priority,
		Assignee:    body.Assignee.toDoublePtr(),
		Description: body.Description.toDoublePtr(),
	})
	if err != nil {
		respondErr(w, err)
		return
	}
	if body.Tags != nil {
		if err := replaceLabels(ctxOf(r), s.store, id, *body.Tags); err != nil {
			respondErr(w, err)
			return
		}
	}
	labels, _ := s.store.LabelsForIssue(ctxOf(r), id)
	writeJSON(w, stdhttp.StatusOK, issueOut{Issue: updated, Labels: nilToEmpty(labels)})
}

// handleCloseIssue — POST /api/issues/{id}/close
func (s *Server) handleCloseIssue(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	closed, err := s.store.MarkClosed(ctxOf(r), id)
	if err != nil {
		respondErr(w, err)
		return
	}
	labels, _ := s.store.LabelsForIssue(ctxOf(r), id)
	writeJSON(w, stdhttp.StatusOK, issueOut{Issue: closed, Labels: nilToEmpty(labels)})
}

// handleReopenIssue — POST /api/issues/{id}/reopen
func (s *Server) handleReopenIssue(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	reopened, err := s.store.Reopen(ctxOf(r), id)
	if err != nil {
		respondErr(w, err)
		return
	}
	labels, _ := s.store.LabelsForIssue(ctxOf(r), id)
	writeJSON(w, stdhttp.StatusOK, issueOut{Issue: reopened, Labels: nilToEmpty(labels)})
}

// handleClaimIssue — POST /api/issues/{id}/claim
//
// X-Clu-Agent is the assignee (matches `clu claim -a <name> <id>`).
func (s *Server) handleClaimIssue(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	agent, ok := requireAgent(w, r)
	if !ok {
		return
	}
	claimed, err := s.store.ClaimByID(ctxOf(r), id, agent)
	if err != nil {
		respondErr(w, err)
		return
	}
	labels, _ := s.store.LabelsForIssue(ctxOf(r), id)
	writeJSON(w, stdhttp.StatusOK, issueOut{Issue: claimed, Labels: nilToEmpty(labels)})
}

// ---- helpers ----

// jsonOpt encodes the three states JSON can express for an optional
// nullable string field on a PATCH body:
//
//	field absent     → Set == false  (Update leaves column alone)
//	field is null    → Set == true,  Value == nil (Update clears column)
//	field is "x"     → Set == true,  Value == &"x" (Update writes "x")
//
// toDoublePtr emits the **string the store.UpdateFields struct expects.
type jsonOpt struct {
	Set   bool
	Value *string
}

func (o *jsonOpt) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	// Treat empty string as null: PATCH {"assignee":""} unassigns,
	// matching `clu update --unassign`. Storing the literal "" would
	// produce a phantom non-null assignee with zero meaning — neither
	// claimed nor unclaimed — that `list -a ""` could match.
	if v == "" {
		o.Value = nil
		return nil
	}
	o.Value = &v
	return nil
}

func (o jsonOpt) toDoublePtr() **string {
	if !o.Set {
		return nil
	}
	return &o.Value
}

// parseListFilter pulls every supported query param into a ListFilter.
// Unknown params are silently ignored.
func parseListFilter(r *stdhttp.Request) (store.ListFilter, error) {
	q := r.URL.Query()
	f := store.ListFilter{}

	if v := q["status"]; len(v) > 0 {
		f.Statuses = v
	}
	// "agent" is the user-facing flag name (per the sticky decision in
	// CLAUDE.md); under the hood it maps to the assignee column.
	if v := q.Get("agent"); v != "" {
		f.Assignee = &v
	}
	if v := q.Get("type"); v != "" {
		f.Type = v
	}
	if v := q["label"]; len(v) > 0 {
		f.Labels = v
	}
	if v := q["label_any"]; len(v) > 0 {
		f.LabelsAny = v
	}
	if v := q["tag"]; len(v) > 0 {
		f.LabelsAny = append(f.LabelsAny, v...)
	}
	if isTrue(q.Get("no_labels")) {
		f.NoLabels = true
	}
	if isTrue(q.Get("no_assignee")) {
		f.NoAssignee = true
	}
	if v := q.Get("q"); v != "" {
		f.TitleContains = v
	}
	if v := q.Get("desc"); v != "" {
		f.DescContains = v
	}
	if v := q.Get("priority_min"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return f, errInvalid("priority_min")
		}
		f.PriorityMin = &n
	}
	if v := q.Get("priority_max"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return f, errInvalid("priority_max")
		}
		f.PriorityMax = &n
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, errInvalid("limit")
		}
		f.Limit = n
	}
	if v := q.Get("sort"); v != "" {
		f.Sort = v
	}
	if isTrue(q.Get("reverse")) {
		f.Reverse = true
	}
	return f, nil
}

func isTrue(s string) bool {
	switch strings.ToLower(s) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func errInvalid(field string) error { return &fieldErr{field: field} }

type fieldErr struct{ field string }

func (e *fieldErr) Error() string { return "invalid value for " + e.field }

// issueIDs extracts the IDs from a slice. Used to batch label/blocked loads.
func issueIDs(issues []store.Issue) []string {
	out := make([]string, len(issues))
	for i, is := range issues {
		out[i] = is.ID
	}
	return out
}

// replaceLabels diffs the issue's current labels against `want` and
// adds/removes the deltas. Used by PATCH when the client sends a
// `tags` array — UI semantics is "this is the new label set".
//
// Workflow-internal labels (run:*, step:*, checkpoint:*, cap:*) are
// preserved automatically: only the user-managed prefix-less subset is
// considered for removal. Without this, dragging a card or saving
// edits would strip the workflow bookkeeping.
func replaceLabels(ctx context.Context, s *store.Store, id string, want []string) error {
	current, err := s.LabelsForIssue(ctx, id)
	if err != nil {
		return err
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, l := range want {
		wantSet[l] = struct{}{}
	}
	currentSet := make(map[string]struct{}, len(current))
	for _, l := range current {
		currentSet[l] = struct{}{}
	}
	var toAdd, toRemove []string
	for l := range wantSet {
		if _, ok := currentSet[l]; !ok {
			toAdd = append(toAdd, l)
		}
	}
	for _, l := range current {
		if isManagedLabel(l) {
			continue
		}
		if _, ok := wantSet[l]; !ok {
			toRemove = append(toRemove, l)
		}
	}
	if len(toAdd) > 0 {
		if _, err := s.AddLabels(ctx, id, toAdd); err != nil {
			return err
		}
	}
	if len(toRemove) > 0 {
		if _, err := s.RemoveLabels(ctx, id, toRemove); err != nil {
			return err
		}
	}
	return nil
}

// isManagedLabel reports whether a label is set by clu internals (workflows,
// checkpoints, capabilities) and must not be touched by user-facing tag edits.
func isManagedLabel(l string) bool {
	for _, p := range []string{"run:", "step:", "checkpoint:", "cap:", "template:"} {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

// nilToEmpty turns a nil string slice into an empty one so JSON serialises
// as `[]` rather than `null` — saner for the UI.
func nilToEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nilToEmptyComments(s []store.Comment) []store.Comment {
	if s == nil {
		return []store.Comment{}
	}
	return s
}
