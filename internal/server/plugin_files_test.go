package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitee.com/kobeyoung81/nanoadmin/internal/config"
)

// registerUIRoutes serves the module's front end: files are served, fingerprinted
// assets are cached hard, the entry point and the manifest are not, and a directory
// request must not become a file listing.
func TestPluginFilesServesOnlyFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"plugin.json":       `{"plugin":"admin"}`,
		"remoteEntry.js":    "export {}",
		"assets/index-1.js": "console.log(1)",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s := New(config.Config{}, fakeStore{})
	mux := http.NewServeMux()
	s.registerUIRoutes(mux, dir)

	cases := []struct {
		path       string
		status     int
		cacheMatch string
	}{
		{"/plugins/admin/plugin.json", http.StatusOK, "no-cache"},
		{"/plugins/admin/remoteEntry.js", http.StatusOK, "no-cache"},
		{"/plugins/admin/assets/index-1.js", http.StatusOK, "immutable"},
		{"/plugins/admin/", http.StatusNotFound, ""},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.status {
			t.Errorf("%s: status = %d, want %d", tc.path, rec.Code, tc.status)
			continue
		}
		if tc.cacheMatch != "" && !strings.Contains(rec.Header().Get("Cache-Control"), tc.cacheMatch) {
			t.Errorf("%s: Cache-Control = %q, want %q", tc.path, rec.Header().Get("Cache-Control"), tc.cacheMatch)
		}
	}
}

// An image without a front end still runs: the routes are simply absent.
func TestPluginFilesAbsentIsNotAnError(t *testing.T) {
	s := New(config.Config{}, fakeStore{})
	mux := http.NewServeMux()
	s.registerUIRoutes(mux, filepath.Join(t.TempDir(), "missing"))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins/admin/plugin.json", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
