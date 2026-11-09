package store

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// configItemsCollection is the runtime configuration every service reads at
// startup. Until the last writer moves here, the bus still writes it; this
// service reads it through the same collection name so a cutover needs no data
// change.
const configItemsCollection = "admin_config"

// ConfigItem is one configuration entry.
type ConfigItem struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	ValueType   string `json:"value_type"`
	Description string `json:"description"`
}

// storedConfigItem is the document shape.
//
// Deliberately not ConfigItem: decoding straight into a type that carries only
// json tags would depend on how the driver lowercases Go field names rather
// than on what the documents actually hold.
type storedConfigItem struct {
	ComponentType string `bson:"component_type"`
	Key           string `bson:"config_key"`
	Value         string `bson:"config_value"`
	ValueType     string `bson:"value_type"`
	Description   string `bson:"description"`
}

func (d storedConfigItem) item() ConfigItem {
	return ConfigItem{Key: d.Key, Value: d.Value, ValueType: d.ValueType, Description: d.Description}
}

// GetConfigItems returns one component's configuration, ordered by key.
func (s *Store) GetConfigItems(componentType string) ([]ConfigItem, error) {
	ctx, cancel := opCtx()
	defer cancel()

	cur, err := s.db.Collection(configItemsCollection).Find(ctx,
		bson.D{{Key: "component_type", Value: componentType}},
		// The unique index is (component_type, config_key), so this sort is
		// served by the index rather than by an in-memory sort.
		options.Find().SetSort(bson.D{{Key: "config_key", Value: 1}}),
	)
	if err != nil {
		return nil, fmt.Errorf("query config items: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	items := []ConfigItem{}
	for cur.Next(ctx) {
		var doc storedConfigItem
		if err := cur.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode config item: %w", err)
		}
		items = append(items, doc.item())
	}
	return items, cur.Err()
}

// GetConfigItem returns a single entry. It reports whether the row exists so a
// caller can tell "not configured" from "configured to the empty string".
func (s *Store) GetConfigItem(componentType, key string) (ConfigItem, bool, error) {
	ctx, cancel := opCtx()
	defer cancel()

	var doc storedConfigItem
	err := s.db.Collection(configItemsCollection).FindOne(ctx, bson.D{
		{Key: "component_type", Value: componentType},
		{Key: "config_key", Value: key},
	}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return ConfigItem{}, false, nil
		}
		return ConfigItem{}, false, fmt.Errorf("get config item: %w", err)
	}
	return doc.item(), true, nil
}

// ListConfigComponents returns the components that have configuration, for a
// console index.
func (s *Store) ListConfigComponents() ([]string, error) {
	ctx, cancel := opCtx()
	defer cancel()

	var values []string
	if err := s.db.Collection(configItemsCollection).Distinct(ctx, "component_type", bson.D{}).Decode(&values); err != nil {
		return nil, fmt.Errorf("list config components: %w", err)
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out, nil
}

// configItemKey is the identity of one item: the pair the unique index covers.
func configItemKey(componentType, key string) bson.D {
	return bson.D{
		{Key: "component_type", Value: componentType},
		{Key: "config_key", Value: key},
	}
}

// SetConfigItemTyped writes a value, creating the item when it is new.
//
// value_type and description apply on insert only: an existing item keeps
// whatever it had, so editing a value never silently reclassifies it (a
// `secret` must stay masked, a `string` must not become one). This mirrors the
// SQL upsert the bus used before the domain moved here.
func (s *Store) SetConfigItemTyped(componentType, key, value, valueType, description string) error {
	ctx, cancel := opCtx()
	defer cancel()

	if valueType == "" {
		valueType = "string"
	}

	now := time.Now().UTC()
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "config_value", Value: value},
			{Key: "updated_at", Value: now},
		}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "value_type", Value: valueType},
			{Key: "description", Value: description},
		}},
	}
	_, err := s.db.Collection(configItemsCollection).UpdateOne(ctx,
		configItemKey(componentType, key), update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("upsert config item: %w", err)
	}
	return nil
}

// SeedConfigItems inserts defaults without overwriting anything already there.
//
// Seeding runs on every start, and an operator's edited value must survive it —
// that is the whole point of $setOnInsert here, in contrast with
// SetConfigItemTyped, which always writes the value.
func (s *Store) SeedConfigItems(componentType string, items []ConfigItem) error {
	if len(items) == 0 {
		return nil
	}
	ctx, cancel := opCtx()
	defer cancel()
	coll := s.db.Collection(configItemsCollection)

	for _, item := range items {
		// Only the value fields: component_type and config_key come from the
		// filter on upsert, and naming them here too risks a path conflict.
		update := bson.D{{Key: "$setOnInsert", Value: bson.D{
			{Key: "config_value", Value: item.Value},
			{Key: "value_type", Value: item.ValueType},
			{Key: "description", Value: item.Description},
			{Key: "updated_at", Value: time.Now().UTC()},
		}}}
		err := retryOnDuplicateUpsert(func() error {
			_, err := coll.UpdateOne(ctx, configItemKey(componentType, item.Key), update,
				options.UpdateOne().SetUpsert(true))
			return err
		})
		if err != nil {
			return fmt.Errorf("seed config item %q.%q: %w", componentType, item.Key, err)
		}
	}
	return nil
}

// ConfigUpdatedAt reports when the component last changed, or the zero time
// when it has no items at all. The console shows "last changed" per component,
// so an empty answer has to be distinguishable from an error.
func (s *Store) ConfigUpdatedAt(componentType string) (time.Time, error) {
	ctx, cancel := opCtx()
	defer cancel()

	var doc struct {
		UpdatedAt time.Time `bson:"updated_at"`
	}
	err := s.db.Collection(configItemsCollection).FindOne(ctx,
		bson.D{{Key: "component_type", Value: componentType}},
		options.FindOne().
			SetProjection(bson.D{{Key: "updated_at", Value: 1}}).
			SetSort(bson.D{{Key: "updated_at", Value: -1}}),
	).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("config updated at: %w", err)
	}
	return doc.UpdatedAt, nil
}

// CountConfigItems counts a component's items, optionally narrowed by a key
// prefix/suffix and an exact value. The console's overview asks this to show
// "12 items, 3 matching X" without downloading the component.
func (s *Store) CountConfigItems(componentType, keyPrefix, keySuffix, value string) (int, error) {
	ctx, cancel := opCtx()
	defer cancel()

	filter := bson.D{{Key: "component_type", Value: componentType}}
	if keyPrefix != "" || keySuffix != "" {
		pattern := "^" + regexp.QuoteMeta(keyPrefix) + ".*" + regexp.QuoteMeta(keySuffix) + "$"
		filter = append(filter, bson.E{Key: "config_key", Value: bson.D{{Key: "$regex", Value: pattern}}})
	}
	if value != "" {
		filter = append(filter, bson.E{Key: "config_value", Value: value})
	}

	count, err := s.db.Collection(configItemsCollection).CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("count config items: %w", err)
	}
	return int(count), nil
}
