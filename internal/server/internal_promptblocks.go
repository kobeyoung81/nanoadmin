package server

// The prompt blocks the runtime and the user plane read.
//
// The bus used to answer the user-facing prompt-block view by reading this module's
// session_head_prompts and agent types. Both are this module's, so the answer is
// composed here: the platform head, the agent's own head override (when one exists)
// and the two mode switch notes.

import (
	"log/slog"
	"net/http"

	"gitee.com/kobeyoung81/nanoprotocol"
)

// restInternalPromptBlocks answers one agent's effective prompt blocks.
func (s *Server) restInternalPromptBlocks(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id is required"})
		return
	}
	platform, err := s.catalog.PlatformSessionHead()
	if err != nil {
		slog.Error("get session head prompt", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	own, err := s.catalog.AgentSessionHead(agentID)
	if err != nil {
		slog.Error("get agent session head prompt", "agent_id", agentID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	head, source := platform.Content, "platform"
	if own != nil && own.Content != "" {
		head, source = own.Content, "agent"
	}
	notes := map[string]string{}
	for _, id := range modeNoteIDs {
		var t *nanoprotocol.AgentTypeDef
		t, err = s.catalog.GetAgentType(id)
		if err != nil {
			slog.Error("get mode note", "type_id", id, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if t != nil {
			notes[id] = t.Prompt
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"head":        head,
		"head_source": source,
		"mode_notes":  notes,
	})
}

// restInternalModel answers one model, and one provider, for the callers that need
// the display name or the endpoint (the bus's voice surface and the agent's info).
func (s *Server) restInternalModel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseInt64(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	model, err := s.catalog.GetModel(id)
	if err != nil {
		slog.Error("get model", "id", id, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if model == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"model": model})
}

func (s *Server) restInternalModelProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := parseInt64(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	provider, err := s.catalog.GetModelProvider(id)
	if err != nil {
		slog.Error("get model provider", "id", id, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if provider == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": provider})
}

// parseInt64 parses a path value that names a row by number.
func parseInt64(raw string) (int64, bool) {
	var out int64
	for _, c := range raw {
		if c < '0' || c > '9' {
			return 0, false
		}
		out = out*10 + int64(c-'0')
	}
	return out, raw != ""
}

// restInternalAgentMode answers one (agent, mode)'s baseline type and the user's
// override, which is what the per-agent mode editor reads.
func (s *Server) restInternalAgentMode(w http.ResponseWriter, r *http.Request) {
	agentID, typeID := r.PathValue("agentID"), r.PathValue("typeID")
	at, err := s.catalog.GetAgentType(typeID)
	if err != nil {
		slog.Error("get agent type", "type_id", typeID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	ov, err := s.catalog.GetAgentTypeOverride(agentID, typeID)
	if err != nil {
		slog.Error("get agent type override", "agent_id", agentID, "type_id", typeID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"type": at, "override": ov})
}

// restInternalAgentTypes answers the type catalogue the runtime asks for.
func (s *Server) restInternalAgentTypes(w http.ResponseWriter, r *http.Request) {
	sessionOnly := r.URL.Query().Get("session_only") == "1"
	types, err := s.catalog.ListAgentTypes(sessionOnly)
	if err != nil {
		slog.Error("list agent types", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"types": types})
}
