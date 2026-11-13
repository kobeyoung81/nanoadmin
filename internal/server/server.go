// Package server exposes nanoadmin's HTTP surface.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"gitee.com/kobeyoung81/nanoadmin/internal/auth"
	"gitee.com/kobeyoung81/nanoadmin/internal/busclient"
	"gitee.com/kobeyoung81/nanoadmin/internal/config"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
	"gitee.com/kobeyoung81/nanoprotocol"
)

// ConfigStore is the slice of the store this service's HTTP layer needs. It is
// an interface so the handlers can be tested without a database.
type ConfigStore interface {
	GetConfigItems(componentType string) ([]store.ConfigItem, error)
	GetConfigItem(componentType, key string) (store.ConfigItem, bool, error)
	ListConfigComponents() ([]string, error)
	SetConfigItemTyped(componentType, key, value, valueType, description string) error
	ConfigUpdatedAt(componentType string) (time.Time, error)
	CountConfigItems(componentType, keyPrefix, keySuffix, value string) (int, error)
	ListAlertRules() ([]store.AlertRule, error)
	UpsertAlertRule(rule store.AlertRule) (int64, error)
	DeleteAlertRule(id int64) error
	ListPermissionKeys() ([]nanoprotocol.PermissionKeyDef, error)
}

// Server is the HTTP surface.
type Server struct {
	cfg   config.Config
	store ConfigStore
	// catalog is the same store, typed concretely: the catalogue handlers need
	// methods the small ConfigStore interface does not carry.
	catalog *store.Store
	key     *InternalKey
	// jwt verifies browser tokens. It is nil when no JWKS URL is configured, in
	// which case only the internal token is accepted.
	jwt *auth.Validator
	// bus carries the few notifications the catalogue writes need (the agent
	// supervisors listen to the bus's SSE stream, not to this service).
	bus *busclient.Client
}

// New builds a server.
func New(cfg config.Config, st ConfigStore) *Server {
	return &Server{
		cfg:   cfg,
		store: st,
		key:   NewInternalKey(cfg.InternalToken),
		jwt:   auth.NewValidator(cfg.JWKSURL),
	}
}

// WithCatalog attaches the concrete store the catalogue handlers need. Tests
// that only exercise configuration routes leave it nil.
func (s *Server) WithCatalog(st *store.Store) *Server {
	s.catalog = st
	return s
}

// WithBus attaches the bus client used for capability-change broadcasts.
func (s *Server) WithBus(bus *busclient.Client) *Server {
	s.bus = bus
	return s
}

// notifyCapabilitiesChanged asks the bus to broadcast. It never fails a write:
// the catalogue is already saved, and the next agent start re-pulls it anyway.
func (s *Server) notifyCapabilitiesChanged(kind string) {
	if s.bus == nil || !s.bus.Enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.bus.CapabilitiesChanged(ctx, kind); err != nil {
		slog.Warn("broadcast capability change", "kind", kind, "error", err)
	}
}

