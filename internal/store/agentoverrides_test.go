package store

// The per-(agent, mode) override is the user plane of the type resolution, and
// its document is this module's. These tests exist because a mock cannot see
// what matters here; what matters is that the override replaces what was stored
// rather than merging into it (the SQL wrote every column), and that the
// read-only key rule still refuses a policy that relaxes a locked permission.

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gitee.com/kobeyoung81/nanoprotocol"
)

// seedOnlyReadOnlyKey puts one read-only key in the catalog, for a test that
// asserts what the validation does with one specific locked tool.
func seedOnlyReadOnlyKey(t *testing.T, s *Store, id string, action nanoprotocol.PermissionAction) {
	t.Helper()
	if _, err := s.DB().Collection("admin_permission_keys").InsertOne(context.Background(), bson.D{
		{Key: "_id", Value: id},
		{Key: "default_action", Value: action},
		{Key: "read_only", Value: true},
	}); err != nil {
		t.Fatalf("seed permission key: %v", err)
	}
}

func TestSetAgentTypeOverride_RoundTrips(t *testing.T) {
	s := testStore(t)

	modelID := int64(7)
	want := nanoprotocol.AgentTypeOverride{
		ModelID:  modelID,
		Thinking: "high",
		Settings: map[string]string{"session_mode": "continuous"},
		Capabilities: []nanoprotocol.CapabilityAttachment{
			{Kind: "skill", Ref: "docs", Enabled: true},
		},
	}
	if err := s.SetAgentTypeOverride("agent-1", "plan", want); err != nil {
		t.Fatalf("SetAgentTypeOverride: %v", err)
	}

	got, err := s.GetAgentTypeOverride("agent-1", "plan")
	if err != nil {
		t.Fatalf("GetAgentTypeOverride: %v", err)
	}
	if got == nil {
		t.Fatal("the override was not stored")
	}
	if got.ModelID != modelID || got.Thinking != "high" {
		t.Fatalf("round trip lost fields: %+v", got)
	}
	if got.Settings["session_mode"] != "continuous" {
		t.Fatalf("settings lost: %+v", got.Settings)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0].Ref != "docs" || !got.Capabilities[0].Enabled {
		t.Fatalf("capabilities lost: %+v", got.Capabilities)
	}
}

