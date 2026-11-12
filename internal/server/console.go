package server

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"gitee.com/kobeyoung81/nanoadmin/internal/store"
)

// The console's own namespace.
//
// nginx routes /api/admin/v1/ to this service, so every route below is served by
// the module that owns its data. The portal keeps the routes whose data is hers
// (users, containers, the overview, conversations, runtime, the runner fleet);
// nothing is proxied between modules to make a page work.
//
// Two routes have a console-specific shape rather than the internal one:
//
//   - configuration is a component summary plus a key→value map (the internal
//     read is an item list), and secrets are masked — the caller is a browser;
//   - audit and alert rules are plain arrays (the internal read wraps them) and
//     the portal used to unwrap them.
func (s *Server) registerConsoleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/v1/configs", s.consoleListConfigs)
	mux.HandleFunc("GET /api/admin/v1/configs/{component}", s.consoleGetConfig)
	mux.HandleFunc("PUT /api/admin/v1/configs/{component}/{key}", s.restSetConfig)

	mux.HandleFunc("GET /api/admin/v1/audit", s.consoleListAudit)
	mux.HandleFunc("GET /api/admin/v1/alert-rules", s.consoleListAlertRules)
	mux.HandleFunc("POST /api/admin/v1/alert-rules", s.restUpsertAlertRule)
	mux.HandleFunc("DELETE /api/admin/v1/alert-rules/{id}", s.restDeleteAlertRule)

	mux.HandleFunc("GET /api/admin/v1/permission-keys", s.restListPermissionKeys)
	mux.HandleFunc("GET /api/admin/v1/prompt-blocks", s.restGetPromptBlocks)
	mux.HandleFunc("PUT /api/admin/v1/prompt-blocks", s.restSavePromptBlock)

	mux.HandleFunc("GET /api/admin/v1/agent-types", s.restListAgentTypes)
	mux.HandleFunc("POST /api/admin/v1/agent-types", s.restSaveAgentType)
	mux.HandleFunc("GET /api/admin/v1/agent-types/prompt-status", s.restAgentTypePromptStatus)
	mux.HandleFunc("POST /api/admin/v1/agent-types/effective-models", s.restEffectiveModels)
	mux.HandleFunc("GET /api/admin/v1/agent-types/{id}", s.restGetAgentType)
	mux.HandleFunc("PUT /api/admin/v1/agent-types/{id}", s.restSaveAgentType)
	mux.HandleFunc("PUT /api/admin/v1/agent-types/{id}/capabilities", s.restSetTypeCapability)
	mux.HandleFunc("DELETE /api/admin/v1/agent-types/{id}/capabilities", s.restDeleteTypeCapability)
	mux.HandleFunc("DELETE /api/admin/v1/agent-types/{id}", s.restDeleteAgentType)
	mux.HandleFunc("POST /api/admin/v1/agent-types/{id}/prompt/reset", s.restResetAgentTypePrompt)

	mux.HandleFunc("GET /api/admin/v1/models", s.restListModelsAdmin)
	mux.HandleFunc("POST /api/admin/v1/models/providers", s.restCreateProviderAdmin)
	mux.HandleFunc("POST /api/admin/v1/models/providers/test", s.restTestProviderAdmin)
	mux.HandleFunc("PUT /api/admin/v1/models/providers/{id}", s.restUpdateProviderAdmin)
	mux.HandleFunc("DELETE /api/admin/v1/models/providers/{id}", s.restDeleteProviderAdmin)
	mux.HandleFunc("POST /api/admin/v1/models/models", s.restCreateModelAdmin)
	mux.HandleFunc("POST /api/admin/v1/models/test", s.restTestModelAdmin)
	mux.HandleFunc("PUT /api/admin/v1/models/models/{id}", s.restUpdateModelAdmin)
	mux.HandleFunc("DELETE /api/admin/v1/models/models/{id}", s.restDeleteModelAdmin)
	mux.HandleFunc("POST /api/admin/v1/models/models/{id}/apply-to-agent-types", s.restApplyModelToBuiltInTypes)

	mux.HandleFunc("GET /api/admin/v1/capabilities/builtin", s.restListBuiltin)
	mux.HandleFunc("PUT /api/admin/v1/capabilities/builtin/{id}", s.restUpdateBuiltin)
	mux.HandleFunc("GET /api/admin/v1/capabilities/mcp", s.restListMcpAdmin)
	mux.HandleFunc("POST /api/admin/v1/capabilities/mcp", s.restCreateMcpAdmin)
	mux.HandleFunc("POST /api/admin/v1/capabilities/mcp/test", s.restTestMcpAdmin)
	mux.HandleFunc("PUT /api/admin/v1/capabilities/mcp/{id}", s.restUpdateMcpAdmin)
	mux.HandleFunc("PUT /api/admin/v1/capabilities/mcp/{id}/state", s.restUpdateMcpState)
	mux.HandleFunc("DELETE /api/admin/v1/capabilities/mcp/{id}", s.restDeleteMcpAdmin)
	mux.HandleFunc("GET /api/admin/v1/capabilities/skills", s.restListSkillsAdmin)
	mux.HandleFunc("POST /api/admin/v1/capabilities/skills", s.restCreateSkillAdmin)
	mux.HandleFunc("PUT /api/admin/v1/capabilities/skills/{id}", s.restUpdateSkillAdmin)
	mux.HandleFunc("PUT /api/admin/v1/capabilities/skills/{id}/state", s.restUpdateSkillState)
	mux.HandleFunc("DELETE /api/admin/v1/capabilities/skills/{id}", s.restDeleteSkillAdmin)
}

