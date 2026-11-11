package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// postAudit records one entry through the internal face, the way the bus and the
// portal do.
func postAudit(t *testing.T, s *Server, entry string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.internalRecordAudit(rec, httptest.NewRequest(http.MethodPost, "/api/v1/internal/audit", strings.NewReader(entry)))
	return rec
}

// listAudit reads the trail through the console's face.
func listAudit(t *testing.T, s *Server, query string) []auditRow {
	t.Helper()
	rec := httptest.NewRecorder()
	s.restListAudit(rec, httptest.NewRequest(http.MethodGet, "/api/v1/audit"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list audit: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Entries []auditRow `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Entries
}

type auditRow struct {
	ActorID      string         `json:"actor_id"`
	ActorName    string         `json:"actor_name"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	ResourceName string         `json:"resource_name"`
	Detail       map[string]any `json:"detail"`
	IP           string         `json:"ip"`
}

// TestInternalRecordAuditAndList: a module posts who did what, this service
// stamps the time and the id, and the console reads the entry back with the
// actor's display name resolved.
func TestInternalRecordAuditAndList(t *testing.T) {
	s, st := testCatalogServer(t)
	ctx := context.Background()
	if _, err := st.DB().Collection("auth_users").InsertOne(ctx, bson.D{
		{Key: "_id", Value: "u-1"}, {Key: "display_name", Value: "Ada"},
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	rec := postAudit(t, s, `{"actor_id":"u-1","actor_role":"admin","action":"agent.delete",
		"resource_type":"agent","resource_id":"agent-1","detail":{"why":"test"},"ip":"10.0.0.1"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	entries := listAudit(t, s, "?limit=10")
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %+v", entries)
	}
	got := entries[0]
	if got.ActorID != "u-1" || got.Action != "agent.delete" || got.ResourceType != "agent" || got.IP != "10.0.0.1" {
		t.Fatalf("unexpected entry: %+v", got)
	}
	if got.ActorName != "Ada" {
		t.Fatalf("the actor's name was not resolved: %+v", got)
	}
	if got.Detail["why"] != "test" {
		t.Fatalf("detail lost: %+v", got.Detail)
	}
}

// TestInternalRecordAudit_RequiresTheActionAndResource: a trail entry without
// those two would be unreadable, and the console could not tell it apart from a
// mistake.
func TestInternalRecordAudit_RequiresTheActionAndResource(t *testing.T) {
	s, _ := testCatalogServer(t)

	if rec := postAudit(t, s, `{"actor_id":"u-1"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if rec := postAudit(t, s, `not json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a malformed body", rec.Code)
	}
	// Each of the two is required on its own: an entry with an action but no
	// resource type cannot be filtered or read.
	if rec := postAudit(t, s, `{"action":"config.update"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a missing resource_type", rec.Code)
	}

	// An anonymous actor is recorded as the system.
	if rec := postAudit(t, s, `{"action":"config.update","resource_type":"config"}`); rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	entries := listAudit(t, s, "")
	if len(entries) != 1 || entries[0].ActorID != "system" {
		t.Fatalf("anonymous entry = %+v, want actor_id system", entries)
	}
}

// TestRestListAudit_FiltersByOneConditionAtATime: the console offers an action
// dropdown, an actor box, a resource-type dropdown and a resource box. A name has
// to reach the row through the id it names — and only for the type that name
// belongs to.
func TestRestListAudit_FiltersByOneConditionAtATime(t *testing.T) {
	s, st := testCatalogServer(t)
	ctx := context.Background()

	seed := []struct{ collection, id, field, value string }{
		{"auth_users", "u-1", "display_name", "Ada"},
		{"auth_users", "u-2", "display_name", "Bob"},
		{"portal_agents", "agent-1", "display_name", "ops-bot"},
	}
	for _, row := range seed {
		if _, err := st.DB().Collection(row.collection).InsertOne(ctx, bson.D{
			{Key: "_id", Value: row.id}, {Key: row.field, Value: row.value},
		}); err != nil {
			t.Fatalf("seed %s: %v", row.collection, err)
		}
	}

	for _, entry := range []string{
		`{"actor_id":"u-1","actor_role":"admin","action":"agent.delete","resource_type":"agent","resource_id":"agent-1"}`,
		`{"actor_id":"u-2","actor_role":"operator","action":"user.update","resource_type":"user","resource_id":"u-1"}`,
	} {
		if rec := postAudit(t, s, entry); rec.Code != http.StatusCreated {
			t.Fatalf("seed audit entry: %d %s", rec.Code, rec.Body.String())
		}
	}

	if got := listAudit(t, s, "?actor=ada"); len(got) != 1 || got[0].ActorID != "u-1" || got[0].ActorName != "Ada" {
		t.Fatalf("actor name filter: %+v", got)
	}
	if got := listAudit(t, s, "?action=user.update"); len(got) != 1 || got[0].ResourceID != "u-1" {
		t.Fatalf("action filter: %+v", got)
	}
	if got := listAudit(t, s, "?resource_type=agent"); len(got) != 1 || got[0].ActorID != "u-1" {
		t.Fatalf("resource type filter: %+v", got)
	}
	// The resource box takes a name, and reaches the row through the id it names.
	if got := listAudit(t, s, "?resource=ops-bot"); len(got) != 1 || got[0].ResourceID != "agent-1" || got[0].ResourceName != "ops-bot" {
		t.Fatalf("resource name filter: %+v", got)
	}
	// A name that names nothing must not widen the result set.
	if got := listAudit(t, s, "?resource=nobody"); len(got) != 0 {
		t.Fatalf("unknown resource name matched %+v", got)
	}
}