func TestGetAgentTypeOverride_NoneIsNil(t *testing.T) {
	s := testStore(t)

	got, err := s.GetAgentTypeOverride("agent-1", "build")
	if err != nil {
		t.Fatalf("GetAgentTypeOverride: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

// TestSetAgentTypeOverride_ClearsWhatIsNotSent: the SQL's upsert took every value
// from the statement, so a later save with no settings removed the earlier ones.
// A merge would leave an override the user believed they had cleared.
func TestSetAgentTypeOverride_ClearsWhatIsNotSent(t *testing.T) {
	s := testStore(t)

	if err := s.SetAgentTypeOverride("agent-1", "build", nanoprotocol.AgentTypeOverride{
		ModelID:  7,
		Thinking: "high",
		Settings: map[string]string{"session_mode": "continuous"},
		Capabilities: []nanoprotocol.CapabilityAttachment{
			{Kind: "skill", Ref: "docs", Enabled: true},
		},
	}); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// A second save that carries none of that: everything goes.
	if err := s.SetAgentTypeOverride("agent-1", "build", nanoprotocol.AgentTypeOverride{}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	got, err := s.GetAgentTypeOverride("agent-1", "build")
	if err != nil {
		t.Fatalf("GetAgentTypeOverride: %v", err)
	}
	if got == nil {
		t.Fatal("the override disappeared entirely")
	}
	if got.ModelID != 0 || got.Thinking != "" || len(got.Settings) != 0 || len(got.Capabilities) != 0 {
		t.Fatalf("the second save merged instead of replacing: %+v", got)
	}
}

// TestSetAgentTypeOverride_IsScopedToThePair pins the SQL primary key: one
// agent's override for a mode is not another's, and one mode's is not another's.
func TestSetAgentTypeOverride_IsScopedToThePair(t *testing.T) {
	s := testStore(t)

	if err := s.SetAgentTypeOverride("agent-1", "build", nanoprotocol.AgentTypeOverride{Thinking: "high"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := s.SetAgentTypeOverride("agent-2", "build", nanoprotocol.AgentTypeOverride{Thinking: "low"}); err != nil {
		t.Fatalf("save: %v", err)
	}

	for agentID, want := range map[string]string{"agent-1": "high", "agent-2": "low"} {
		got, err := s.GetAgentTypeOverride(agentID, "build")
		if err != nil {
			t.Fatalf("GetAgentTypeOverride(%s): %v", agentID, err)
		}
		if got == nil || got.Thinking != want {
			t.Fatalf("%s: got %+v, want thinking %q", agentID, got, want)
		}
	}

	if err := s.SetAgentTypeOverride("agent-1", "plan", nanoprotocol.AgentTypeOverride{Thinking: "off"}); err != nil {
		t.Fatalf("save plan: %v", err)
	}
	if got, err := s.GetAgentTypeOverride("agent-1", "plan"); err != nil || got == nil || got.Thinking != "off" {
		t.Fatalf("the second mode's override: %+v (err %v)", got, err)
	}
	if got, err := s.GetAgentTypeOverride("agent-2", "plan"); err != nil || got != nil {
		t.Fatalf("another agent's mode returned an override: %+v (err %v)", got, err)
	}
}

func TestListAgentTypeOverrides_KeyedByType(t *testing.T) {
	s := testStore(t)

	for _, row := range []struct{ agent, typeID, thinking string }{
		{"agent-1", "build", "high"},
		{"agent-1", "plan", "low"},
		{"agent-2", "build", "medium"},
	} {
		if err := s.SetAgentTypeOverride(row.agent, row.typeID, nanoprotocol.AgentTypeOverride{Thinking: row.thinking}); err != nil {
			t.Fatalf("save %s/%s: %v", row.agent, row.typeID, err)
		}
	}

	overrides, err := s.ListAgentTypeOverrides("agent-1")
	if err != nil {
		t.Fatalf("ListAgentTypeOverrides: %v", err)
	}
	if len(overrides) != 2 {
		t.Fatalf("expected 2 overrides, got %+v", overrides)
	}
	if overrides["build"].Thinking != "high" || overrides["plan"].Thinking != "low" {
		t.Fatalf("unexpected overrides: %+v", overrides)
	}
}

func TestDeleteAgentTypeOverride(t *testing.T) {
	s := testStore(t)

	if err := s.SetAgentTypeOverride("agent-1", "build", nanoprotocol.AgentTypeOverride{Thinking: "high"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := s.DeleteAgentTypeOverride("agent-1", "build"); err != nil {
		t.Fatalf("DeleteAgentTypeOverride: %v", err)
	}

	if got, err := s.GetAgentTypeOverride("agent-1", "build"); err != nil || got != nil {
		t.Fatalf("the override survived: %+v (err %v)", got, err)
	}
	// Deleting it again is not an error, as with the SQL DELETE.
	if err := s.DeleteAgentTypeOverride("agent-1", "build"); err != nil {
		t.Fatalf("DeleteAgentTypeOverride on a missing override: %v", err)
	}
}

func TestSetAgentTypeOverride_RequiresBothIDs(t *testing.T) {
	s := testStore(t)

	if err := s.SetAgentTypeOverride("", "build", nanoprotocol.AgentTypeOverride{}); err == nil {
		t.Fatal("an empty agent id was accepted")
	}
	if err := s.SetAgentTypeOverride("agent-1", "", nanoprotocol.AgentTypeOverride{}); err == nil {
		t.Fatal("an empty type id was accepted")
	}
}

// TestSetAgentTypeOverride_RejectsReadOnlyOverride: the catalog's read-only keys
// must keep their default action however the rules are stacked.
func TestSetAgentTypeOverride_RejectsReadOnlyOverride(t *testing.T) {
	s := testStore(t)
	seedOnlyReadOnlyKey(t, s, "read", nanoprotocol.PermissionAllow)

	if err := s.SetAgentTypeOverride("agent-1", "build", nanoprotocol.AgentTypeOverride{
		Permissions: []nanoprotocol.PermissionRule{{Pattern: "read", Action: nanoprotocol.PermissionDeny}},
	}); err == nil {
		t.Fatal("expected read-only override to be rejected")
	}

	// A wildcard deny is refused too, unless the read-only key is re-allowed
	// after it.
	if err := s.SetAgentTypeOverride("agent-1", "build", nanoprotocol.AgentTypeOverride{
		Permissions: []nanoprotocol.PermissionRule{{Pattern: "*", Action: nanoprotocol.PermissionDeny}},
	}); err == nil {
		t.Fatal("expected wildcard deny to be rejected while the read-only key is not re-allowed")
	}
}

func TestSetAgentTypeOverride_AcceptsAllowlistAfterWildcardDeny(t *testing.T) {
	// A catalog with the one key the policy has to re-allow: with the shipped
	// catalog every other read-only tool would have to be re-allowed too.
	s := testStore(t)
	seedOnlyReadOnlyKey(t, s, "read", nanoprotocol.PermissionAllow)

	err := s.SetAgentTypeOverride("agent-1", "build", nanoprotocol.AgentTypeOverride{
		Permissions: []nanoprotocol.PermissionRule{
			{Pattern: "*", Action: nanoprotocol.PermissionDeny},
			{Pattern: "read", Action: nanoprotocol.PermissionAllow},
		},
	})
	if err != nil {
		t.Fatalf("SetAgentTypeOverride: %v", err)
	}

	got, err := s.GetAgentTypeOverride("agent-1", "build")
	if err != nil {
		t.Fatalf("GetAgentTypeOverride: %v", err)
	}
	if len(got.Permissions) != 2 {
		t.Fatalf("permissions lost: %+v", got.Permissions)
	}
	if got.Permissions[0].Pattern != "*" || got.Permissions[1].Action != nanoprotocol.PermissionAllow {
		t.Fatalf("permissions came back in a different shape: %+v", got.Permissions)
	}
}

// TestPatchAgentTypeOverride_AppliesOnlyWhatIsSent is the user-plane write: the
// console sends the fields it changed, and everything it did not send — the
// permission rules and the capability toggles included — must survive.
func TestPatchAgentTypeOverride_AppliesOnlyWhatIsSent(t *testing.T) {
	s := testStore(t)
	seedOnlyReadOnlyKey(t, s, "read", nanoprotocol.PermissionAllow)

	if err := s.SetAgentTypeOverride("agent-1", "build", nanoprotocol.AgentTypeOverride{
		ModelID:  3,
		Thinking: "high",
		Settings: map[string]string{"session_mode": "continuous"},
		Permissions: []nanoprotocol.PermissionRule{
			{Pattern: "read", Action: nanoprotocol.PermissionAllow},
		},
		Capabilities: []nanoprotocol.CapabilityAttachment{{Kind: "skill", Ref: "docs", Enabled: true}},
	}); err != nil {
		t.Fatalf("seed override: %v", err)
	}

	thinking := "low"
	if err := s.PatchAgentTypeOverride("agent-1", "build", ModeOverridePatch{Thinking: &thinking}); err != nil {
		t.Fatalf("patch thinking: %v", err)
	}
	got, err := s.GetAgentTypeOverride("agent-1", "build")
	if err != nil {
		t.Fatalf("GetAgentTypeOverride: %v", err)
	}
	if got.Thinking != "low" {
		t.Fatalf("thinking was not patched: %+v", got)
	}
	if got.ModelID != 3 || len(got.Permissions) != 1 || len(got.Capabilities) != 1 {
		t.Fatalf("the patch dropped a field it did not mention: %+v", got)
	}

	// Settings replace as a whole: the console sends the map it wants stored.
	settings := map[string]string{"shell_timeout": "60s"}
	if err := s.PatchAgentTypeOverride("agent-1", "build", ModeOverridePatch{Settings: &settings}); err != nil {
		t.Fatalf("patch settings: %v", err)
	}
	got, _ = s.GetAgentTypeOverride("agent-1", "build")
	if got.Settings["shell_timeout"] != "60s" || len(got.Settings) != 1 {
		t.Fatalf("settings did not replace: %+v", got.Settings)
	}

	// Clearing the model is its own flag: a zero id cannot mean "clear", because
	// absent and zero are different requests.
	if err := s.PatchAgentTypeOverride("agent-1", "build", ModeOverridePatch{ClearModelID: true}); err != nil {
		t.Fatalf("clear model: %v", err)
	}
	got, _ = s.GetAgentTypeOverride("agent-1", "build")
	if got.ModelID != 0 || got.Thinking != "low" {
		t.Fatalf("clearing the model disturbed another field: %+v", got)
	}
}

// TestPatchAgentTypeOverride_CreatesWhenNoneExists: the first write on a mode may
// be a patch, so there is nothing to read first.
func TestPatchAgentTypeOverride_CreatesWhenNoneExists(t *testing.T) {
	s := testStore(t)

	modelID := int64(5)
	if err := s.PatchAgentTypeOverride("agent-1", "plan", ModeOverridePatch{ModelID: &modelID}); err != nil {
		t.Fatalf("patch: %v", err)
	}
	got, err := s.GetAgentTypeOverride("agent-1", "plan")
	if err != nil {
		t.Fatalf("GetAgentTypeOverride: %v", err)
	}
	if got == nil || got.ModelID != 5 {
		t.Fatalf("the override was not created: %+v", got)
	}

	if err := s.PatchAgentTypeOverride("", "plan", ModeOverridePatch{}); err == nil {
		t.Fatal("an empty agent id was accepted")
	}
}

// TestSetModeCapability_Upserts: it asserts what the SQL version could not — that
// toggling the same capability replaces its entry instead of appending a second
// one.
func TestSetModeCapability_Upserts(t *testing.T) {
	s := testStore(t)

	if err := s.SetModeCapability("agent-1", "build", "mcp", "mcp:github", true); err != nil {
		t.Fatalf("SetModeCapability: %v", err)
	}
	got, err := s.GetAgentTypeOverride("agent-1", "build")
	if err != nil {
		t.Fatalf("GetAgentTypeOverride: %v", err)
	}
	if got == nil || len(got.Capabilities) != 1 {
		t.Fatalf("expected one capability override, got %+v", got)
	}
	if got.Capabilities[0].Kind != "mcp" || got.Capabilities[0].Ref != "mcp:github" || !got.Capabilities[0].Enabled {
		t.Fatalf("unexpected override: %+v", got.Capabilities[0])
	}

	// Toggling it off updates that entry rather than adding another.
	if err := s.SetModeCapability("agent-1", "build", "mcp", "mcp:github", false); err != nil {
		t.Fatalf("SetModeCapability: %v", err)
	}
	got, err = s.GetAgentTypeOverride("agent-1", "build")
	if err != nil {
		t.Fatalf("GetAgentTypeOverride: %v", err)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0].Enabled {
		t.Fatalf("the capability was appended instead of updated: %+v", got.Capabilities)
	}
}

// TestSetModePermissions_ReplacesTheList is what the console's form does: the
// rules it shows are the rules that get stored.
func TestSetModePermissions_ReplacesTheList(t *testing.T) {
	s := testStore(t)

	if err := s.SetModePermissions("agent-1", "build", []nanoprotocol.PermissionRule{
		{Pattern: "bash", Action: nanoprotocol.PermissionAsk},
	}); err != nil {
		t.Fatalf("SetModePermissions: %v", err)
	}
	if err := s.SetModePermissions("agent-1", "build", []nanoprotocol.PermissionRule{
		{Pattern: "write", Action: nanoprotocol.PermissionAllow},
	}); err != nil {
		t.Fatalf("SetModePermissions: %v", err)
	}

	got, err := s.GetAgentTypeOverride("agent-1", "build")
	if err != nil {
		t.Fatalf("GetAgentTypeOverride: %v", err)
	}
	if len(got.Permissions) != 1 || got.Permissions[0].Pattern != "write" {
		t.Fatalf("the list was merged instead of replaced: %+v", got.Permissions)
	}

	// The runtime's message appends one rule at a time, replacing an existing
	// rule with the same pattern.
	if err := s.AppendModePermission("agent-1", "build", nanoprotocol.PermissionRule{Pattern: "write", Action: nanoprotocol.PermissionDeny}); err != nil {
		t.Fatalf("AppendModePermission: %v", err)
	}
	if err := s.AppendModePermission("agent-1", "build", nanoprotocol.PermissionRule{Pattern: "edit", Action: nanoprotocol.PermissionAllow}); err != nil {
		t.Fatalf("AppendModePermission: %v", err)
	}
	got, _ = s.GetAgentTypeOverride("agent-1", "build")
	if len(got.Permissions) != 2 {
		t.Fatalf("append produced %d rules: %+v", len(got.Permissions), got.Permissions)
	}
	if got.Permissions[0].Pattern != "write" || got.Permissions[0].Action != nanoprotocol.PermissionDeny {
		t.Fatalf("the existing rule was not replaced: %+v", got.Permissions[0])
	}
}
