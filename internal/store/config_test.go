package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gitee.com/kobeyoung81/nanoadmin/migrations"
)

// These tests need a real MongoDB: the behaviour under test (the unique index
// rejecting a second row, the migration records being scoped by service) is the
// server's, not Go's. Without NANO_ADMIN_TEST_URI they skip rather than pretend
// to pass.
func testStore(t *testing.T) *Store {
	t.Helper()
	uri := os.Getenv("NANO_ADMIN_TEST_URI")
	if uri == "" {
		t.Skip("NANO_ADMIN_TEST_URI not set")
	}
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("parse uri: %v", err)
	}
	var b [6]byte
	_, _ = rand.Read(b[:])
	u.Path = "/nanoadmin_test_" + hex.EncodeToString(b[:])

	st, err := Open(u.String())
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
	return st
}

func TestConfigItemsRoundTrip(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	coll := st.DB().Collection("admin_config")
	docs := []any{
		bson.D{{Key: "component_type", Value: "agent"}, {Key: "config_key", Value: "agent.b"}, {Key: "config_value", Value: "2"}},
		bson.D{{Key: "component_type", Value: "agent"}, {Key: "config_key", Value: "agent.a"}, {Key: "config_value", Value: "1"}},
		bson.D{{Key: "component_type", Value: "voice"}, {Key: "config_key", Value: "voice.enabled"}, {Key: "config_value", Value: "true"}},
	}
	for _, doc := range docs {
		if _, err := coll.InsertOne(ctx, doc); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	items, err := st.GetConfigItems("agent")
	if err != nil {
		t.Fatalf("GetConfigItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	// Sorted by key: that is the order the console renders and the order the
	// (component_type, config_key) index serves.
	if items[0].Key != "agent.a" || items[1].Key != "agent.b" {
		t.Fatalf("order = %s, %s", items[0].Key, items[1].Key)
	}

	item, ok, err := st.GetConfigItem("agent", "agent.b")
	if err != nil || !ok {
		t.Fatalf("GetConfigItem: ok=%v err=%v", ok, err)
	}
	if item.Value != "2" {
		t.Fatalf("value = %q, want 2", item.Value)
	}

	if _, ok, err := st.GetConfigItem("agent", "missing"); err != nil || ok {
		t.Fatalf("missing item: ok=%v err=%v", ok, err)
	}

	components, err := st.ListConfigComponents()
	if err != nil {
		t.Fatalf("ListConfigComponents: %v", err)
	}
	if len(components) != 2 {
		t.Fatalf("components = %v, want 2", components)
	}
}

// One collection, one writer: the unique index is what makes a double insert a
// loud error rather than two rows that disagree.
func TestConfigKeyIsUniquePerComponent(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	coll := st.DB().Collection("admin_config")

	doc := bson.D{
		{Key: "component_type", Value: "agent"},
		{Key: "config_key", Value: "agent.language.default"},
		{Key: "config_value", Value: "zh"},
	}
	if _, err := coll.InsertOne(ctx, doc); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := coll.InsertOne(ctx, doc); err == nil {
		t.Fatal("duplicate (component_type, config_key) was accepted")
	}
}

// The shared sys_migrations collection keys records by service; a record
// without the prefix would be invisible to this service on the next start.
func TestMigrationRecordsAreScopedToAdmin(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	var record bson.M
	if err := st.DB().Collection("sys_migrations").FindOne(ctx,
		bson.D{{Key: "service", Value: "admin"}}).Decode(&record); err != nil {
		t.Fatalf("read migration record: %v", err)
	}
	id, _ := record["_id"].(string)
	if !strings.HasPrefix(id, "admin:") {
		t.Fatalf("record id = %q, want an admin: prefix", id)
	}
}

// A value edit must not reclassify an item: making a `secret` ordinary would
// unmask it in every console that trusts value_type.
func TestSetConfigItemKeepsTheInsertedTypeAndDescription(t *testing.T) {
	st := testStore(t)

	if err := st.SetConfigItemTyped("auth", "jwtPrivateKey", "pem-1", "secret", "JWT private key"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := st.SetConfigItemTyped("auth", "jwtPrivateKey", "pem-2", "", ""); err != nil {
		t.Fatalf("second write: %v", err)
	}

	item, ok, err := st.GetConfigItem("auth", "jwtPrivateKey")
	if err != nil || !ok {
		t.Fatalf("read back: ok=%v err=%v", ok, err)
	}
	if item.Value != "pem-2" {
		t.Errorf("value = %q, want pem-2", item.Value)
	}
	if item.ValueType != "secret" {
		t.Errorf("value_type = %q, want secret (an edit must not reclassify)", item.ValueType)
	}
	if item.Description != "JWT private key" {
		t.Errorf("description = %q, want the inserted one", item.Description)
	}
}

// Seeding runs on every start, so an operator's edited value has to survive it.
func TestSeedDoesNotOverwriteExistingValues(t *testing.T) {
	st := testStore(t)

	defaults := []ConfigItem{{Key: "agent.language.default", Value: "zh", ValueType: "string", Description: "Language"}}
	if err := st.SeedConfigItems("agent", defaults); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := st.SetConfigItemTyped("agent", "agent.language.default", "en", "", ""); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if err := st.SeedConfigItems("agent", defaults); err != nil {
		t.Fatalf("re-seed: %v", err)
	}

	item, _, err := st.GetConfigItem("agent", "agent.language.default")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if item.Value != "en" {
		t.Fatalf("value = %q, want the edited en (seeding must not overwrite)", item.Value)
	}
}
