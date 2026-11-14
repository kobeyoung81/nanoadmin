package server

// Resolving one agent's runtime from the catalogue.
//
// The agent starts by asking the bus for a snapshot; the bus used to read this
// module's collections to build it. The collections are this module's, so the
// resolution is here now and the bus asks over the internal face. What the answer
// cannot include is which types one agent runs — that is the agent row's, and the
// rows are the portal's — so the caller passes those facts in.

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"gitee.com/kobeyoung81/nanoadmin/internal/store"
)

// restResolveAgentRuntime answers the runtime view of one agent: the capability
// catalogue for its default mode, its effective settings, its definitions and its
// primary type.
func (s *Server) restResolveAgentRuntime(w http.ResponseWriter, r *http.Request) {
	var in store.RuntimeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if in.AgentID == "" || in.UserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id and user_id are required"})
		return
	}
	resolution, err := s.catalog.ResolveRuntime(in)
	if err != nil {
		slog.Error("resolve agent runtime", "agent_id", in.AgentID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, resolution)
}

// restResolveModeCapabilities answers one (agent, mode) catalogue snapshot. The
// per-agent capability pages ask for a mode other than the default.
func (s *Server) restResolveModeCapabilities(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AgentID string `json:"agent_id"`
		UserID  string `json:"user_id"`
		TypeID  string `json:"type_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if in.AgentID == "" || in.UserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id and user_id are required"})
		return
	}
	caps, err := s.catalog.ResolveModeCapabilities(in.AgentID, in.UserID, in.TypeID)
	if err != nil {
		slog.Error("resolve mode capabilities", "agent_id", in.AgentID, "type_id", in.TypeID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, caps)
}
