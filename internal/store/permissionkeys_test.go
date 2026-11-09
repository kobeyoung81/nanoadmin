package store

import (
	"context"
	"testing"

	"gitee.com/kobeyoung81/nanoprotocol"

	"gitee.com/kobeyoung81/nanoadmin/internal/permissionkeys"
)

// Seeding inserts what is missing and never overwrites: descriptions and default
// actions are admin-editable, and a deploy must not undo those edits.
func TestSeedPermissionKeysIsIdempotentAndKeepsEdits(t *testing.T) {
	st := testStore(t)

	inserted, err := st.SeedPermissionKeys(permissionkeys.Keys())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if inserted != len(permissionkeys.Keys()) {
		t.Fatalf("inserted %d, want %d", inserted, len(permissionkeys.Keys()))
	}

	again, err := st.SeedPermissionKeys(permissionkeys.Keys())
	if err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	if again != 0 {
		t.Fatalf("re-seed inserted %d, want 0", again)
	}

	// An admin edit survives a re-seed.
	first := permissionkeys.Keys()[0]
	if _, err := st.DB().Collection(permissionKeysCollection).UpdateOne(context.Background(),
		map[string]any{"_id": first.ID},
		map[string]any{"$set": map[string]any{"description": "edited by an admin"}}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if _, err := st.SeedPermissionKeys(permissionkeys.Keys()); err != nil {
		t.Fatalf("seed after edit: %v", err)
	}

	keys, err := st.ListPermissionKeys()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != len(permissionkeys.Keys()) {
		t.Fatalf("listed %d keys, want %d", len(keys), len(permissionkeys.Keys()))
	}
	var edited bool
	for _, k := range keys {
		if k.ID == first.ID {
			edited = k.Description == "edited by an admin"
		}
	}
	if !edited {
		t.Fatal("seeding overwrote an admin edit")
	}
}

// The read-only keys are the strong claim the bus enforces; the listing has to
// carry the flag faithfully or a console would let an admin unlock them.
func TestListPermissionKeysKeepsTheReadOnlyFlag(t *testing.T) {
	st := testStore(t)
	if _, err := st.SeedPermissionKeys([]nanoprotocol.PermissionKeyDef{
		{ID: "bash", Description: "shell", DefaultAction: nanoprotocol.PermissionDeny, ReadOnly: false},
		{ID: "readonly_bash", Description: "read-only shell", DefaultAction: nanoprotocol.PermissionAllow, ReadOnly: true},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	keys, err := st.ListPermissionKeys()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 2 || keys[0].ID != "bash" || keys[1].ID != "readonly_bash" {
		t.Fatalf("keys = %+v, want ordered by id", keys)
	}
	if keys[1].ReadOnly != true {
		t.Fatal("read-only flag lost: the console would let an admin unlock it")
	}
}
