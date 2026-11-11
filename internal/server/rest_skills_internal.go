package server

import (
	"encoding/json"
	"net/http"

	"gitee.com/kobeyoung81/nanoadmin/internal/skillbundle"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
	"gitee.com/kobeyoung81/nanoprotocol"
)

// The agent runtime's skill calls, as HTTP: the bus forwards the protocol
// messages it still receives. A user-scoped skill is the only thing this face
// creates or deletes; platform skills are the console's.

type skillSaveBody struct {
	ID          string            `json:"id"`
	UserID      string            `json:"user_id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Files       map[string]string `json:"files"`
	Origin      string            `json:"origin"`
	CreatedBy   string            `json:"created_by"`
}

type skillDeleteBody struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

type skillAckBody struct {
	UserID  string `json:"user_id"`
	SkillID string `json:"skill_id"`
	Version int    `json:"version"`
}

func (s *Server) internalSkillSave(w http.ResponseWriter, r *http.Request) {
	var body skillSaveBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Name == "" || len(body.Files) == 0 || body.UserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, files and user_id are required"})
		return
	}
	if !validSkillName(body.Name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid skill name"})
		return
	}
	origin := body.Origin
	if origin != "agent" {
		origin = "human"
	}
	taken, err := s.catalog.SkillNameTaken(body.Name, body.UserID, body.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if taken {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a skill with this name already exists"})
		return
	}

	rec := store.SkillRecord{
		Capability: store.Capability{
			ID: body.ID, Scope: "user", OwnerUserID: body.UserID,
			Name: body.Name, Description: body.Description,
		},
		Files: body.Files, Origin: origin, Status: "published",
		ContentHash: skillbundle.ContentHash(body.Files), CreatedBy: body.CreatedBy,
	}
	if rec.ID == "" {
		rec.ID = capabilitySlug("skill", body.Name) + "-" + randomSuffix()
		if err := s.catalog.CreateSkill(rec); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
	} else {
		existing, err := s.catalog.GetSkill(rec.ID)
		if err != nil || existing == nil || existing.Scope != "user" || existing.OwnerUserID != body.UserID {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found or not owned"})
			return
		}
		// User skills are overwritten in place; provenance and versioning stay.
		rec.Origin = existing.Origin
		rec.Version = existing.Version
		rec.PublishedVersion = existing.PublishedVersion
		if err := s.catalog.UpdateSkill(rec.ID, rec); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
	}
	s.notifyCapabilitiesChanged("skill")

	stored, _ := s.catalog.GetSkill(rec.ID)
	def := nanoprotocol.SkillDef{}
	if stored != nil {
		def = skillDef(*stored)
	}
	writeJSON(w, http.StatusOK, def)
}

func (s *Server) internalSkillDelete(w http.ResponseWriter, r *http.Request) {
	var body skillDeleteBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.UserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}
	recID := body.ID
	if recID == "" && body.Name != "" {
		found, err := s.catalog.GetUserSkillByName(body.UserID, body.Name)
		if err != nil || found == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found or not owned"})
			return
		}
		recID = found.ID
	}
	if recID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id or name is required"})
		return
	}
	existing, err := s.catalog.GetSkill(recID)
	if err != nil || existing == nil || existing.Scope != "user" || existing.OwnerUserID != body.UserID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found or not owned"})
		return
	}
	if err := s.catalog.DeleteSkill(recID, body.UserID, "user"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	s.notifyCapabilitiesChanged("skill")
	writeJSON(w, http.StatusOK, map[string]string{"id": recID})
}

func (s *Server) internalSkillAck(w http.ResponseWriter, r *http.Request) {
	var body skillAckBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.UserID == "" || body.SkillID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id and skill_id are required"})
		return
	}
	if err := s.catalog.AckSkill(body.UserID, body.SkillID, body.Version); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skill_id": body.SkillID, "version": body.Version})
}

// validSkillName keeps the catalogue readable: a name becomes a directory under
// the agent's skills root.
func validSkillName(name string) bool {
	if name == "" || len(name) > 64 || name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}
