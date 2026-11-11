package store

// The MCP server catalogue: platform and per-user servers, their state, and the
// seeder for the first-party entries this deployment ships.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"gitee.com/kobeyoung81/nanoprotocol"
)

const mcpServersCollection = "admin_mcp_servers"

// ErrMcpNotFound reports an MCP lookup or write against a missing row.
var ErrMcpNotFound = fmt.Errorf("mcp server not found")

// McpServerRecord is a row in the MCP server catalogue.
type McpServerRecord struct {
	Capability
	Transport string
	URL       string
	Headers   map[string]string
}

// storedMcpServer is an MCP server document.
type storedMcpServer struct {
	ID    string `bson:"_id"`
	Scope string `bson:"scope"`
	// Written even when empty (the SQL stored NULL): the name-uniqueness check
	// compares it, and an absent field would not match an equality on "".
	OwnerUserID    string            `bson:"owner_user_id"`
	Name           string            `bson:"name"`
	Description    string            `bson:"description,omitempty"`
	Transport      string            `bson:"transport"`
	URL            string            `bson:"url,omitempty"`
	Headers        map[string]string `bson:"headers,omitempty"`
	DefaultEnabled bool              `bson:"default_enabled"`
	Locked         string            `bson:"locked,omitempty"`
	CreatedAt      time.Time         `bson:"created_at"`
	UpdatedAt      time.Time         `bson:"updated_at"`
}

func (d storedMcpServer) record() McpServerRecord {
	return McpServerRecord{
		Capability: Capability{
			ID: d.ID, Scope: d.Scope, OwnerUserID: d.OwnerUserID, Name: d.Name,
			Description: d.Description, DefaultEnabled: d.DefaultEnabled, Locked: d.Locked,
			CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
		},
		Transport: d.Transport, URL: d.URL, Headers: orEmptyHeaders(d.Headers),
	}
}

// orEmptyHeaders keeps the shape the SQL scan produced: a JSON null or "{}" came
// back as an empty map, never as nil.
func orEmptyHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return map[string]string{}
	}
	return headers
}

func (s *Store) ListMcpServers(userID string) ([]McpServerRecord, error) {
	ctx, cancel := opCtx()
	defer cancel()

	cur, err := s.db.Collection(mcpServersCollection).Find(ctx, visibleCatalog(userID),
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list mcp servers: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	out := []McpServerRecord{}
	for cur.Next(ctx) {
		var doc storedMcpServer
		if err := cur.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode mcp server: %w", err)
		}
		out = append(out, doc.record())
	}
	return out, cur.Err()
}

func (s *Store) GetMcpServer(id string) (*McpServerRecord, error) {
	ctx, cancel := opCtx()
	defer cancel()

	var doc storedMcpServer
	err := s.db.Collection(mcpServersCollection).FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrCapabilityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get mcp server: %w", err)
	}
	rec := doc.record()
	return &rec, nil
}

// CreateMcp inserts a new MCP server entry.
func (s *Store) CreateMcp(rec McpServerRecord) error {
	ctx, cancel := opCtx()
	defer cancel()

	if err := s.ensureMcpNameFree(rec.Id(), rec.OwnerUserID, rec.Name); err != nil {
		return err
	}

	now := time.Now().UTC()
	if _, err := s.db.Collection(mcpServersCollection).InsertOne(ctx, storedMcpServer{
		ID: rec.Id(), Scope: rec.Scope, OwnerUserID: rec.OwnerUserID, Name: rec.Name,
		Description: rec.Description, Transport: rec.Transport, URL: rec.URL,
		Headers: rec.Headers, DefaultEnabled: rec.DefaultEnabled, Locked: rec.Locked,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return fmt.Errorf("create mcp server: %w", err)
	}
	// A server every type inherits by default is exactly 081's failure: the
	// read-only roles have to say no out loud.
	return s.completeMcpBelts(ctx)
}

// completeMcpBelts gives every restricted role an explicit decision about every
// configured MCP server (081's rule: the read-only roles refuse them, and an
// MCP tool may have side effects on the far side).
func (s *Store) completeMcpBelts(ctx context.Context) error {
	cur, err := s.db.Collection(mcpServersCollection).Find(ctx, bson.D{},
		options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return fmt.Errorf("list mcp servers: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	ids := make([]string, 0, 4)
	for cur.Next(ctx) {
		var doc struct {
			ID string `bson:"_id"`
		}
		if err := cur.Decode(&doc); err != nil {
			return fmt.Errorf("decode mcp server id: %w", err)
		}
		ids = append(ids, doc.ID)
	}
	if err := cur.Err(); err != nil {
		return fmt.Errorf("list mcp servers: %w", err)
	}
	return s.completeRestrictedBelts(ctx, "mcp", ids)
}

// SeedMcpServer inserts a server the binary ships, leaving an existing row
// untouched: an administrator may have changed its URL, headers or default, and
// a deploy must not undo that. Safe to re-run.
func (s *Store) SeedMcpServer(rec McpServerRecord) (bool, error) {
	ctx, cancel := opCtx()
	defer cancel()

	count, err := s.db.Collection(mcpServersCollection).CountDocuments(ctx,
		bson.D{{Key: "_id", Value: rec.Id()}})
	if err != nil {
		return false, fmt.Errorf("check mcp server: %w", err)
	}
	if count > 0 {
		return false, nil
	}
	if err := s.CreateMcp(rec); err != nil {
		return false, err
	}
	return true, nil
}

// UpdateMcp replaces an MCP server entry (overwrite-in-place).
func (s *Store) UpdateMcp(id string, rec McpServerRecord) error {
	ctx, cancel := opCtx()
	defer cancel()

	// The SQL UPDATE matched zero rows for an unknown id and said nothing, so
	// neither does this.
	_, err := s.db.Collection(mcpServersCollection).UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "scope", Value: rec.Scope},
			{Key: "owner_user_id", Value: rec.OwnerUserID},
			{Key: "name", Value: rec.Name},
			{Key: "description", Value: rec.Description},
			{Key: "transport", Value: rec.Transport},
			{Key: "url", Value: rec.URL},
			{Key: "headers", Value: rec.Headers},
			{Key: "default_enabled", Value: rec.DefaultEnabled},
			{Key: "locked", Value: rec.Locked},
			{Key: "updated_at", Value: time.Now().UTC()},
		}}},
	)
	if err != nil {
		return fmt.Errorf("update mcp server: %w", err)
	}
	return nil
}

