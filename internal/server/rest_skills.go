package server

// The skill catalogue's HTTP face: the console manages definitions, users manage
// their own, and the agent runtime's save/delete/ack arrive through the internal
// endpoints below (the bus forwards them).

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"gitee.com/kobeyoung81/nanoadmin/internal/skillbundle"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
	"gitee.com/kobeyoung81/nanoprotocol"
)

type skillBody struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Files          map[string]string `json:"files"`
	DefaultEnabled bool              `json:"default_enabled"`
	Locked         string            `json:"locked,omitempty"`
}

func (b skillBody) record(scope, owner string) store.SkillRecord {
	id := b.ID
	if id == "" {
		id = capabilitySlug("skill", b.Name) + "-" + randomSuffix()
	}
	origin := "human"
	if scope == "platform" {
		origin = "platform"
	}
	return store.SkillRecord{
		Capability: store.Capability{
			ID:             id,
			Scope:          scope,
			OwnerUserID:    owner,
			Name:           b.Name,
			Description:    b.Description,
			DefaultEnabled: b.DefaultEnabled,
			Locked:         b.Locked,
		},
		Files:  b.Files,
		Origin: origin,
		Status: "published",
	}
}

func skillDef(rec store.SkillRecord) nanoprotocol.SkillDef {
	return nanoprotocol.SkillDef{
		ID:               rec.ID,
		Scope:            rec.Scope,
		OwnerUserID:      rec.OwnerUserID,
		Name:             rec.Name,
		Description:      rec.Description,
		Files:            rec.Files,
		DefaultEnabled:   rec.DefaultEnabled,
		Locked:           rec.Locked,
		Origin:           rec.Origin,
		Version:          rec.Version,
		Status:           rec.Status,
		PublishedVersion: rec.PublishedVersion,
		ContentHash:      rec.ContentHash,
		CreatedBy:        rec.CreatedBy,
	}
}
func capabilitySlug(prefix, name string) string {
	slug := strings.ToLower(name)
	var b strings.Builder
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		b.WriteString("cap")
	}
	// Trim to keep the composite id within VARCHAR(96).
	out := b.String()
	if len(out) > 48 {
		out = out[:48]
	}
	return prefix + ":" + out
}

func randomSuffix() string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func (s *Server) restListSkillsAdmin(w http.ResponseWriter, r *http.Request) {
	recs, err := s.catalog.ListSkills("")
	if err != nil {
		slog.Error("list skills", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	out := make([]nanoprotocol.SkillDef, 0, len(recs))
	for _, rec := range recs {
		out = append(out, skillDef(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": out, "count": len(out)})
}

func (s *Server) restCreateSkillAdmin(w http.ResponseWriter, r *http.Request) {
	var body skillBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Name == "" || len(body.Files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and files are required"})
		return
	}
	if taken, err := s.catalog.SkillNameTaken(body.Name, "", ""); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	} else if taken {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a skill with this name already exists"})
		return
	}
	rec := body.record("platform", "")
	rec.ContentHash = skillbundle.ContentHash(body.Files)
	if err := s.catalog.CreateSkill(rec); err != nil {
		slog.Error("create skill", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.notifyCapabilitiesChanged("skill")
	writeJSON(w, http.StatusCreated, map[string]string{"id": rec.ID, "status": "created"})
}

func (s *Server) restUpdateSkillAdmin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body skillBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	rec := body.record("platform", "")
	rec.ID = id
	existing, err := s.catalog.GetSkill(id)
	if err != nil || existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}
	if existing.Origin == "bundled" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "bundled skills are read-only"})
		return
	}
	if taken, terr := s.catalog.SkillNameTaken(body.Name, "", id); terr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	} else if taken {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a skill with this name already exists"})
		return
	}
	rec.Origin = existing.Origin
	rec.Version = existing.Version + 1
	rec.PublishedVersion = existing.PublishedVersion + 1
	rec.Status = "published"
	rec.ContentHash = skillbundle.ContentHash(body.Files)
	if err := s.catalog.UpdateSkill(id, rec); err != nil {
		slog.Error("update skill", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	s.notifyCapabilitiesChanged("skill")
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "updated"})
}

// restUpdateSkillState updates only the definition-plane default (default_enabled
// / locked) for a skill. It works for read-only bundled skills too, whose content
// cannot be edited but whose availability baseline is platform-managed.
func (s *Server) restUpdateSkillState(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body builtinStateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := s.catalog.UpdateSkillState(id, body.DefaultEnabled, body.Locked); err != nil {
		slog.Error("update skill state", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.notifyCapabilitiesChanged("skill")
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "updated"})
}

func (s *Server) restDeleteSkillAdmin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.catalog.GetSkill(id)
	if err != nil || existing == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}
	if existing.Origin == "bundled" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "bundled skills are read-only"})
		return
	}
	if err := s.catalog.DeleteSkill(id, "", "platform"); err != nil {
		slog.Error("delete skill", "error", err)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}
	s.notifyCapabilitiesChanged("skill")
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "deleted"})
}

func (s *Server) restListMySkills(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromQuery(r)
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}
	recs, err := s.catalog.ListSkills(userID)
	if err != nil {
		slog.Error("list my skills", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	out := make([]nanoprotocol.SkillDef, 0, len(recs))
	for _, rec := range recs {
		out = append(out, skillDef(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": out, "count": len(out)})
}

func (s *Server) restCreateMySkill(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromQuery(r)
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}
	var body skillBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Name == "" || len(body.Files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and files are required"})
		return
	}
	if taken, err := s.catalog.SkillNameTaken(body.Name, userID, ""); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	} else if taken {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a skill with this name already exists"})
		return
	}
	rec := body.record("user", userID)
	rec.ContentHash = skillbundle.ContentHash(body.Files)
	if err := s.catalog.CreateSkill(rec); err != nil {
		slog.Error("create my skill", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.notifyCapabilitiesChanged("skill")
	writeJSON(w, http.StatusCreated, map[string]string{"id": rec.ID, "status": "created"})
}

func (s *Server) restUpdateMySkill(w http.ResponseWriter, r *http.Request) {
	userID, id := userIDFromQuery(r), r.PathValue("id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}
	rec, err := s.catalog.GetSkill(id)
	if err != nil || rec == nil || rec.Scope != "user" || rec.OwnerUserID != userID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}
	var body skillBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	upd := body.record("user", userID)
	upd.ID = id
	upd.ContentHash = skillbundle.ContentHash(body.Files)
	if taken, terr := s.catalog.SkillNameTaken(upd.Name, userID, id); terr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	} else if taken {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a skill with this name already exists"})
		return
	}
	upd.Origin = rec.Origin
	upd.Version = rec.Version
	upd.PublishedVersion = rec.PublishedVersion
	upd.CreatedBy = rec.CreatedBy
	if err := s.catalog.UpdateSkill(id, upd); err != nil {
		slog.Error("update my skill", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	s.notifyCapabilitiesChanged("skill")
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "updated"})
}

func (s *Server) restDeleteMySkill(w http.ResponseWriter, r *http.Request) {
	userID, id := userIDFromQuery(r), r.PathValue("id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}
	if err := s.catalog.DeleteSkill(id, userID, "user"); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found"})
		return
	}
	s.notifyCapabilitiesChanged("skill")
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "deleted"})
}
