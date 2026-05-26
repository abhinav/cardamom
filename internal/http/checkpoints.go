package http

import (
	stdhttp "net/http"
)

type resolveCheckpointReq struct {
	Reason string `json:"reason,omitempty"`
}

// handleApproveCheckpoint — POST /api/checkpoints/{id}/approve
//
// X-Clu-Agent is the approver identity; store.ResolveCheckpoint checks
// it against the cp:<id> payload's approver list for approval-kind
// checkpoints. body.reason (optional) is appended to the issue's notes.
func (s *Server) handleApproveCheckpoint(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	s.resolveCheckpoint(w, r, true)
}

// handleFailCheckpoint — POST /api/checkpoints/{id}/fail
//
// Cascades a cancel through downstream issues; same engine as the CLI's
// `clu checkpoint fail`.
func (s *Server) handleFailCheckpoint(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	s.resolveCheckpoint(w, r, false)
}

func (s *Server) resolveCheckpoint(w stdhttp.ResponseWriter, r *stdhttp.Request, pass bool) {
	id := r.PathValue("id")
	agent, ok := requireAgent(w, r)
	if !ok {
		return
	}
	var body resolveCheckpointReq
	// Body is optional — POST .../approve with no body is fine.
	if r.ContentLength > 0 {
		if err := readJSON(r, &body); err != nil {
			writeError(w, stdhttp.StatusBadRequest, err.Error())
			return
		}
	}
	res, err := s.store.ResolveCheckpoint(ctxOf(r), id, agent, pass, body.Reason)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, stdhttp.StatusOK, res)
}
