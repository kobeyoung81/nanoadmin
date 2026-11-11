package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"gitee.com/kobeyoung81/nanoadmin/internal/store"
	"gitee.com/kobeyoung81/nanoprotocol"
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

// The scalar half of one mode's override (model, thinking level, settings).
// Ownership and the accepted thinking levels are the bus's checks — it serves
// the user plane — so this face takes the patch it has already validated.

type modeOverrideBody struct {
	ModelID      *int64             `json:"model_id"`
	ClearModelID bool               `json:"clear_model_id"`
	Thinking     *string            `json:"thinking"`
	Settings     *map[string]string `json:"settings"`
}

func (s *Server) internalPatchModeOverride(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	typeID := r.PathValue("typeID")
	if agentID == "" || typeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent and type are required"})
		return
	}
	var body modeOverrideBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := s.catalog.PatchAgentTypeOverride(agentID, typeID, store.ModeOverridePatch{
		ModelID: body.ModelID, ClearModelID: body.ClearModelID,
		Thinking: body.Thinking, Settings: body.Settings,
	}); err != nil {
		slog.Error("patch mode override", "agent", agentID, "type", typeID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"agent_id": agentID, "type_id": typeID, "status": "updated"})
}

// One mode's permission rules. `append` adds or replaces one rule at a time (the
// agent runtime's message); without it the list replaces what is stored (the
// console's form).

type modePermissionsBody struct {
	TypeID      string                        `json:"type_id"`
	Permissions []nanoprotocol.PermissionRule `json:"permissions"`
	Append      bool                          `json:"append"`
}

func (s *Server) internalSetModePermissions(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	var body modePermissionsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if agentID == "" || body.TypeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent and type are required"})
		return
	}
	if body.Append {
		for _, rule := range body.Permissions {
			if err := s.catalog.AppendModePermission(agentID, body.TypeID, rule); err != nil {
				slog.Error("append mode permission", "agent", agentID, "type", body.TypeID, "error", err)
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}
	} else if err := s.catalog.SetModePermissions(agentID, body.TypeID, body.Permissions); err != nil {
		slog.Error("set mode permissions", "agent", agentID, "type", body.TypeID, "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"agent_id": agentID, "type_id": body.TypeID, "status": "updated"})
}
