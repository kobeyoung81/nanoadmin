package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"gitee.com/kobeyoung81/nanoadmin/internal/agenttypes"
	"gitee.com/kobeyoung81/nanoprotocol"
)

// The builtin-tool catalogue: the definitions an agent binary announces, plus the
// platform-wide default and lock an administrator sets for each. The agent
// runtime announces them (sync_builtin_tools, forwarded by the bus); the console
// edits the state. Both writes are here, which is what makes this collection
// admin's.
const builtinToolsCollection = "admin_builtin_tools"

// ErrCapabilityNotFound reports an update against a catalogue row that is not
// there.
var ErrCapabilityNotFound = errors.New("capability not found")

// storedBuiltinTool is a tool definition document.
type storedBuiltinTool struct {
	ID          string `bson:"_id"`
	Description string `bson:"description,omitempty"`
	// A JSON blob on both sides of the wire (the protocol carries it as
	// json.RawMessage) and never queried, so it stays the text the SQL column
	// held rather than becoming a document nothing reads field by field.
	Parameters     string    `bson:"parameters,omitempty"`
	DefaultEnabled bool      `bson:"default_enabled"`
	Locked         string    `bson:"locked,omitempty"`
	UpdatedAt      time.Time `bson:"updated_at"`
}

// ListBuiltinTools returns the catalogue ordered by id.
func (s *Store) ListBuiltinTools() ([]nanoprotocol.BuiltinToolDef, error) {
	ctx, cancel := opCtx()
	defer cancel()

	cur, err := s.db.Collection(builtinToolsCollection).Find(ctx, bson.D{},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list builtin tools: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	out := []nanoprotocol.BuiltinToolDef{}
	for cur.Next(ctx) {
		var doc storedBuiltinTool
		if err := cur.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode builtin tool: %w", err)
		}
		def := nanoprotocol.BuiltinToolDef{
			ID: doc.ID, Description: doc.Description,
			DefaultEnabled: doc.DefaultEnabled, Locked: doc.Locked,
		}
		// An unusable parameters blob is left empty, as it was when the SQL read
		// failed to unmarshal it.
		var raw json.RawMessage
		if json.Unmarshal([]byte(doc.Parameters), &raw) == nil {
			def.Parameters = raw
		}
		out = append(out, def)
	}
	return out, cur.Err()
}

// UpdateBuiltinState updates the definition-plane default (default_enabled /
// locked) for a builtin tool. It is the platform-wide default applied to every
// agent type that has no explicit baseline. Descriptions and parameters are owned
// by the agent binary.
func (s *Store) UpdateBuiltinState(id string, defaultEnabled *bool, locked *string) error {
	ctx, cancel := opCtx()
	defer cancel()

	// Only the fields the caller named are written; the SQL read the row first and
	// merged in Go, which is the same outcome with an extra round trip.
	set := bson.D{{Key: "updated_at", Value: time.Now().UTC()}}
	if defaultEnabled != nil {
		set = append(set, bson.E{Key: "default_enabled", Value: *defaultEnabled})
	}
	if locked != nil {
		set = append(set, bson.E{Key: "locked", Value: *locked})
	}

	res, err := s.db.Collection(builtinToolsCollection).UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}}, bson.D{{Key: "$set", Value: set}})
	if err != nil {
		return fmt.Errorf("update builtin state: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrCapabilityNotFound
	}
	return nil
}

// SyncBuiltinTools upserts runtime definitions from the agent binary. Admin state
// (default_enabled / locked) is never overwritten: the declared default applies
// only when the row is created, so a tool the binary has just grown arrives
// switched off and every type that should have it says so explicitly. That is what
// makes a new builtin tool opt-in rather than silently available.
func (s *Store) SyncBuiltinTools(tools []nanoprotocol.BuiltinToolDef) error {
	ctx, cancel := opCtx()
	defer cancel()

	ids := make([]string, 0, len(tools))
	for _, t := range tools {
		params, _ := json.Marshal(t.Parameters)
		update := bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "description", Value: t.Description},
				{Key: "parameters", Value: string(params)},
			}},
			{Key: "$setOnInsert", Value: bson.D{
				{Key: "default_enabled", Value: t.DefaultEnabled},
				{Key: "locked", Value: ""},
			}},
		}
		err := retryOnDuplicateUpsert(func() error {
			_, err := s.db.Collection(builtinToolsCollection).UpdateOne(ctx,
				bson.D{{Key: "_id", Value: t.ID}}, update, options.UpdateOne().SetUpsert(true))
			return err
		})
		if err != nil {
			return fmt.Errorf("sync builtin tool %q: %w", t.ID, err)
		}
		// Rows inserted before the binary announced a default carry the zero value
		// and no updated_at (sync never writes it; only an administrator's
		// UpdateBuiltinState does). Backfill the declared default there so a
		// catalog created by an older binary starts on the intended footing, while
		// every decision an administrator made stays untouched.
		if err := s.fillLegacyBuiltinDefault(ctx, t.ID, t.DefaultEnabled); err != nil {
			return err
		}
		ids = append(ids, t.ID)
	}
	// A new tool is opt-in, so the roles that decide about every catalog entry
	// have to decide about it too.
	return s.completeRestrictedBelts(ctx, "builtin_tool", ids)
}

// fillLegacyBuiltinDefault gives a row inserted before tools declared defaults the
// default the binary now declares. The guard is updated_at: synchronization never
// writes it and UpdateBuiltinState (the administrator's switch) always does, so an
// absent or zero timestamp is the mark of a row nobody has decided about yet.
func (s *Store) fillLegacyBuiltinDefault(ctx context.Context, id string, declared bool) error {
	_, err := s.db.Collection(builtinToolsCollection).UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}, {Key: "$or", Value: bson.A{
			bson.D{{Key: "updated_at", Value: bson.D{{Key: "$exists", Value: false}}}},
			bson.D{{Key: "updated_at", Value: nil}},
			bson.D{{Key: "updated_at", Value: time.Time{}}},
		}}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "default_enabled", Value: declared}}}},
	)
	if err != nil {
		return fmt.Errorf("backfill builtin tool %q default: %w", id, err)
	}
	return nil
}

// completeRestrictedBelts writes an explicit decision for every catalog entry a
// restricted role has not decided about: for those roles a missing row is not
// "inherit the catalog default", it is an unfinished decision. Rows that are
// already there are never touched, so this converges on the first sync.
func (s *Store) completeRestrictedBelts(ctx context.Context, kind string, refs []string) error {
	if len(refs) == 0 {
		return nil
	}
	for _, belt := range agenttypes.CompleteBelts() {
		for _, ref := range refs {
			undecided := bson.D{{Key: "_id", Value: belt.TypeID}, {Key: "capabilities", Value: bson.D{
				{Key: "$not", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
					{Key: "kind", Value: kind}, {Key: "ref", Value: ref},
				}}}},
			}}}
			row := storedTypeCapability{
				Kind:           nanoprotocol.CapabilityKind(kind),
				Ref:            ref,
				DefaultEnabled: belt.Allows(kind, ref),
			}
			if _, err := s.db.Collection(agentTypesCollection).UpdateOne(ctx, undecided,
				bson.D{{Key: "$push", Value: bson.D{{Key: "capabilities", Value: row}}}},
			); err != nil {
				return fmt.Errorf("complete the %s belt of %s with %q: %w", kind, belt.TypeID, ref, err)
			}
		}
	}
	return nil
}
