package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitee.com/kobeyoung81/nanoadmin/internal/config"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
)

type fakeStore struct {
	items      []store.ConfigItem
	components []string
	single     store.ConfigItem
	singleOK   bool
}

func (f fakeStore) GetConfigItems(string) ([]store.ConfigItem, error) { return f.items, nil }
func (f fakeStore) GetConfigItem(string, string) (store.ConfigItem, bool, error) {
	return f.single, f.singleOK, nil
}
func (f fakeStore) ListConfigComponents() ([]string, error) { return f.components, nil }

func (f fakeStore) SetConfigItemTyped(string, string, string, string, string) error { return nil }

func newTestServer(token string) http.Handler {
	cfg := config.Config{Addr: ":0", InternalToken: token}
	st := fakeStore{
		items:      []store.ConfigItem{{Key: "agent.language.default", Value: "zh"}},
		components: []string{"agent"},
		single:     store.ConfigItem{Key: "agent.language.default", Value: "zh"},
		singleOK:   true,
	}
	return New(cfg, st).Handler()
}

// The probe must not need the secret: a health check that only answers when the
// caller already has the credential cannot tell "up" from "misconfigured".
func TestHealthzIsOpen(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer("secret").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// Every /api/v1 route carries module data, so the guard is the only thing
// between the port and the database.
func TestInternalRoutesRequireTheToken(t *testing.T) {
	handler := newTestServer("secret")
	for _, path := range []string{"/api/v1/config", "/api/v1/config/agent", "/api/v1/config/agent/key"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a token = %d, want 401", path, rec.Code)
		}
	}
}

// An unset token is a deployment mistake, and the guard must fail closed rather
// than serve the domain to anyone who can reach the port.
func TestMissingConfiguredTokenFailsClosed(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer("").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/config", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestConfigReadsWithToken(t *testing.T) {
	handler := newTestServer("secret")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/agent", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"component_type":"agent"`) || !strings.Contains(body, "agent.language.default") {
		t.Fatalf("body = %s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/config/agent/agent.language.default", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"value":"zh"`) {
		t.Fatalf("single item: status = %d body = %s", rec.Code, rec.Body.String())
	}
}

// Writing is the whole reason the domain moved: the console's save must reach
// this service, and it must be refused without the token like any other read.
func TestConfigWriteRequiresTheTokenAndAcceptsTheBody(t *testing.T) {
	handler := newTestServer("secret")

	req := httptest.NewRequest(http.MethodPut, "/api/v1/config/agent/agent.language.default",
		strings.NewReader(`{"value":"en","value_type":"string"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated write = %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/v1/config/agent/agent.language.default",
		strings.NewReader(`{"value":"en","value_type":"string"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("write = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"value":"en"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/api/v1/config/agent/agent.language.default",
		strings.NewReader(`not json`))
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body = %d, want 400", rec.Code)
	}
}

func TestUnknownConfigItemIsNotFound(t *testing.T) {
	cfg := config.Config{InternalToken: "secret"}
	srv := New(cfg, fakeStore{}).Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/agent/nope", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
