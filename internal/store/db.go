// Package store is nanoadmin's MongoDB access layer.
//
// It writes only the admin_ prefix (and the shared sys_migrations bookkeeping).
// Everything else it needs — a user's identity, an agent's ownership — is asked
// over HTTP, so this package never learns another module's schema. The one
// exception is the audit trail: its listing resolves display names from the
// collections that own them (see names.go), read-only, because a per-page fan
// out to three modules to render a name is worse than three indexed lookups.
package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"gitee.com/kobeyoung81/nanoadmin/internal/metrics"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Store provides access to the platform database.
type Store struct {
	mongo *mongo.Client
	db    *mongo.Database
	// clearTypeRefs asks the owner of the agent rows to clear the type
	// reference before a type is deleted.
	clearTypeRefs func(typeID string) error
}

// Open connects to MongoDB and returns a Store. It pings the server so a bad URI
// (or bad credentials) fails at startup rather than on the first request.
func Open(mongoURI string) (*Store, error) {
	if mongoURI == "" {
		return nil, errors.New("mongo uri is required")
	}
	name, err := databaseNameOf(mongoURI)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// One monitor per client: it feeds the module's database numbers (command
	// counts and latency) without every call site having to time itself.
	client, err := mongo.Connect(options.Client().
		ApplyURI(mongoURI).
		SetMonitor(metrics.Monitor()))
	if err != nil {
		return nil, fmt.Errorf("connect mongo: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping mongo: %w", err)
	}

	return &Store{mongo: client, db: client.Database(name)}, nil
}

// databaseNameOf extracts the database name from a MongoDB URI's path.
func databaseNameOf(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("parse mongo uri: %w", err)
	}
	name := strings.Trim(u.Path, "/")
	if name == "" {
		return "", errors.New("mongo uri must name a database, e.g. mongodb://host:27017/nanosec")
	}
	if i := strings.IndexByte(name, '/'); i >= 0 {
		return "", fmt.Errorf("mongo uri names more than one database: %q", name)
	}
	return name, nil
}

// NewMongo wraps an existing client and database name (tests use it).
func NewMongo(client *mongo.Client, database string) *Store {
	return &Store{mongo: client, db: client.Database(database)}
}

// Close releases the client.
func (s *Store) Close() error {
	if s.mongo == nil {
		return nil
	}
	return s.mongo.Disconnect(context.Background())
}

// DB returns the underlying database.
func (s *Store) DB() *mongo.Database { return s.db }

// opTimeout bounds a single database operation, so a hung server surfaces as an
// error instead of a request that never returns.
const opTimeout = 15 * time.Second

func opCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), opTimeout)
}

// retryOnDuplicateUpsert runs fn again once when it fails with a duplicate-key
// error. MongoDB documents that concurrent upserts on a unique index can raise
// E11000 instead of updating in place; the documented remedy is a retry, and
// without it a race turns into a user-visible failure.
func retryOnDuplicateUpsert(fn func() error) error {
	err := fn()
	if err != nil && mongo.IsDuplicateKeyError(err) {
		return fn()
	}
	return err
}
