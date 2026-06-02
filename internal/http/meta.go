package http

import (
	stdhttp "net/http"

	"github.com/Rovak/agents-clu/internal/store"
)

// metaOut is the single-call payload the web UI uses to populate
// dropdowns: valid statuses/types, the project's ID prefix, the schema
// version. Cheap to compute; called once on app load.
type metaOut struct {
	Statuses []string `json:"statuses"`
	Types    []string `json:"types"`
	IDPrefix string   `json:"id_prefix"`
	Schema   int      `json:"schema_version"`
}

// handleMeta — GET /api/meta
func (s *Server) handleMeta(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	v, _ := s.store.DBVersion(ctxOf(r))
	writeJSON(w, stdhttp.StatusOK, metaOut{
		Statuses: append([]string(nil), store.ValidStatuses...),
		Types:    append([]string(nil), store.ValidTypes...),
		IDPrefix: s.store.IDPrefix(),
		Schema:   v,
	})
}

// handleAgents — GET /api/agents
//
// Returns the live agent list (heartbeat freshness == default). Used
// by the identity picker dropdown so users can pick from agents that
// are actually running, plus see who's idle.
func (s *Server) handleAgents(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	agents, err := s.store.AgentList(ctxOf(r), store.AgentStaleThresholdSec)
	if err != nil {
		respondErr(w, err)
		return
	}
	if agents == nil {
		agents = []store.ActiveAgent{}
	}
	writeJSON(w, stdhttp.StatusOK, agents)
}

// handleTags — GET /api/tags
//
// Distinct labels excluding workflow-managed prefixes. Used to
// populate the tag filter dropdown in the list view.
func (s *Server) handleTags(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	all, err := s.store.AllLabels(ctxOf(r))
	if err != nil {
		respondErr(w, err)
		return
	}
	out := make([]string, 0, len(all))
	for _, l := range all {
		if isManagedLabel(l) {
			continue
		}
		out = append(out, l)
	}
	writeJSON(w, stdhttp.StatusOK, out)
}
