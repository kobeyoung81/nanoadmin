package store

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"gitee.com/kobeyoung81/nanoprotocol"
)

// permissionKeysCollection is the catalog of permission keys a policy may name.
//
// No migration manifest names it: every read is by _id — including the listing,
// which orders by it — and MongoDB indexes _id with the collection.
const permissionKeysCollection = "admin_permission_keys"

// storedPermissionKey is a catalog entry.
type storedPermissionKey struct {
	ID            string                        `bson:"_id"`
	Description   string                        `bson:"description,omitempty"`
	DefaultAction nanoprotocol.PermissionAction `bson:"default_action"`
	ReadOnly      bool                          `bson:"read_only"`
	UpdatedAt     time.Time                     `bson:"updated_at"`
}

// SeedPermissionKeys fills the catalog from the binary.
//
// It inserts what is missing and touches nothing that is there: the catalog is
// admin-editable (the console edits a key's description and default action), and
// a seed that overwrote them would undo that on every deploy.
//
// Retiring a key is deliberately not this method's job: a seeder runs from
// whatever binary is deployed, and during a rollback that is an older one, which
// must not delete a key a newer binary added.
func (s *Store) SeedPermissionKeys(keys []nanoprotocol.PermissionKeyDef) (inserted int, err error) {
	ctx, cancel := opCtx()
	defer cancel()

	now := time.Now().UTC()
	for _, k := range keys {
		if k.ID == "" {
			continue
		}
		update := bson.D{{Key: "$setOnInsert", Value: bson.D{
			{Key: "description", Value: k.Description},
			{Key: "default_action", Value: k.DefaultAction},
			{Key: "read_only", Value: k.ReadOnly},
			{Key: "updated_at", Value: now},
		}}}
		res, err := s.db.Collection(permissionKeysCollection).UpdateOne(ctx,
			bson.D{{Key: "_id", Value: k.ID}}, update, options.UpdateOne().SetUpsert(true))
		if err != nil {
			return inserted, fmt.Errorf("seed permission key %q: %w", k.ID, err)
		}
		inserted += int(res.UpsertedCount)
	}
	return inserted, nil
}

// ListPermissionKeys returns the catalog ordered by id.
func (s *Store) ListPermissionKeys() ([]nanoprotocol.PermissionKeyDef, error) {
	ctx, cancel := opCtx()
	defer cancel()

	cur, err := s.db.Collection(permissionKeysCollection).Find(ctx, bson.D{},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list permission keys: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	out := []nanoprotocol.PermissionKeyDef{}
	for cur.Next(ctx) {
		var doc storedPermissionKey
		if err := cur.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode permission key: %w", err)
		}
		out = append(out, nanoprotocol.PermissionKeyDef{
			ID: doc.ID, Description: doc.Description,
			DefaultAction: doc.DefaultAction, ReadOnly: doc.ReadOnly,
		})
	}
	return out, cur.Err()
}