// consoleListConfigs answers the configuration index: which components have
// values, and when each was last written.
func (s *Server) consoleListConfigs(w http.ResponseWriter, r *http.Request) {
	components, err := s.store.ListConfigComponents()
	if err != nil {
		slog.Error("list config components", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	type summary struct {
		ComponentType string `json:"component_type"`
		UpdatedAt     string `json:"updated_at"`
		Source        string `json:"source"`
	}
	out := make([]summary, 0, len(components))
	for _, component := range components {
		updated := ""
		if at, err := s.store.ConfigUpdatedAt(component); err == nil && !at.IsZero() {
			updated = at.UTC().Format(time.RFC3339)
		}
		out = append(out, summary{ComponentType: component, UpdatedAt: updated, Source: "nanoadmin"})
	}
	writeJSON(w, http.StatusOK, out)
}

// consoleGetConfig returns one component as the console edits it: one map of
// key→value, with secrets masked (the caller is a browser).
func (s *Server) consoleGetConfig(w http.ResponseWriter, r *http.Request) {
	component := r.PathValue("component")
	items, err := s.store.GetConfigItems(component)
	if err != nil {
		slog.Error("get config items", "component", component, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if len(items) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "config not found"})
		return
	}
	config := make(map[string]any, len(items))
	for _, item := range maskItems(items) {
		config[item.Key] = item.Value
	}
	updated := ""
	if at, err := s.store.ConfigUpdatedAt(component); err == nil && !at.IsZero() {
		updated = at.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"component_type": component,
		"updated_at":     updated,
		"config":         config,
	})
}

// consoleListAudit answers the console's array shape; the filters are the same
// ones the internal route takes.
func (s *Server) consoleListAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	entries, err := s.catalog.ListAudit(store.AuditFilter{
		Limit:        limit,
		Action:       searchTerm(r, "action"),
		Actor:        searchTerm(r, "actor"),
		ResourceType: searchTerm(r, "resource_type"),
		Resource:     searchTerm(r, "resource"),
	})
	if err != nil {
		slog.Error("list audit", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if entries == nil {
		entries = []store.AuditEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// consoleListAlertRules answers the array shape the console renders.
func (s *Server) consoleListAlertRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.store.ListAlertRules()
	if err != nil {
		slog.Error("list alert rules", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if rules == nil {
		rules = []store.AlertRule{}
	}
	writeJSON(w, http.StatusOK, rules)
}
