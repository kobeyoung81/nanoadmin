package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"gitee.com/kobeyoung81/nanoadmin/internal/store"
	"gitee.com/kobeyoung81/nanoprotocol"
)

// ======== builtin tools ========

type builtinStateBody struct {
	DefaultEnabled *bool   `json:"default_enabled"`
	Locked         *string `json:"locked"`
}

func (s *Server) restListBuiltin(w http.ResponseWriter, r *http.Request) {
	tools, err := s.catalog.ListBuiltinTools()
	if err != nil {
		slog.Error("list builtin tools", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if tools == nil {
		tools = []nanoprotocol.BuiltinToolDef{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools, "count": len(tools)})
}

func (s *Server) restUpdateBuiltin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body builtinStateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := s.catalog.UpdateBuiltinState(id, body.DefaultEnabled, body.Locked); err != nil {
		if errors.Is(err, store.ErrCapabilityNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "builtin tool not found"})
			return
		}
		slog.Error("update builtin tool", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "updated"})
}

// restSyncBuiltinTools takes the catalogue an agent binary announces. The bus
// forwards the runtime's message here, so the runtime does not need to learn a new
// address and this collection keeps one writer.
func (s *Server) restSyncBuiltinTools(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tools []nanoprotocol.BuiltinToolDef `json:"tools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := s.catalog.SyncBuiltinTools(body.Tools); err != nil {
		slog.Error("sync builtin tools", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "synced", "count": len(body.Tools)})
}
