package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gitee.com/kobeyoung81/nanoadmin/internal/config"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
	"gitee.com/kobeyoung81/nanoadmin/migrations"
)

// testCatalogServer builds a server over a throwaway database. The catalogue
// resolution under test is a query, so it needs the real driver; without
// NANOSEC_ADMIN_TEST_URI the test skips rather than pretend to pass.
func testCatalogServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	uri := os.Getenv("NANOSEC_ADMIN_TEST_URI")
	if uri == "" {
		t.Skip("NANOSEC_ADMIN_TEST_URI not set")
	}
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("parse uri: %v", err)
	}
	var b [6]byte
	_, _ = rand.Read(b[:])
	u.Path = "/nanoadmin_test_" + hex.EncodeToString(b[:])

	st, err := store.Open(u.String())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		_ = st.DB().Drop(context.Background())
		_ = st.Close()
	})
	manifests, err := migrations.Load()
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}
	if _, err := st.Migrate(manifests); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := New(config.Config{}, st).WithCatalog(st)
	return s, st
}

// TestRestEffectiveModels pins the two halves of the resolution: the type's
// default model applies when nothing overrides it, and a per-agent override wins
// when a type allows it.
func TestRestEffectiveModels(t *testing.T) {
	s, st := testCatalogServer(t)
	ctx := context.Background()

	// Two models and a type whose default is the first.
	for _, m := range []bson.D{
		{{Key: "_id", Value: int64(1)}, {Key: "provider_id", Value: int64(1)}, {Key: "scope", Value: "platform"}, {Key: "owner_user_id", Value: ""}, {Key: "name", Value: "basic"}, {Key: "model", Value: "basic"}},
		{{Key: "_id", Value: int64(2)}, {Key: "provider_id", Value: int64(1)}, {Key: "scope", Value: "platform"}, {Key: "owner_user_id", Value: ""}, {Key: "name", Value: "pro"}, {Key: "model", Value: "pro"}},
	} {
		if _, err := st.DB().Collection("admin_models").InsertOne(ctx, m); err != nil {
			t.Fatalf("seed model: %v", err)
		}
	}
	if _, err := st.DB().Collection("admin_agent_types").InsertOne(ctx, bson.D{
		{Key: "_id", Value: "build"}, {Key: "name", Value: "Build"}, {Key: "strategy", Value: "react"},
		{Key: "context", Value: "windowed"}, {Key: "output", Value: "stream"}, {Key: "session_binding", Value: "shared"},
		{Key: "models", Value: bson.A{
			bson.D{{Key: "model_id", Value: int64(1)}, {Key: "is_default", Value: true}},
			bson.D{{Key: "model_id", Value: int64(2)}},
		}},
	}); err != nil {
		t.Fatalf("seed type: %v", err)
	}
	// agent-overridden picked model 2; agent-default has no override.
	if _, err := st.DB().Collection("admin_agent_type_overrides").InsertOne(ctx, bson.D{
		{Key: "agent_id", Value: "agent-overridden"}, {Key: "type_id", Value: "build"},
		{Key: "model_id", Value: int64(2)}, {Key: "updated_at", Value: time.Now()},
	}); err != nil {
		t.Fatalf("seed override: %v", err)
	}

	body := `{"agents":[{"agent_id":"agent-default","type_id":"build"},{"agent_id":"agent-overridden","type_id":"build"}]}`
	rec := httptest.NewRecorder()
	s.restEffectiveModels(rec, httptest.NewRequest(http.MethodPost, "/api/v1/agent-types/effective-models", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Models map[string]struct {
			ModelID int64  `json:"model_id"`
			Name    string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := out.Models["agent-default"]; got.ModelID != 1 || got.Name != "basic" {
		t.Fatalf("agent-default = %+v, want the type default (1/basic)", got)
	}
	if got := out.Models["agent-overridden"]; got.ModelID != 2 || got.Name != "pro" {
		t.Fatalf("agent-overridden = %+v, want its override (2/pro)", got)
	}
}
