package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"gitee.com/kobeyoung81/nanoadmin/internal/mcpprobe"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
	"gitee.com/kobeyoung81/nanoprotocol"
)

type mcpTestBody struct {
	ID        string            `json:"id"`
	Transport string            `json:"transport"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
}

func mcpVisible(rec *store.McpServerRecord, owner string) bool {
	if rec.Scope == "platform" {
		return true
	}
	return owner != "" && rec.OwnerUserID == owner
}

// mcpDef maps a row to the wire type.
func mcpDef(rec store.McpServerRecord) nanoprotocol.McpServerDef {
	return nanoprotocol.McpServerDef{
		ID: rec.ID, Scope: rec.Scope, OwnerUserID: rec.OwnerUserID, Name: rec.Name,
		Description: rec.Description, Transport: rec.Transport, URL: rec.URL,
		Headers: rec.Headers, DefaultEnabled: rec.DefaultEnabled, Locked: rec.Locked,
	}
}

// maskMcp hides the values of an MCP server's headers: the console shows that a
// header exists, never the credential it carries.
func maskMcp(d nanoprotocol.McpServerDef) nanoprotocol.McpServerDef {
	m := d
	if len(m.Headers) > 0 {
		headers := make(map[string]string, len(m.Headers))
		for k := range m.Headers {
			headers[k] = maskValue
		}
		m.Headers = headers
	}
	return m
}

// preserveMaskedHeaders replaces incoming masked placeholders with the stored
// secret, so editing a server whose headers were masked on read does not
// overwrite the real value.
func preserveMaskedHeaders(incoming, stored map[string]string) {
	for k, v := range incoming {
		if v != maskValue {
			continue
		}
		if old, ok := stored[k]; ok {
			incoming[k] = old
		} else {
			delete(incoming, k)
		}
	}
}

// The MCP server catalogue's HTTP face: the console manages platform and user
// servers, users manage their own, and the connectivity test runs here (it is a
// probe, not a write).

type mcpBody struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Transport      string            `json:"transport"`
	URL            string            `json:"url,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	DefaultEnabled bool              `json:"default_enabled"`
	Locked         string            `json:"locked,omitempty"`
}

func (b mcpBody) record(scope, owner string) store.McpServerRecord {
	id := b.ID
	if id == "" {
		id = capabilitySlug("mcp", b.Name) + "-" + randomSuffix()
	}
	return store.McpServerRecord{
		Capability: store.Capability{
			ID:             id,
			Scope:          scope,
			OwnerUserID:    owner,
			Name:           b.Name,
			Description:    b.Description,
			DefaultEnabled: b.DefaultEnabled,
			Locked:         b.Locked,
		},
		Transport: b.Transport,
		URL:       b.URL,
		Headers:   b.Headers,
	}
}

func (s *Server) restListMcpAdmin(w http.ResponseWriter, r *http.Request) {
	recs, err := s.catalog.ListMcpServers("")
	if err != nil {
		slog.Error("list mcp servers", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	out := make([]nanoprotocol.McpServerDef, 0, len(recs))
	for _, rec := range recs {
		out = append(out, mcpDef(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": out, "count": len(out)})
}

func (s *Server) restCreateMcpAdmin(w http.ResponseWriter, r *http.Request) {
	var body mcpBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Name == "" || body.Transport == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and transport are required"})
		return
	}
	rec := body.record("platform", "")
	if err := s.catalog.CreateMcp(rec); err != nil {
		slog.Error("create mcp server", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.notifyCapabilitiesChanged("mcp")
	writeJSON(w, http.StatusCreated, map[string]string{"id": rec.ID, "status": "created"})
}

// testMcp probes an MCP server (by id, or an unsaved draft) and always responds
// 200 with {ok:false,error} on failure. owner=="" is the admin/global scope.
func (s *Server) testMcp(w http.ResponseWriter, r *http.Request, owner string) {
	var body mcpTestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	transport, rawURL := body.Transport, body.URL
	headers := map[string]string{}
	if body.ID != "" {
		rec, err := s.catalog.GetMcpServer(body.ID)
		if err != nil || rec == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "mcp server not found"})
			return
		}
		if !mcpVisible(rec, owner) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "mcp server not owned"})
			return
		}
		transport, rawURL = rec.Transport, rec.URL
		for k, v := range rec.Headers {
			headers[k] = v
		}
	}
	if body.Transport != "" {
		transport = body.Transport
	}
	if body.URL != "" {
		rawURL = body.URL
	}
	for k, v := range body.Headers {
		if v != "" && v != maskValue {
			headers[k] = v
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), testProbeTimeout)
	defer cancel()
	start := time.Now()
	result, err := mcpprobe.Probe(ctx, transport, rawURL, headers)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "latency_ms": latency, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"latency_ms":       latency,
		"tools":            result.Tools,
		"server_name":      result.ServerName,
		"protocol_version": result.ProtocolVersion,
	})
}

