package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitee.com/kobeyoung81/nanoadmin/internal/permissionkeys"
	"gitee.com/kobeyoung81/nanoprotocol"
)

// The bus forwards two per-(agent, mode) writes here: the scalar patch and the
// permission list. Its ownership and thinking-level checks happen before the
// call, so this face is asserted for what it does with a well-formed request and
// for refusing one that is not.

func patchMode(t *testing.T, s *Server, agentID, typeID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/internal/agents/"+agentID+"/modes/"+typeID, strings.NewReader(body))
	// The handler reads the pair from the route; a direct call has to set it.
	req.SetPathValue("agentID", agentID)
	req.SetPathValue("typeID", typeID)
	rec := httptest.NewRecorder()
	s.internalPatchModeOverride(rec, req)
	return rec
}

func postPermissions(t *testing.T, s *Server, agentID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/internal/agents/"+agentID+"/permissions", strings.NewReader(body))
	req.SetPathValue("agentID", agentID)
	rec := httptest.NewRecorder()
	s.internalSetModePermissions(rec, req)
	return rec
}

// TestInternalPatchModeOverride_MergesIntoTheStoredDocument: the bus sends only
// what the console changed, and the merge happens here, against the stored
// document.
func TestInternalPatchModeOverride_MergesIntoTheStoredDocument(t *testing.T) {
	s, st := testCatalogServer(t)

	rec := patchMode(t, s, "agent-1", "build", `{"thinking":"high"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	// A second patch changes one field; the first survives.
	if rec := patchMode(t, s, "agent-1", "build", `{"settings":{"shell_timeout":"60s"}}`); rec.Code != http.StatusOK {
		t.Fatalf("second patch: %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := patchMode(t, s, "agent-1", "build", `{"clear_model_id":true}`); rec.Code != http.StatusOK {
		t.Fatalf("third patch: %d (%s)", rec.Code, rec.Body.String())
	}

	ov, err := st.GetAgentTypeOverride("agent-1", "build")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if ov == nil || ov.Thinking != "high" || ov.Settings["shell_timeout"] != "60s" {
		t.Fatalf("the patches did not accumulate: %+v", ov)
	}
}

// TestInternalPatchModeOverride_RejectsAMalformedCall.
func TestInternalPatchModeOverride_RejectsAMalformedCall(t *testing.T) {
	s, _ := testCatalogServer(t)

	if rec := patchMode(t, s, "", "build", `{"thinking":"high"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty agent: status = %d, want 400", rec.Code)
	}
	if rec := patchMode(t, s, "agent-1", "build", `not json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad body: status = %d, want 400", rec.Code)
	}
}

// TestInternalSetModePermissions_ReplacesOrAppends: the console's form replaces
// the list, the runtime's message appends one rule, and both arrive at the same
// endpoint.
func TestInternalSetModePermissions_ReplacesOrAppends(t *testing.T) {
	s, st := testCatalogServer(t)
	// The rules name permissions, so the catalog has to exist (admin-init seeds
	// it before the service starts).
	if _, err := st.SeedPermissionKeys(permissionkeys.Keys()); err != nil {
		t.Fatalf("seed permission keys: %v", err)
	}

	rec := postPermissions(t, s, "agent-1", `{"type_id":"build","permissions":[{"pattern":"bash","action":"ask"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	rec = postPermissions(t, s, "agent-1", `{"type_id":"build","permissions":[{"pattern":"write","action":"allow"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("replace: %d (%s)", rec.Code, rec.Body.String())
	}
	ov, err := st.GetAgentTypeOverride("agent-1", "build")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(ov.Permissions) != 1 || ov.Permissions[0].Pattern != "write" {
		t.Fatalf("the list was not replaced: %+v", ov.Permissions)
	}

	rec = postPermissions(t, s, "agent-1", `{"type_id":"build","append":true,"permissions":[{"pattern":"edit","action":"allow"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("append: %d (%s)", rec.Code, rec.Body.String())
	}
	ov, _ = st.GetAgentTypeOverride("agent-1", "build")
	if len(ov.Permissions) != 2 {
		t.Fatalf("the append did not add a rule: %+v", ov.Permissions)
	}
}

// TestInternalSetModePermissions_RequiresTheMode: without it there is no document
// to write.
func TestInternalSetModePermissions_RequiresTheMode(t *testing.T) {
	s, _ := testCatalogServer(t)

	if rec := postPermissions(t, s, "agent-1", `{"permissions":[]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if rec := postPermissions(t, s, "", `{"type_id":"build","permissions":[]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty agent: status = %d, want 400", rec.Code)
	}
}

// TestInternalSetModePermissions_RefusesToRelaxAReadOnlyKey: the read-only catalog
// is enforced where the document is written, and the refusal is a client error so
// the console can show it.
func TestInternalSetModePermissions_RefusesToRelaxAReadOnlyKey(t *testing.T) {
	s, st := testCatalogServer(t)

	if _, err := st.DB().Collection("admin_permission_keys").InsertOne(t.Context(), map[string]any{
		"_id": "read", "default_action": nanoprotocol.PermissionAllow, "read_only": true,
	}); err != nil {
		t.Fatalf("seed key: %v", err)
	}

	rec := postPermissions(t, s, "agent-1", `{"type_id":"build","permissions":[{"pattern":"read","action":"deny"}]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
		t.Fatalf("the refusal was not explained: %s", rec.Body.String())
	}
}
