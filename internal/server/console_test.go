package server

// The console routes in this service have their own shapes (a component summary
// and a key→value map, plain arrays for audit and alert rules) because the pages
// consume those directly. These tests pin the shapes and the masking, so a page
// break shows up here rather than as an empty table.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitee.com/kobeyoung81/nanoadmin/internal/config"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
)

func TestConsoleListConfigs_ReturnsSummaries(t *testing.T) {
	st := fakeStore{components: []string{"agent", "auth"}}
	s := New(config.Config{}, st)

	rec := httptest.NewRecorder()
	s.consoleListConfigs(rec, httptest.NewRequest(http.MethodGet, "/api/admin/v1/configs", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out []struct {
		ComponentType string `json:"component_type"`
		UpdatedAt     string `json:"updated_at"`
		Source        string `json:"source"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 || out[0].ComponentType != "agent" || out[1].ComponentType != "auth" {
		t.Fatalf("summaries = %+v", out)
	}
	if out[0].UpdatedAt == "" {
		t.Fatal("the summary carries no updated_at, so the page cannot show freshness")
	}
}

func TestConsoleGetConfig_MasksSecrets(t *testing.T) {
	st := fakeStore{items: []store.ConfigItem{
		{Key: "jwtPrivateKey", Value: "pem-secret", ValueType: "secret"},
		{Key: "jwksUrl", Value: "http://nanoauth/jwks", ValueType: "string"},
	}}
	s := New(config.Config{}, st)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/v1/configs/auth", nil)
	req.SetPathValue("component", "auth")
	rec := httptest.NewRecorder()
	s.consoleGetConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out struct {
		ComponentType string         `json:"component_type"`
		Config        map[string]any `json:"config"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ComponentType != "auth" {
		t.Fatalf("component_type = %q", out.ComponentType)
	}
	if out.Config["jwtPrivateKey"] != maskValue {
		t.Fatalf("a secret reached the console unmasked: %v", out.Config["jwtPrivateKey"])
	}
	if out.Config["jwksUrl"] != "http://nanoauth/jwks" {
		t.Fatalf("an ordinary value was masked: %v", out.Config["jwksUrl"])
	}
}

func TestConsoleGetConfig_UnknownComponentIs404(t *testing.T) {
	s := New(config.Config{}, fakeStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/v1/configs/nope", nil)
	req.SetPathValue("component", "nope")
	rec := httptest.NewRecorder()
	s.consoleGetConfig(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestConsoleListAlertRules_IsAnArray(t *testing.T) {
	st := fakeStore{rules: []store.AlertRule{{ID: 1, Severity: "warning", Message: "disk"}}}
	s := New(config.Config{}, st)

	rec := httptest.NewRecorder()
	s.consoleListAlertRules(rec, httptest.NewRequest(http.MethodGet, "/api/admin/v1/alert-rules", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out []store.AlertRule
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("the console shape is not an array: %v (%s)", err, rec.Body.String())
	}
	if len(out) != 1 || out[0].ID != 1 {
		t.Fatalf("rules = %+v", out)
	}
}
