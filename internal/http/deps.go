package http

import (
	stdhttp "net/http"
)

type addDepReq struct {
	Parent string `json:"parent"`
}

// handleAddDep — POST /api/issues/{id}/deps
//
// {id} is the child (the issue that depends), body.parent is what it
// depends on. Wraps store.AddDep, which validates both exist and
// rejects cycles via the recursive-CTE check.
func (s *Server) handleAddDep(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	var body addDepReq
	if err := readJSON(r, &body); err != nil {
		writeError(w, stdhttp.StatusBadRequest, err.Error())
		return
	}
	if body.Parent == "" {
		writeError(w, stdhttp.StatusBadRequest, "parent required")
		return
	}
	if err := s.store.AddDep(ctxOf(r), id, body.Parent); err != nil {
		respondErr(w, err)
		return
	}
	respondDeps(w, r, s, id)
}

// handleRemoveDep — DELETE /api/issues/{id}/deps/{parent}
func (s *Server) handleRemoveDep(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	parent := r.PathValue("parent")
	if err := s.store.RemoveDep(ctxOf(r), id, parent); err != nil {
		respondErr(w, err)
		return
	}
	respondDeps(w, r, s, id)
}

// respondDeps emits the issue's current parent + blocks lists. Shared
// by AddDep and RemoveDep so the client can refresh its dep view
// without a separate fetch.
func respondDeps(w stdhttp.ResponseWriter, r *stdhttp.Request, s *Server, id string) {
	parents, blocks, err := s.store.Deps(ctxOf(r), id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, map[string]any{
		"depends_on": nilToEmpty(parents),
		"blocks":     nilToEmpty(blocks),
	})
}
