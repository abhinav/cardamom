// Package http is the REST API server that backs `clu web`. It exposes
// the internal/store surface as JSON over net/http so the web UI (and
// any other client) can drive the tracker without shelling out.
//
// Layout mirrors internal/store:
//
//	server.go       Server struct, mux, helpers
//	issues.go       /api/issues (list, create, get, patch, close, reopen, claim)
//	labels.go       /api/issues/:id/labels
//	deps.go         /api/issues/:id/deps
//	comments.go     /api/issues/:id/comments
//	checkpoints.go  /api/checkpoints/:id/approve|fail
//	meta.go         /api/meta, /api/agents, /api/tags
package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"strings"

	"github.com/rovak/clu/internal/store"
)

// Server is the REST API bound to a single Store. Construct with New
// and pass to http.Server (or call Mux for tests).
type Server struct {
	store *store.Store
}

// New constructs a Server.
func New(s *store.Store) *Server {
	return &Server{store: s}
}

// Mux returns the routed handler. Routes use Go 1.22 method-aware
// patterns ("GET /api/issues", "PATCH /api/issues/{id}"); the package
// requires Go ≥ 1.22, which the project already uses (see go.mod).
func (s *Server) Mux() stdhttp.Handler {
	mux := stdhttp.NewServeMux()

	mux.HandleFunc("GET /api/healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/meta", s.handleMeta)
	mux.HandleFunc("GET /api/agents", s.handleAgents)
	mux.HandleFunc("GET /api/tags", s.handleTags)

	mux.HandleFunc("GET /api/issues", s.handleListIssues)
	mux.HandleFunc("POST /api/issues", s.handleCreateIssue)
	mux.HandleFunc("GET /api/issues/{id}", s.handleGetIssue)
	mux.HandleFunc("PATCH /api/issues/{id}", s.handlePatchIssue)
	mux.HandleFunc("POST /api/issues/{id}/close", s.handleCloseIssue)
	mux.HandleFunc("POST /api/issues/{id}/reopen", s.handleReopenIssue)
	mux.HandleFunc("POST /api/issues/{id}/claim", s.handleClaimIssue)

	mux.HandleFunc("POST /api/issues/{id}/labels", s.handleAddLabels)
	mux.HandleFunc("DELETE /api/issues/{id}/labels/{label}", s.handleRemoveLabel)

	mux.HandleFunc("POST /api/issues/{id}/deps", s.handleAddDep)
	mux.HandleFunc("DELETE /api/issues/{id}/deps/{parent}", s.handleRemoveDep)

	mux.HandleFunc("GET /api/issues/{id}/comments", s.handleListComments)
	mux.HandleFunc("POST /api/issues/{id}/comments", s.handleAddComment)

	mux.HandleFunc("POST /api/checkpoints/{id}/approve", s.handleApproveCheckpoint)
	mux.HandleFunc("POST /api/checkpoints/{id}/fail", s.handleFailCheckpoint)

	return withCORS(mux)
}

// agentHeader is the request header carrying the caller's identity
// (the value passed as `-a` on the CLI side). Used by claim/comment/
// checkpoint endpoints.
const agentHeader = "X-Clu-Agent"

// agentFrom extracts the X-Clu-Agent header, trimmed. Empty string
// means "no agent identity supplied".
func agentFrom(r *stdhttp.Request) string {
	return strings.TrimSpace(r.Header.Get(agentHeader))
}

// agentPtr is the *string convention the store uses for assignee/agent.
func agentPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// requireAgent is for endpoints that need an identity (claim, comment,
// checkpoint). Writes a 400 and returns "", false if the header is
// missing — the handler should return immediately.
func requireAgent(w stdhttp.ResponseWriter, r *stdhttp.Request) (string, bool) {
	a := agentFrom(r)
	if a == "" {
		writeError(w, stdhttp.StatusBadRequest, "missing "+agentHeader+" header")
		return "", false
	}
	return a, true
}

// withCORS wraps a handler so the dev frontend (different origin/port
// than the API in dev mode) can call it. Localhost-only deployment;
// no need for an allowlist.
func withCORS(h stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+agentHeader)
		if r.Method == stdhttp.MethodOptions {
			w.WriteHeader(stdhttp.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// writeJSON encodes v as JSON with HTML escaping off (matches the CLI's
// emitJSON convention). Falls back to a 500 if encoding fails after
// headers are sent — rare, but worth logging eventually.
func writeJSON(w stdhttp.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// errorBody is the JSON shape for every non-2xx response.
type errorBody struct {
	Error string `json:"error"`
}

func writeError(w stdhttp.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

// readJSON decodes the request body into v with a hard cap and
// DisallowUnknownFields so typos in clients surface as 400s instead of
// silent no-ops. 1 MB cap is plenty for any single-issue payload.
func readJSON(r *stdhttp.Request, v any) error {
	r.Body = stdhttp.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// errorStatus maps store sentinel errors to HTTP status codes. Anything
// unmapped is a 500.
func errorStatus(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound),
		errors.Is(err, store.ErrKVNotFound),
		errors.Is(err, store.ErrCommentNotFound),
		errors.Is(err, store.ErrDepNotFound),
		errors.Is(err, store.ErrCheckpointNoPayload):
		return stdhttp.StatusNotFound
	case errors.Is(err, store.ErrAlreadyClosed),
		errors.Is(err, store.ErrAlreadyOpen),
		errors.Is(err, store.ErrNotClaimable),
		errors.Is(err, store.ErrCheckpointClosed):
		return stdhttp.StatusConflict
	case errors.Is(err, store.ErrCycle),
		errors.Is(err, store.ErrSelfDep),
		errors.Is(err, store.ErrNotDeferred),
		errors.Is(err, store.ErrNotCheckpoint),
		errors.Is(err, store.ErrNotApprover):
		return stdhttp.StatusBadRequest
	}
	return stdhttp.StatusInternalServerError
}

// respondErr writes the right status + JSON body for a store error.
func respondErr(w stdhttp.ResponseWriter, err error) {
	writeError(w, errorStatus(err), err.Error())
}

// handleHealthz is the no-op liveness probe `clu web` polls before
// loading the page.
func (s *Server) handleHealthz(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	writeJSON(w, stdhttp.StatusOK, map[string]any{"ok": true})
}

// ctxOf is a tiny shim so handler functions don't have to keep typing
// r.Context() everywhere; mostly here to make tests easier to rewire
// later (e.g. injecting a deadline).
func ctxOf(r *stdhttp.Request) context.Context { return r.Context() }
