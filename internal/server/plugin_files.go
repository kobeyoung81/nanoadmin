package server

// This module's front end.
//
// It is a module-federation remote the portal's shell loads at runtime from
// /plugins/admin/. Two rules the plain file server gets wrong for a bundle: there
// is no index to serve (a directory request must not list the folder), and only the
// fingerprinted assets may be cached — the entry point and the manifest keep their
// names across releases, so a cached copy would pin an old build.

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
)

// registerUIRoutes serves the built front end from dir. An empty or missing
// directory is not an error: the module then contributes no page, and the console
// shows the shell's own.
func (s *Server) registerUIRoutes(mux *http.ServeMux, dir string) {
	if dir == "" {
		return
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		slog.Info("no admin front end in this image", "dir", dir)
		return
	}
	files := http.StripPrefix("/plugins/admin/", http.FileServer(http.Dir(dir)))
	mux.Handle("GET /plugins/admin/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	}))
	slog.Info("serving the admin front end", "dir", dir)
}