func (s *Server) restTestMcpAdmin(w http.ResponseWriter, r *http.Request) {
	s.testMcp(w, r, "")
}

func (s *Server) restTestMcpMy(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromQuery(r)
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}
	s.testMcp(w, r, userID)
}

func (s *Server) restUpdateMcpAdmin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body mcpBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	rec := body.record("platform", "")
	rec.ID = id
	if existing, err := s.catalog.GetMcpServer(id); err == nil && existing != nil {
		preserveMaskedHeaders(rec.Headers, existing.Headers)
	}
	if err := s.catalog.UpdateMcp(id, rec); err != nil {
		slog.Error("update mcp server", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	s.notifyCapabilitiesChanged("mcp")
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "updated"})
}

// restUpdateMcpState updates only the definition-plane default (default_enabled /
// locked) for an MCP server; used by the Capability Center's inline controls.
func (s *Server) restUpdateMcpState(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body builtinStateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := s.catalog.UpdateMcpState(id, body.DefaultEnabled, body.Locked); err != nil {
		slog.Error("update mcp state", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.notifyCapabilitiesChanged("mcp")
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "updated"})
}

func (s *Server) restDeleteMcpAdmin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.catalog.DeleteMcp(id, "", "platform"); err != nil {
		slog.Error("delete mcp server", "error", err)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "mcp server not found"})
		return
	}
	s.notifyCapabilitiesChanged("mcp")
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "deleted"})
}

func (s *Server) restListMyMcp(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromQuery(r)
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}
	recs, err := s.catalog.ListMcpServers(userID)
	if err != nil {
		slog.Error("list my mcp servers", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	out := make([]nanoprotocol.McpServerDef, 0, len(recs))
	for _, rec := range recs {
		def := maskMcp(mcpDef(rec))
		out = append(out, def)
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": out, "count": len(out)})
}

func (s *Server) restCreateMyMcp(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromQuery(r)
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}
	var body mcpBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Name == "" || body.Transport == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and transport are required"})
		return
	}
	rec := body.record("user", userID)
	if err := s.catalog.CreateMcp(rec); err != nil {
		slog.Error("create my mcp server", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.notifyCapabilitiesChanged("mcp")
	writeJSON(w, http.StatusCreated, map[string]string{"id": rec.ID, "status": "created"})
}

func (s *Server) restUpdateMyMcp(w http.ResponseWriter, r *http.Request) {
	userID, id := userIDFromQuery(r), r.PathValue("id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}
	rec, err := s.catalog.GetMcpServer(id)
	if err != nil || rec == nil || rec.Scope != "user" || rec.OwnerUserID != userID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "mcp server not found"})
		return
	}
	var body mcpBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	upd := body.record("user", userID)
	upd.ID = id
	preserveMaskedHeaders(upd.Headers, rec.Headers)
	if err := s.catalog.UpdateMcp(id, upd); err != nil {
		slog.Error("update my mcp server", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	s.notifyCapabilitiesChanged("mcp")
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "updated"})
}

func (s *Server) restDeleteMyMcp(w http.ResponseWriter, r *http.Request) {
	userID, id := userIDFromQuery(r), r.PathValue("id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}
	if err := s.catalog.DeleteMcp(id, userID, "user"); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "mcp server not found"})
		return
	}
	s.notifyCapabilitiesChanged("mcp")
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "deleted"})
}
