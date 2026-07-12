package http

import (
	stdhttp "net/http"
)

// handleListComments — GET /api/issues/{id}/comments
func (s *Server) handleListComments(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	comments, err := s.store.Comments(ctxOf(r), id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, nilToEmptyComments(comments))
}

type addCommentReq struct {
	Body string `json:"body"`
}

// handleAddComment — POST /api/issues/{id}/comments
//
// Author is the X-Clu-Agent header (matches `clu comment add -a foo`).
// Without the header we return 400 — comments are always attributed,
// not anonymous.
func (s *Server) handleAddComment(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	author, ok := requireAgent(w, r)
	if !ok {
		return
	}
	var body addCommentReq
	if err := readJSON(r, &body); err != nil {
		writeError(w, stdhttp.StatusBadRequest, err.Error())
		return
	}
	if body.Body == "" {
		writeError(w, stdhttp.StatusBadRequest, "body required")
		return
	}
	c, err := s.store.AddComment(ctxOf(r), id, author, body.Body)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusCreated, c)
}
