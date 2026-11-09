package store

import (
	"errors"
	"fmt"

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
