package server

import (
	"encoding/json"
	"net/http"
)

// Per-(agent, mode) capability attachments. The agent rows and the mode the
// caller means are decided by the bus, which owns that read; this service owns the
// override document the attachment lives in, so the write lands here.

type modeCapabilityBody struct {
	TypeID  string `json:"type_id"`
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	Enabled bool   `json:"enabled"`
}

func (s *Server) internalSetAgentCapability(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	var body modeCapabilityBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if agentID == "" || body.TypeID == "" || body.Kind == "" || body.Ref == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent, type, kind and ref are required"})
		return
	}
	if err := s.catalog.SetModeCapability(agentID, body.TypeID, body.Kind, body.Ref, body.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	s.notifyCapabilitiesChanged(body.Kind)
	writeJSON(w, http.StatusOK, map[string]string{"agent_id": agentID, "status": "updated"})
}

func (s *Server) internalDeleteAgentCapability(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	var body modeCapabilityBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if agentID == "" || body.TypeID == "" || body.Kind == "" || body.Ref == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent, type, kind and ref are required"})
		return
	}
	if err := s.catalog.DeleteModeCapability(agentID, body.TypeID, body.Kind, body.Ref); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	s.notifyCapabilitiesChanged(body.Kind)
	writeJSON(w, http.StatusOK, map[string]string{"agent_id": agentID, "status": "deleted"})
}
