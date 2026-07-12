package http

import (
	stdhttp "net/http"
)

type addLabelsReq struct {
	Labels []string `json:"labels"`
}

// handleAddLabels — POST /api/issues/{id}/labels
//
// Adds one or more labels to the issue. Idempotent — re-adding a
// present label is a no-op (store handles via INSERT … ON CONFLICT).
func (s *Server) handleAddLabels(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	var body addLabelsReq
	if err := readJSON(r, &body); err != nil {
		writeError(w, stdhttp.StatusBadRequest, err.Error())
		return
	}
	if len(body.Labels) == 0 {
		writeError(w, stdhttp.StatusBadRequest, "labels required")
		return
	}
	if _, err := s.store.AddLabels(ctxOf(r), id, body.Labels); err != nil {
		respondErr(w, err)
		return
	}
	labels, err := s.store.LabelsForIssue(ctxOf(r), id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, map[string]any{"labels": nilToEmpty(labels)})
}

// handleRemoveLabel — DELETE /api/issues/{id}/labels/{label}
//
// Removes a single label. The label is in the URL (URL-decoded by the
// router) rather than the body so a UI can fire-and-forget DELETEs
// without serialising a body.
func (s *Server) handleRemoveLabel(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id := r.PathValue("id")
	label := r.PathValue("label")
	removed, err := s.store.RemoveLabels(ctxOf(r), id, []string{label})
	if err != nil {
		respondErr(w, err)
		return
	}
	// 404 when the label isn't on the issue (mirrors the CLI's honest
	// no-op count for `label rm`). Otherwise a scripted DELETE has no
	// way to tell whether the call changed anything.
	if removed == 0 {
		writeError(w, stdhttp.StatusNotFound, "label "+label+" is not on issue "+id)
		return
	}
	labels, err := s.store.LabelsForIssue(ctxOf(r), id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, map[string]any{"labels": nilToEmpty(labels)})
}
