package http

import (
	stdhttp "net/http"

	"github.com/Rovak/agents-clu/internal/store"
)

// pendingCheckpointOut extends store.PendingCheckpoint with the
// derived "blocks" list — the issues this checkpoint gates. Useful
// context for the approval UI: shows what shipping (or cancelling)
// will unblock.
type pendingCheckpointOut struct {
	store.PendingCheckpoint
	Blocks []string `json:"blocks"`
	Labels []string `json:"labels"`
}

// handleListCheckpoints — GET /api/checkpoints
//
// Returns every open checkpoint with its parsed approvers payload
// plus the issues it currently blocks. Powers the /approvals page.
// Empty array (never null) when there's nothing pending.
func (s *Server) handleListCheckpoints(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	pending, err := s.store.PendingCheckpoints(ctxOf(r))
	if err != nil {
		respondErr(w, err)
		return
	}
	out := make([]pendingCheckpointOut, len(pending))
	for i, p := range pending {
		_, blocks, dErr := s.store.Deps(ctxOf(r), p.ID)
		if dErr != nil {
			respondErr(w, dErr)
			return
		}
		labels, lErr := s.store.LabelsForIssue(ctxOf(r), p.ID)
		if lErr != nil {
			respondErr(w, lErr)
			return
		}
		out[i] = pendingCheckpointOut{
			PendingCheckpoint: p,
			Blocks:            nilToEmpty(blocks),
			Labels:            nilToEmpty(labels),
		}
	}
	writeJSON(w, stdhttp.StatusOK, out)
}

type resolveCheckpointReq struct {
	Reason string `json:"reason,omitempty"`
}

// handleApproveCheckpoint — POST /api/checkpoints/{id}/approve
//
// X-Clu-Agent is the approver identity, recorded on the checkpoint.
// It is informational only — the declared approver list is not enforced
// (single-user model). body.reason (optional) is appended to the notes.
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
