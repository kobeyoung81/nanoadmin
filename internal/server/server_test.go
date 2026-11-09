package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitee.com/kobeyoung81/nanoadmin/internal/config"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
)

type fakeStore struct {
	rules      []store.AlertRule
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

func (f fakeStore) ConfigUpdatedAt(string) (time.Time, error) {
	return time.Unix(1700000000, 0).UTC(), nil
}

func (f fakeStore) CountConfigItems(string, string, string, string) (int, error) {
	return len(f.items), nil
}

func (f fakeStore) ListAlertRules() ([]store.AlertRule, error)     { return f.rules, nil }
func (f fakeStore) UpsertAlertRule(store.AlertRule) (int64, error) { return 7, nil }
func (f fakeStore) DeleteAlertRule(int64) error                    { return nil }

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

// The console's overview needs the two summary endpoints; both are internal
// like every other config route.
func TestConfigSummaryEndpoints(t *testing.T) {
	handler := newTestServer("secret")

	for _, path := range []string{"/api/v1/config-updated-at/agent", "/api/v1/config-counts/agent?key_prefix=agent."} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a token = %d, want 401", path, rec.Code)
		}

		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer secret")
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s = %d: %s", path, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config-updated-at/agent", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `"updated_at":"2023-11-14T22:13:20Z"`) {
		t.Fatalf("updated-at body = %s", rec.Body.String())
	}
}

// Alert rules are pure CRUD: the module owns list, upsert and delete, and the
// console reads them through this surface.
func TestAlertRulesCRUD(t *testing.T) {
	handler := newTestServer("secret")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/alert-rules", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"rules"`) {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/alert-rules", strings.NewReader(`{"severity":"warning","message":"cpu high"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":7`) {
		t.Fatalf("upsert = %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/alert-rules", strings.NewReader(`{"message":"no severity"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing severity = %d, want 400", rec.Code)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/alert-rules/7", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d", rec.Code)
	}

	// And without the token nothing is served.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/alert-rules", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list = %d, want 401", rec.Code)
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