// Handler returns the mux with the internal-key guard applied.
//
// Everything under /api/v1 is internal: the browser never talks to this service
// directly, the portal's server proxies the admin console's calls. /healthz is
// deliberately outside the guard so a probe does not need the secret.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/v1/config", s.restListConfigComponents)
	mux.HandleFunc("GET /api/v1/config/{component}", s.restGetConfig)
	mux.HandleFunc("GET /api/v1/config/{component}/{key}", s.restGetConfigItem)
	mux.HandleFunc("PUT /api/v1/config/{component}/{key}", s.restSetConfig)
	// User-scoped catalogue: a user's own models/providers and the assignable
	// agent types. Ownership is the portal's check; it passes user_id explicitly.
	mux.HandleFunc("GET /api/v1/user/agent-types", s.restListMyAgentTypes)
	mux.HandleFunc("GET /api/v1/user/models", s.restListMyModels)
	mux.HandleFunc("POST /api/v1/user/models/providers", s.restCreateMyProvider)
	mux.HandleFunc("POST /api/v1/user/models/providers/test", s.restTestProviderMy)
	mux.HandleFunc("PUT /api/v1/user/models/providers/{id}", s.restUpdateMyProvider)
	mux.HandleFunc("DELETE /api/v1/user/models/providers/{id}", s.restDeleteMyProvider)
	mux.HandleFunc("POST /api/v1/user/models/models", s.restCreateMyModel)
	mux.HandleFunc("POST /api/v1/user/models/test", s.restTestModelMy)
	mux.HandleFunc("PUT /api/v1/user/models/models/{id}", s.restUpdateMyModel)
	mux.HandleFunc("DELETE /api/v1/user/models/models/{id}", s.restDeleteMyModel)

	// Catalogue: agent types, their capability bindings and prompts.
	mux.HandleFunc("GET /api/v1/agent-types", s.restListAgentTypes)
	mux.HandleFunc("POST /api/v1/agent-types", s.restSaveAgentType)
	mux.HandleFunc("GET /api/v1/agent-types/prompt-status", s.restAgentTypePromptStatus)
	// Batch model resolution for the portal's agent rows: the catalogue lives
	// here, the agents there, and a per-row round trip would not scale.
	mux.HandleFunc("POST /api/v1/agent-types/effective-models", s.restEffectiveModels)
	mux.HandleFunc("GET /api/v1/agent-types/{id}", s.restGetAgentType)
	mux.HandleFunc("PUT /api/v1/agent-types/{id}", s.restSaveAgentType)
	mux.HandleFunc("PUT /api/v1/agent-types/{id}/capabilities", s.restSetTypeCapability)
	mux.HandleFunc("DELETE /api/v1/agent-types/{id}/capabilities", s.restDeleteTypeCapability)
	mux.HandleFunc("DELETE /api/v1/agent-types/{id}", s.restDeleteAgentType)
	mux.HandleFunc("POST /api/v1/agent-types/{id}/prompt/reset", s.restResetAgentTypePrompt)
	mux.HandleFunc("GET /api/v1/prompt-blocks", s.restGetPromptBlocks)
	mux.HandleFunc("PUT /api/v1/prompt-blocks", s.restSavePromptBlock)

	// Model catalogue and providers.
	mux.HandleFunc("GET /api/v1/models", s.restListModelsAdmin)
	mux.HandleFunc("POST /api/v1/models/providers", s.restCreateProviderAdmin)
	mux.HandleFunc("POST /api/v1/models/providers/test", s.restTestProviderAdmin)
	mux.HandleFunc("PUT /api/v1/models/providers/{id}", s.restUpdateProviderAdmin)
	mux.HandleFunc("DELETE /api/v1/models/providers/{id}", s.restDeleteProviderAdmin)
	mux.HandleFunc("POST /api/v1/models/models", s.restCreateModelAdmin)
	mux.HandleFunc("POST /api/v1/models/test", s.restTestModelAdmin)
	mux.HandleFunc("PUT /api/v1/models/models/{id}", s.restUpdateModelAdmin)
	mux.HandleFunc("DELETE /api/v1/models/models/{id}", s.restDeleteModelAdmin)
	mux.HandleFunc("POST /api/v1/models/models/{id}/apply-to-agent-types", s.restApplyModelToBuiltInTypes)

	// Builtin-tool catalogue: the console edits the platform default; the agent
	// runtime announces definitions through the bus, which forwards here.
	mux.HandleFunc("GET /api/v1/capabilities/builtin", s.restListBuiltin)
	mux.HandleFunc("PUT /api/v1/capabilities/builtin/{id}", s.restUpdateBuiltin)
	mux.HandleFunc("POST /api/v1/internal/builtin-tools/sync", s.restSyncBuiltinTools)

	// The skill catalogue: the console manages definitions, users manage their own,
	// and the agent runtime's save/delete/ack arrive through the internal face
	// (the bus forwards them).
	mux.HandleFunc("GET /api/v1/capabilities/mcp", s.restListMcpAdmin)
	mux.HandleFunc("POST /api/v1/capabilities/mcp", s.restCreateMcpAdmin)
	mux.HandleFunc("POST /api/v1/capabilities/mcp/test", s.restTestMcpAdmin)
	mux.HandleFunc("PUT /api/v1/capabilities/mcp/{id}", s.restUpdateMcpAdmin)
	mux.HandleFunc("PUT /api/v1/capabilities/mcp/{id}/state", s.restUpdateMcpState)
	mux.HandleFunc("DELETE /api/v1/capabilities/mcp/{id}", s.restDeleteMcpAdmin)
	mux.HandleFunc("GET /api/v1/user/capabilities/library/mcp", s.restListMyMcp)
	mux.HandleFunc("POST /api/v1/user/capabilities/library/mcp", s.restCreateMyMcp)
	mux.HandleFunc("POST /api/v1/user/capabilities/library/mcp/test", s.restTestMcpMy)
	mux.HandleFunc("PUT /api/v1/user/capabilities/library/mcp/{id}", s.restUpdateMyMcp)
	mux.HandleFunc("DELETE /api/v1/user/capabilities/library/mcp/{id}", s.restDeleteMyMcp)

	mux.HandleFunc("GET /api/v1/capabilities/skills", s.restListSkillsAdmin)
	mux.HandleFunc("POST /api/v1/capabilities/skills", s.restCreateSkillAdmin)
	mux.HandleFunc("PUT /api/v1/capabilities/skills/{id}", s.restUpdateSkillAdmin)
	mux.HandleFunc("PUT /api/v1/capabilities/skills/{id}/state", s.restUpdateSkillState)
	mux.HandleFunc("DELETE /api/v1/capabilities/skills/{id}", s.restDeleteSkillAdmin)
	mux.HandleFunc("GET /api/v1/user/capabilities/library/skills", s.restListMySkills)
	mux.HandleFunc("POST /api/v1/user/capabilities/library/skills", s.restCreateMySkill)
	mux.HandleFunc("PUT /api/v1/user/capabilities/library/skills/{id}", s.restUpdateMySkill)
	mux.HandleFunc("DELETE /api/v1/user/capabilities/library/skills/{id}", s.restDeleteMySkill)
	mux.HandleFunc("POST /api/v1/internal/skills/save", s.internalSkillSave)
	mux.HandleFunc("POST /api/v1/internal/skills/delete", s.internalSkillDelete)
	mux.HandleFunc("POST /api/v1/internal/skills/ack", s.internalSkillAck)
	mux.HandleFunc("POST /api/v1/internal/agents/{agentID}/capabilities", s.internalSetAgentCapability)
	mux.HandleFunc("POST /api/v1/internal/agents/{agentID}/capabilities/delete", s.internalDeleteAgentCapability)

	// Per-(agent, mode) writes: the override document is this module's, and the
	// bus forwards once it has decided what the caller meant (owner, mode).
	mux.HandleFunc("PUT /api/v1/internal/agents/{agentID}/modes/{typeID}", s.internalPatchModeOverride)
	mux.HandleFunc("POST /api/v1/internal/agents/{agentID}/permissions", s.internalSetModePermissions)

	mux.HandleFunc("GET /api/v1/permission-keys", s.restListPermissionKeys)
	mux.HandleFunc("GET /api/v1/alert-rules", s.restListAlertRules)
	mux.HandleFunc("POST /api/v1/alert-rules", s.restUpsertAlertRule)
	mux.HandleFunc("DELETE /api/v1/alert-rules/{id}", s.restDeleteAlertRule)
	mux.HandleFunc("GET /api/v1/config-updated-at/{component}", s.restConfigUpdatedAt)
	mux.HandleFunc("GET /api/v1/config-counts/{component}", s.restCountConfigItems)

	// The audit trail. The console reads it here; every module appends through
	// the internal face (the bus and the portal both post an entry).
	mux.HandleFunc("GET /api/v1/audit", s.restListAudit)
	mux.HandleFunc("POST /api/v1/internal/audit", s.internalRecordAudit)

	// The console's namespace, under both the historical prefix (nginx routes
	// /api/admin/v1/ here) and the target-state one (/api/v1/admin/, one
	// namespace per module). Same handlers, same shapes.
	s.registerConsoleRoutes(mux, "/api/admin/v1")
	s.registerConsoleRoutes(mux, "/api/v1/admin")

	return s.authenticate(mux)
}

