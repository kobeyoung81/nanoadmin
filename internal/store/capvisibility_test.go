package store

// The two small reads the bus needs from the catalogue's owner.
//
// Both used to be the bus's own: it looked up the model an agent's default mode
// runs with, and checked whether a catalogue item was visible to a user, straight
// out of this module's collections. The collections are this module's, so the
// answers are here and these tests pin the rules — an override names the model when
// it does, a locked default wins, and a user-scope item belongs to its owner.

import (
	"context"
	"testing"

	"gitee.com/kobeyoung81/nanoprotocol"
)

// seedTypeWithModels writes one type whose model bindings are the given
// (model id, default) pairs.
func seedTypeWithModels(t *testing.T, s *Store, id string, bindings ...nanoprotocol.TypeModelBinding) {
	t.Helper()
	if err := s.SaveAgentType(nanoprotocol.AgentTypeDef{
		ID: id, Name: id, Models: bindings,
	}); err != nil {
		t.Fatalf("seed type %s: %v", id, err)
	}
}

func TestDefaultModeModel_PrefersTheOverride(t *testing.T) {
	s := testStore(t)
	seedTypeWithModels(t, s, "build",
		nanoprotocol.TypeModelBinding{ModelID: 1, IsDefault: true},
		nanoprotocol.TypeModelBinding{ModelID: 2},
	)
	s.putModelForTest(t, 1, "basic")
	s.putModelForTest(t, 2, "pro")

	// No override: the type's default binding decides.
	if id, name := s.DefaultModeModel("agent-1", "build"); id != 1 || name != "basic" {
		t.Fatalf("default = %d/%q, want 1/basic", id, name)
	}

	// The override names a model the type allows: it wins.
	if err := s.SetAgentTypeOverride("agent-1", "build", nanoprotocol.AgentTypeOverride{ModelID: 2}); err != nil {
		t.Fatalf("set override: %v", err)
	}
	if id, name := s.DefaultModeModel("agent-1", "build"); id != 2 || name != "pro" {
		t.Fatalf("override = %d/%q, want 2/pro", id, name)
	}
}

// A locked default is the platform's decision: an override that names another
// model of the same type must not take it away.
func TestDefaultModeModel_ALockedDefaultWins(t *testing.T) {
	s := testStore(t)
	seedTypeWithModels(t, s, "build",
		nanoprotocol.TypeModelBinding{ModelID: 1, IsDefault: true, Locked: true},
		nanoprotocol.TypeModelBinding{ModelID: 2},
	)
	s.putModelForTest(t, 1, "basic")
	if err := s.SetAgentTypeOverride("agent-1", "build", nanoprotocol.AgentTypeOverride{ModelID: 2}); err != nil {
		t.Fatalf("set override: %v", err)
	}

	if id, _ := s.DefaultModeModel("agent-1", "build"); id != 1 {
		t.Fatalf("model = %d, want the locked default 1", id)
	}
}

func TestCapabilityVisible_PlatformAndUserScope(t *testing.T) {
	s := testStore(t)
	for _, rec := range []McpServerRecord{
		{Capability: Capability{ID: "mcp-platform", Scope: "platform", Name: "shared"}},
		{Capability: Capability{ID: "mcp-mine", Scope: "user", OwnerUserID: "user-1", Name: "mine"}},
	} {
		if err := s.CreateMcp(rec); err != nil {
			t.Fatalf("seed mcp %s: %v", rec.ID, err)
		}
	}

	cases := []struct {
		name  string
		kind  string
		ref   string
		user  string
		allow bool
	}{
		{"platform item is everyone's", "mcp", "mcp-platform", "user-2", true},
		{"own item is visible", "mcp", "mcp-mine", "user-1", true},
		{"someone else's item is not", "mcp", "mcp-mine", "user-2", false},
		{"unknown item is not visible", "mcp", "nope", "user-1", false},
		{"unknown kind is not visible", "widget", "mcp-platform", "user-1", false},
	}
	for _, tc := range cases {
		got, err := s.CapabilityVisible(tc.kind, tc.ref, tc.user)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.allow {
			t.Errorf("%s: visible = %v, want %v", tc.name, got, tc.allow)
		}
	}
}

// TestCapabilityVisible_BuiltinToolExists pins the third kind: a built-in tool is
// visible when the catalogue lists it.
func TestCapabilityVisible_BuiltinToolExists(t *testing.T) {
	s := testStore(t)
	if err := s.SyncBuiltinTools([]nanoprotocol.BuiltinToolDef{{ID: "bash", Description: "run commands"}}); err != nil {
		t.Fatalf("seed builtin: %v", err)
	}
	if ok, err := s.CapabilityVisible("builtin_tool", "bash", "user-1"); err != nil || !ok {
		t.Fatalf("bash visible = %v (err %v), want true", ok, err)
	}
	if ok, err := s.CapabilityVisible("builtin_tool", "nope", "user-1"); err != nil || ok {
		t.Fatalf("unknown tool visible = %v (err %v), want false", ok, err)
	}
}

// putModelForTest writes one model row so a name can be resolved.
func (s *Store) putModelForTest(t *testing.T, id int64, name string) {
	t.Helper()
	if _, err := s.DB().Collection(modelsCollection).InsertOne(context.Background(), map[string]any{
		"_id": id, "provider_id": 1, "scope": "platform", "owner_user_id": "",
		"name": name, "model": name, "context_window": 128000, "max_output_tokens": 4096,
	}); err != nil {
		t.Fatalf("seed model %d: %v", id, err)
	}
}
