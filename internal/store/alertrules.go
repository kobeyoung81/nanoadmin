package store

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// alertRulesCollection holds the platform's alert rules.
//
// The rules are pure data: nothing in the platform evaluates them yet, so this
// service owns the whole lifecycle (list, upsert, delete) and no other module
// reads them.
const alertRulesCollection = "admin_alert_rules"

// countersCollection holds the named counters that stand in for MySQL's
// AUTO_INCREMENT.
const countersCollection = "sys_counters"

// AlertRule is one row of the alert rule table.
type AlertRule struct {
	ID        int64     `json:"id"`
	Severity  string    `json:"severity"`
	Message   string    `json:"message"`
	Metric    string    `json:"metric"`
	Threshold float64   `json:"threshold"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// storedAlertRule is the document shape.
type storedAlertRule struct {
	ID        int64     `bson:"_id"`
	Severity  string    `bson:"severity"`
	Message   string    `bson:"message"`
	Metric    string    `bson:"metric"`
	Threshold float64   `bson:"threshold"`
	Enabled   bool      `bson:"enabled"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

func (d storedAlertRule) rule() AlertRule {
	return AlertRule{
		ID: d.ID, Severity: d.Severity, Message: d.Message,
		Metric: d.Metric, Threshold: d.Threshold, Enabled: d.Enabled,
		CreatedAt: d.CreatedAt,
	}
}

// ListAlertRules returns the enabled rules, ordered by id.
func (s *Store) ListAlertRules() ([]AlertRule, error) {
	ctx, cancel := opCtx()
	defer cancel()

	cur, err := s.db.Collection(alertRulesCollection).Find(ctx,
		bson.D{{Key: "enabled", Value: true}},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}),
	)
	if err != nil {
		return nil, fmt.Errorf("list alert rules: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	rules := []AlertRule{}
	for cur.Next(ctx) {
		var doc storedAlertRule
		if err := cur.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode alert rule: %w", err)
		}
		rules = append(rules, doc.rule())
	}
	return rules, cur.Err()
}

// UpsertAlertRule inserts or updates a rule and returns the id it holds.
//
// A zero id means "new rule" (the console's create path); the id is handed out
// from a counter because it is part of the API — the console deletes a rule by
// id, so it cannot become an ObjectId.
func (s *Store) UpsertAlertRule(rule AlertRule) (int64, error) {
	ctx, cancel := opCtx()
	defer cancel()
	coll := s.db.Collection(alertRulesCollection)
	now := time.Now().UTC()

	if rule.ID == 0 {
		id, err := s.nextSequence(ctx, alertRulesCollection)
		if err != nil {
			return 0, err
		}
		doc := storedAlertRule{
			ID: id, Severity: rule.Severity, Message: rule.Message, Metric: rule.Metric,
			Threshold: rule.Threshold, Enabled: rule.Enabled,
			CreatedAt: now, UpdatedAt: now,
		}
		if _, err := coll.InsertOne(ctx, doc); err != nil {
			return 0, fmt.Errorf("insert alert rule: %w", err)
		}
		return id, nil
	}

	// An unknown id matches nothing and reports no error, which is what the SQL
	// UPDATE did; the console treats it as a no-op.
	_, err := coll.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: rule.ID}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "severity", Value: rule.Severity},
			{Key: "message", Value: rule.Message},
			{Key: "metric", Value: rule.Metric},
			{Key: "threshold", Value: rule.Threshold},
			{Key: "enabled", Value: rule.Enabled},
			{Key: "updated_at", Value: now},
		}}},
	)
	if err != nil {
		return 0, fmt.Errorf("update alert rule: %w", err)
	}
	return rule.ID, nil
}

// DeleteAlertRule removes a rule; a missing id is not an error.
func (s *Store) DeleteAlertRule(id int64) error {
	ctx, cancel := opCtx()
	defer cancel()

	if _, err := s.db.Collection(alertRulesCollection).DeleteOne(ctx, bson.D{{Key: "_id", Value: id}}); err != nil {
		return fmt.Errorf("delete alert rule: %w", err)
	}
	return nil
}

// nextSequence returns the next value of a named counter (see countersCollection).
func (s *Store) nextSequence(ctx context.Context, name string) (int64, error) {
	var doc struct {
		Seq int64 `bson:"seq"`
	}
	err := retryOnDuplicateUpsert(func() error {
		return s.db.Collection(countersCollection).FindOneAndUpdate(ctx,
			bson.D{{Key: "_id", Value: name}},
			bson.D{{Key: "$inc", Value: bson.D{{Key: "seq", Value: int64(1)}}}},
			options.FindOneAndUpdate().
				SetUpsert(true).
				SetReturnDocument(options.After),
		).Decode(&doc)
	})
	if err != nil {
		return 0, fmt.Errorf("next %s id: %w", name, err)
	}
	return doc.Seq, nil
}