func (s *Server) restListConfigComponents(w http.ResponseWriter, r *http.Request) {
	components, err := s.store.ListConfigComponents()
	if err != nil {
		slog.Error("list config components", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"components": components})
}

func (s *Server) restGetConfig(w http.ResponseWriter, r *http.Request) {
	component := r.PathValue("component")
	items, err := s.store.GetConfigItems(component)
	if err != nil {
		slog.Error("get config items", "component", component, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if items == nil {
		items = []store.ConfigItem{}
	}
	if wantsMaskedValues(r) {
		items = maskItems(items)
	}
	writeJSON(w, http.StatusOK, map[string]any{"component_type": component, "items": items})
}

func (s *Server) restGetConfigItem(w http.ResponseWriter, r *http.Request) {
	component := r.PathValue("component")
	key := r.PathValue("key")
	item, ok, err := s.store.GetConfigItem(component, key)
	if err != nil {
		slog.Error("get config item", "component", component, "key", key, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if wantsMaskedValues(r) {
		item = maskItems([]store.ConfigItem{item})[0]
	}
	writeJSON(w, http.StatusOK, item)
}

// restListPermissionKeys returns the catalog the console renders next to a
// policy editor, and which the bus reads to validate a policy's read-only keys.
func (s *Server) restListPermissionKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.store.ListPermissionKeys()
	if err != nil {
		slog.Error("list permission keys", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if keys == nil {
		keys = []nanoprotocol.PermissionKeyDef{}
	}
	writeJSON(w, http.StatusOK, nanoprotocol.PermissionKeysResponse{Keys: keys})
}

func (s *Server) restListAlertRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.store.ListAlertRules()
	if err != nil {
		slog.Error("list alert rules", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if rules == nil {
		rules = []store.AlertRule{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (s *Server) restUpsertAlertRule(w http.ResponseWriter, r *http.Request) {
	var rule store.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if rule.Severity == "" || rule.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "severity and message are required"})
		return
	}

	id, err := s.store.UpsertAlertRule(rule)
	if err != nil {
		slog.Error("upsert alert rule", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	// The id comes back so the caller that just created a rule can name it.
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "id": id})
}

func (s *Server) restDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid rule id"})
		return
	}
	if err := s.store.DeleteAlertRule(id); err != nil {
		slog.Error("delete alert rule", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// restConfigUpdatedAt answers "when did this component last change", which the
// console shows per component. An empty updated_at means "no items", not an
// error: the page renders a dash.
func (s *Server) restConfigUpdatedAt(w http.ResponseWriter, r *http.Request) {
	component := r.PathValue("component")
	updated, err := s.store.ConfigUpdatedAt(component)
	if err != nil {
		slog.Error("config updated at", "component", component, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	body := map[string]any{"component_type": component, "updated_at": ""}
	if !updated.IsZero() {
		body["updated_at"] = updated.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, body)
}

// restCountConfigItems answers the overview's count query without downloading
// the component.
func (s *Server) restCountConfigItems(w http.ResponseWriter, r *http.Request) {
	component := r.PathValue("component")
	query := r.URL.Query()
	count, err := s.store.CountConfigItems(component,
		query.Get("key_prefix"), query.Get("key_suffix"), query.Get("value"))
	if err != nil {
		slog.Error("count config items", "component", component, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"component_type": component, "count": count})
}

func (s *Server) restSetConfig(w http.ResponseWriter, r *http.Request) {
	component := r.PathValue("component")
	key := r.PathValue("key")

	var body struct {
		Value string `json:"value"`
		// Optional, and used only when the item is created: a component that
		// owns a key it generates itself (nanoauth's JWT keypair) declares its
		// kind, and `secret` is what makes the console mask it.
		ValueType   string `json:"value_type,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	// A form that read a masked secret sends the mask back untouched; writing it
	// would replace a live key with three asterisks.
	if body.Value == maskValue {
		writeJSON(w, http.StatusOK, map[string]any{
			"component_type": component,
			"key":            key,
			"value":          maskValue,
			"unchanged":      true,
		})
		return
	}
	if err := s.store.SetConfigItemTyped(component, key, body.Value, body.ValueType, body.Description); err != nil {
		slog.Error("set config item", "component", component, "key", key, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"component_type": component,
		"key":            key,
		"value":          body.Value,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
