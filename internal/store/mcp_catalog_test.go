package store

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func mcpRow(id, name, endpoint string) McpServerRecord {
	return McpServerRecord{
		Capability: Capability{
			ID: id, Scope: "platform", Name: name, DefaultEnabled: true,
		},
		Transport: "http",
		URL:       endpoint,
		Headers:   map[string]string{"Authorization": "Bearer x"},
	}
}

// The seeder runs on every start. It must add the shipped server once, and it
// must not add a second copy of an endpoint the deployment already configured by
// hand under another id: that is how one server ended up listed twice, under two
// tool prefixes, in every agent's catalogue.
func TestSeedMcpServerIsIdempotentAndMatchesByEndpoint(t *testing.T) {
	st := testStore(t)
	const endpoint = "http://library:8080/mcp"

	inserted, err := st.SeedMcpServer(mcpRow("mcp:first-party", "first-party", endpoint))
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if !inserted {
		t.Fatal("the first seed must insert")
	}

	inserted, err = st.SeedMcpServer(mcpRow("mcp:first-party", "first-party", endpoint))
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if inserted {
		t.Fatal("seeding twice inserted a second row")
	}

	// A hand-configured row at the same endpoint, under a different id and name.
	inserted, err = st.SeedMcpServer(mcpRow("mcp:platform-library", "library", endpoint))
	if err != nil {
		t.Fatalf("endpoint seed: %v", err)
	}
	if inserted {
		t.Fatal("an endpoint that is already configured must not be seeded again")
	}

	rows, err := st.ListMcpServers("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("catalogue has %d row(s), want 1: %+v", len(rows), rows)
	}
}

// A name is unique inside a namespace — the platform's, or one user's — not only
// inside "user". The old check looked at user rows alone, so a platform row could
// sit beside the seeded one under the same name.
func TestCreateMcpKeepsNamesUniquePerNamespace(t *testing.T) {
	st := testStore(t)

	if err := st.CreateMcp(mcpRow("mcp:one", "library", "http://one/mcp")); err != nil {
		t.Fatalf("first platform row: %v", err)
	}
	second := mcpRow("mcp:two", "library", "http://two/mcp")
	if err := st.CreateMcp(second); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want a name collision in the platform namespace", err)
	}

	userRow := mcpRow("mcp:mine", "library", "http://mine/mcp")
	userRow.Scope = "user"
	userRow.OwnerUserID = "u1"
	if err := st.CreateMcp(userRow); err != nil {
		t.Fatalf("a user may reuse a platform name: %v", err)
	}
	again := userRow
	again.ID = "mcp:mine-too"
	if err := st.CreateMcp(again); err == nil {
		t.Fatal("one user must not have two servers under the same name")
	}
	otherUser := userRow
	otherUser.ID = "mcp:theirs"
	otherUser.OwnerUserID = "u2"
	if err := st.CreateMcp(otherUser); err != nil {
		t.Fatalf("another user may reuse the name: %v", err)
	}
}

// The readable check is a courtesy; the migration's unique index is what holds
// when two writers race, so it has to be there.
func TestMcpNameIndexIsUnique(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	insert := func(id, name string) error {
		_, err := st.DB().Collection(mcpServersCollection).InsertOne(ctx, bson.D{
			{Key: "_id", Value: id}, {Key: "scope", Value: "platform"},
			{Key: "owner_user_id", Value: ""}, {Key: "name", Value: name},
		})
		return err
	}
	if err := insert("mcp:one", "library"); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	err := insert("mcp:two", "library")
	if err == nil {
		t.Fatal("a second row with the same namespace and name was accepted")
	}
	var dup mongo.WriteException
	if !asWriteException(err, &dup) || len(dup.WriteErrors) == 0 || dup.WriteErrors[0].Code != 11000 {
		t.Fatalf("err = %v, want a duplicate-key rejection", err)
	}
}

func asWriteException(err error, target *mongo.WriteException) bool {
	for err != nil {
		if we, ok := err.(mongo.WriteException); ok {
			*target = we
			return true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}