// UpdateMcpState updates only the definition-plane default (default_enabled /
// locked) for an MCP server; used by the Capability Center's inline controls.
func (s *Store) UpdateMcpState(id string, defaultEnabled *bool, locked *string) error {
	return s.updateCatalogState(mcpServersCollection, id, defaultEnabled, locked, ErrCapabilityNotFound)
}

func (s *Store) ensureMcpNameFree(id, ownerUserID, name string) error {
	ctx, cancel := opCtx()
	defer cancel()

	// Same soft check as the SQL: it counts the caller's own name collisions
	// before inserting, and two concurrent creates could still race — the
	// catalog has no unique key on (scope, owner, name) to lean on.
	count, err := s.db.Collection(mcpServersCollection).CountDocuments(ctx, bson.D{
		{Key: "scope", Value: "user"},
		{Key: "owner_user_id", Value: ownerUserID},
		{Key: "name", Value: name},
		{Key: "_id", Value: bson.D{{Key: "$ne", Value: id}}},
	})
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("an MCP server named %q already exists", name)
	}
	return nil
}

func (s *Store) DeleteMcp(id string, ownerUserID, scope string) error {
	ctx, cancel := opCtx()
	defer cancel()

	filter := bson.D{{Key: "_id", Value: id}}
	if scope == "user" {
		filter = append(filter, bson.E{Key: "owner_user_id", Value: ownerUserID})
	}

	res, err := s.db.Collection(mcpServersCollection).DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("delete mcp server: %w", err)
	}
	if res.DeletedCount == 0 {
		return ErrCapabilityNotFound
	}
	return nil
}

// ListModeCapabilities returns the user's capability overrides for one
// (agent, mode) as catalog attachments.
func (s *Store) ListModeCapabilities(agentID, typeID string) ([]nanoprotocol.CapabilityAttachment, error) {
	ov, err := s.GetAgentTypeOverride(agentID, typeID)
	if err != nil || ov == nil {
		return nil, err
	}
	return ov.Capabilities, nil
}

// SetModeCapability upserts one capability override for an (agent, mode).
func (s *Store) SetModeCapability(agentID, typeID, kind, ref string, enabled bool) error {
	ov, err := s.GetAgentTypeOverride(agentID, typeID)
	if err != nil {
		return err
	}
	if ov == nil {
		ov = &nanoprotocol.AgentTypeOverride{}
	}
	k := nanoprotocol.CapabilityKind(kind)
	found := false
	for i := range ov.Capabilities {
		if ov.Capabilities[i].Kind == k && ov.Capabilities[i].Ref == ref {
			ov.Capabilities[i].Enabled = enabled
			found = true
			break
		}
	}
	if !found {
		ov.Capabilities = append(ov.Capabilities, nanoprotocol.CapabilityAttachment{Kind: k, Ref: ref, Enabled: enabled})
	}
	return s.SetAgentTypeOverride(agentID, typeID, *ov)
}

// DeleteModeCapability removes one capability override for an (agent, mode),
// reverting the item to the type baseline.
func (s *Store) DeleteModeCapability(agentID, typeID, kind, ref string) error {
	ov, err := s.GetAgentTypeOverride(agentID, typeID)
	if err != nil || ov == nil {
		return err
	}
	k := nanoprotocol.CapabilityKind(kind)
	kept := ov.Capabilities[:0]
	for _, c := range ov.Capabilities {
		if c.Kind == k && c.Ref == ref {
			continue
		}
		kept = append(kept, c)
	}
	ov.Capabilities = kept
	return s.SetAgentTypeOverride(agentID, typeID, *ov)
}
