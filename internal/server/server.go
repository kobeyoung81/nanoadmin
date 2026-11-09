// Package server exposes nanoadmin's HTTP surface.
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"gitee.com/kobeyoung81/nanoadmin/internal/config"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
)

// ConfigStore is the slice of the store this service's HTTP layer needs. It is
// an interface so the handlers can be tested without a database.
type ConfigStore interface {
	GetConfigItems(componentType string) ([]store.ConfigItem, error)
	GetConfigItem(componentType, key string) (store.ConfigItem, bool, error)
	ListConfigComponents() ([]string, error)
	SetConfigItemTyped(componentType, key, value, valueType, description string) error
}

// Server is the HTTP surface.
type Server struct {
	cfg   config.Config
	store ConfigStore
	key   *InternalKey
}

// New builds a server.
func New(cfg config.Config, st ConfigStore) *Server {
	return &Server{cfg: cfg, store: st, key: NewInternalKey(cfg.InternalToken)}
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

	return s.key.Guard(mux, func(r *http.Request) bool {
		return strings.HasPrefix(r.URL.Path, "/api/v1/")
	})
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
	writeJSON(w, http.StatusOK, item)
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
