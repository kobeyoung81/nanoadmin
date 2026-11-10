package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// restEffectiveModels resolves the model an agent's default mode ends up using.
//
// The agent rows moved to the portal service; the catalogue (types, per-agent
// overrides, models) lives here. The portal cannot answer "which model does this
// agent show" without the catalogue, and asking per agent would cost a round
// trip per row, so it sends the whole list and gets one map back.
//
// The caller supplies the mode id, because it comes from the agent's own
// document (its default session agent), which this service does not own.
func (s *Server) restEffectiveModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Agents []struct {
			AgentID string `json:"agent_id"`
			TypeID  string `json:"type_id"`
		} `json:"agents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	type entry struct {
		ModelID int64  `json:"model_id"`
		Name    string `json:"name"`
	}
	out := make(map[string]entry, len(body.Agents))
	for _, a := range body.Agents {
		if a.AgentID == "" {
			continue
		}
		typeID := a.TypeID
		if typeID == "" {
			typeID = s.catalog.DefaultAgentTypeID()
		}
		modelID := s.catalog.DefaultModelForType(typeID)
		if ov, err := s.catalog.GetAgentTypeOverride(a.AgentID, typeID); err == nil && ov != nil && ov.ModelID != 0 {
			modelID = s.catalog.EffectiveModelIDForType(typeID, ov.ModelID)
		}
		name := ""
		if m, err := s.catalog.GetModel(modelID); err == nil && m != nil {
			name = m.Name
		}
		out[a.AgentID] = entry{ModelID: modelID, Name: name}
	}
	slog.Debug("resolved effective models", "agents", len(body.Agents))
	writeJSON(w, http.StatusOK, map[string]any{"models": out})
}
