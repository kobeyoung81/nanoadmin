package server

// The runtime resolution the bus used to compute itself. These tests pin the two
// things that would break an agent's start silently: the answer covers the mode the
// agent actually runs, and the caller's facts are honoured rather than re-derived
// from a row this module does not own.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gitee.com/kobeyoung81/nanoadmin/internal/store"
)

// testInternalToken is what these tests present on the internal face.
const testInternalToken = "test-token"

func resolve(t *testing.T, srv *httptest.Server, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testInternalToken)
	rec := httptest.NewRecorder()
	srv.Config.Handler.ServeHTTP(rec, req)
	return rec
}

func runtimeServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	s, st := testCatalogServer(t)
	seedSessionTypes(t, st)
	return httptest.NewServer(s.Handler()), st
}

// seedSessionTypes writes the two switchable modes the resolution is about: a fresh
// database has the collections but no types (the catalogue is seeded by
// `nanoadmin agent-types seed`, which a test database does not run).
func seedSessionTypes(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	for _, def := range []bson.D{
		{
			{Key: "_id", Value: "build"}, {Key: "name", Value: "Build"},
			{Key: "strategy", Value: "react"}, {Key: "context", Value: "windowed"},
			{Key: "output", Value: "stream"}, {Key: "session_binding", Value: "shared"},
			{Key: "prompt", Value: "build prompt"},
			{Key: "settings", Value: bson.D{{Key: "thinking", Value: "medium"}}},
			{Key: "capabilities", Value: bson.A{}},
			{Key: "permissions", Value: bson.A{}},
		},
		{
			{Key: "_id", Value: "plan"}, {Key: "name", Value: "Plan"},
			{Key: "strategy", Value: "react"}, {Key: "context", Value: "windowed"},
			{Key: "output", Value: "stream"}, {Key: "session_binding", Value: "shared"},
			{Key: "prompt", Value: "plan prompt"},
			{Key: "settings", Value: bson.D{{Key: "thinking", Value: "high"}}},
			{Key: "capabilities", Value: bson.A{}},
			{Key: "permissions", Value: bson.A{}},
		},
	} {
		if _, err := st.DB().Collection("admin_agent_types").InsertOne(ctx, def); err != nil {
			t.Fatalf("seed type: %v", err)
		}
	}
}

func TestResolveAgentRuntimeUsesTheCallersDefaultMode(t *testing.T) {
	srv, _ := runtimeServer(t)
	defer srv.Close()

	rec := resolve(t, srv, "/api/v1/internal/agent-runtime", map[string]any{
		"agent_id": "agent-1", "user_id": "user-1",
		"primary_type": "build", "session_types": []string{"build", "plan"},
		"default_type": "plan",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		DefaultModeTypeID string `json:"default_mode_type_id"`
		Capabilities      struct {
			BuiltinTools []map[string]any `json:"builtin_tools"`
			Models       []map[string]any `json:"models"`
		} `json:"capabilities"`
		Type struct {
			ID string `json:"id"`
		} `json:"type"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if out.DefaultModeTypeID != "plan" {
		t.Fatalf("the caller's default mode was ignored: %q", out.DefaultModeTypeID)
	}
	if out.Type.ID != "build" {
		t.Fatalf("the primary type should still be the agent's own: %q", out.Type.ID)
	}
}

func TestResolveAgentRuntimeFallsBackToThePrimaryType(t *testing.T) {
	srv, _ := runtimeServer(t)
	defer srv.Close()

	rec := resolve(t, srv, "/api/v1/internal/agent-runtime", map[string]any{
		"agent_id": "agent-1", "user_id": "user-1", "primary_type": "build",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		DefaultModeTypeID string `json:"default_mode_type_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.DefaultModeTypeID != "build" {
		t.Fatalf("expected the primary type as the fallback mode, got %q", out.DefaultModeTypeID)
	}
}

func TestResolveAgentRuntimeRequiresTheAgentAndOwner(t *testing.T) {
	srv, _ := runtimeServer(t)
	defer srv.Close()

	rec := resolve(t, srv, "/api/v1/internal/agent-runtime", map[string]any{"agent_id": "agent-1"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without user_id, got %d", rec.Code)
	}
}

func TestResolveModeCapabilitiesAnswersTheRequestedMode(t *testing.T) {
	srv, _ := runtimeServer(t)
	defer srv.Close()

	rec := resolve(t, srv, "/api/v1/internal/mode-capabilities", map[string]any{
		"agent_id": "agent-1", "user_id": "user-1", "type_id": "plan",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("resolve: %d %s", rec.Code, rec.Body.String())
	}
	var caps struct {
		AgentType struct {
			ID string `json:"id"`
		} `json:"agent_type"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &caps)
	if caps.AgentType.ID != "plan" {
		t.Fatalf("expected plan's type, got %q", caps.AgentType.ID)
	}
}
