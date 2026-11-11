package store

// The skill catalogue: the definitions (bundled, platform and per-user), their
// per-type baselines, the per-user acknowledgements that drive re-materialization,
// and the seeder for the packages this binary ships.
//
// The bus keeps a read-only view for the agent snapshot; the writes are here.

import (
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"gitee.com/kobeyoung81/nanoprotocol"
)

const (
	skillsCollection        = "admin_skills"
	userSkillAcksCollection = "admin_user_skill_acks"
)

// Capability is the part of a catalogue row every kind shares.
type Capability struct {
	ID             string
	Scope          string
	OwnerUserID    string
	Name           string
	Description    string
	DefaultEnabled bool
	Locked         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SkillRecord is a row in the skill catalogue.
type SkillRecord struct {
	Capability
	Files            map[string]string
	Origin           string
	Version          int
	Status           string
	PublishedVersion int
	ContentHash      string
	CreatedBy        string
}

// Id returns the row's id.
func (c Capability) Id() string { return c.ID }

// visibleCatalog is the visibility rule the per-kind readers share: platform
// rows for everyone, plus the caller's own.
func visibleCatalog(userID string) bson.D {
	return bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "scope", Value: "platform"}},
		bson.D{{Key: "scope", Value: "user"}, {Key: "owner_user_id", Value: userID}},
	}}}
}

// updateCatalogState writes the definition-plane default for one row.
func (s *Store) updateCatalogState(collection, id string, defaultEnabled *bool, locked *string, missing error) error {
	ctx, cancel := opCtx()
	defer cancel()

	set := bson.D{{Key: "updated_at", Value: time.Now().UTC()}}
	if defaultEnabled != nil {
		set = append(set, bson.E{Key: "default_enabled", Value: *defaultEnabled})
	}
	if locked != nil {
		set = append(set, bson.E{Key: "locked", Value: *locked})
	}
	res, err := s.db.Collection(collection).UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}}, bson.D{{Key: "$set", Value: set}})
	if err != nil {
		return fmt.Errorf("update catalog state: %w", err)
	}
	if res.MatchedCount == 0 {
		return missing
	}
	return nil
}

// storedSkill is a skill document.
//
// The files map is stored as it is: its keys are file names ("SKILL.md") that a
// document holds happily, and the protocol hands the same map back.
type storedSkill struct {
	ID               string            `bson:"_id"`
	Scope            string            `bson:"scope"`
	OwnerUserID      string            `bson:"owner_user_id"`
	Name             string            `bson:"name"`
	Description      string            `bson:"description,omitempty"`
	Files            map[string]string `bson:"files"`
	DefaultEnabled   bool              `bson:"default_enabled"`
	Locked           string            `bson:"locked,omitempty"`
	Origin           string            `bson:"origin"`
	Version          int               `bson:"version"`
	Status           string            `bson:"status"`
	PublishedVersion int               `bson:"published_version"`
	ContentHash      string            `bson:"content_hash,omitempty"`
	CreatedBy        string            `bson:"created_by,omitempty"`
	CreatedAt        time.Time         `bson:"created_at"`
	UpdatedAt        time.Time         `bson:"updated_at"`
}

func (d storedSkill) record() SkillRecord {
	return SkillRecord{
		Capability: Capability{
			ID: d.ID, Scope: d.Scope, OwnerUserID: d.OwnerUserID, Name: d.Name,
			Description: d.Description, DefaultEnabled: d.DefaultEnabled, Locked: d.Locked,
			CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
		},
		Files:            orEmptyFiles(d.Files),
		Origin:           d.Origin,
		Version:          d.Version,
		Status:           d.Status,
		PublishedVersion: d.PublishedVersion,
		ContentHash:      d.ContentHash,
		CreatedBy:        d.CreatedBy,
	}
}

// orEmptyFiles keeps the SQL scan's shape: a null or "{}" files column came back
// as an empty map, never nil.
func orEmptyFiles(files map[string]string) map[string]string {
	if files == nil {
		return map[string]string{}
	}
	return files
}

func (s *Store) ListSkills(userID string) ([]SkillRecord, error) {
	ctx, cancel := opCtx()
	defer cancel()

	cur, err := s.db.Collection(skillsCollection).Find(ctx, visibleCatalog(userID),
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	out := []SkillRecord{}
	for cur.Next(ctx) {
		var doc storedSkill
		if err := cur.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode skill: %w", err)
		}
		out = append(out, doc.record())
	}
	return out, cur.Err()
}

func (s *Store) GetSkill(id string) (*SkillRecord, error) {
	ctx, cancel := opCtx()
	defer cancel()

	var doc storedSkill
	err := s.db.Collection(skillsCollection).FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrCapabilityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get skill: %w", err)
	}
	rec := doc.record()
	return &rec, nil
}

// SkillNameTaken reports whether a skill name is already used by a platform
// skill or by a skill owned by the given user (names are globally unique).
func (s *Store) SkillNameTaken(name, userID, excludeID string) (bool, error) {
	ctx, cancel := opCtx()
	defer cancel()

	count, err := s.db.Collection(skillsCollection).CountDocuments(ctx, bson.D{
		{Key: "name", Value: name},
		{Key: "_id", Value: bson.D{{Key: "$ne", Value: excludeID}}},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "scope", Value: "platform"}},
			bson.D{{Key: "owner_user_id", Value: userID}},
		}},
	})
	if err != nil {
		return false, fmt.Errorf("check skill name: %w", err)
	}
	return count > 0, nil
}

func (s *Store) CreateSkill(rec SkillRecord) error {
	if rec.Origin == "" {
		rec.Origin = "human"
	}
	if rec.Status == "" {
		rec.Status = "published"
	}
	if rec.Version == 0 {
		rec.Version = 1
	}
	if rec.PublishedVersion == 0 {
		rec.PublishedVersion = rec.Version
	}

	ctx, cancel := opCtx()
	defer cancel()

	now := time.Now().UTC()
	if _, err := s.db.Collection(skillsCollection).InsertOne(ctx, storedSkill{
		ID: rec.Id(), Scope: rec.Scope, OwnerUserID: rec.OwnerUserID, Name: rec.Name,
		Description: rec.Description, Files: rec.Files,
		DefaultEnabled: rec.DefaultEnabled, Locked: rec.Locked,
		Origin: rec.Origin, Version: rec.Version, Status: rec.Status,
		PublishedVersion: rec.PublishedVersion, ContentHash: rec.ContentHash,
		CreatedBy: rec.CreatedBy,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		// A duplicate name is refused by the unique index, as it was by the
		// SQL unique key.
		return fmt.Errorf("create skill: %w", err)
	}
	return nil
}

func (s *Store) UpdateSkill(id string, rec SkillRecord) error {
	ctx, cancel := opCtx()
	defer cancel()

	set := bson.D{
		{Key: "scope", Value: rec.Scope},
		{Key: "owner_user_id", Value: rec.OwnerUserID},
		{Key: "name", Value: rec.Name},
		{Key: "description", Value: rec.Description},
		{Key: "files", Value: rec.Files},
		{Key: "default_enabled", Value: rec.DefaultEnabled},
		{Key: "locked", Value: rec.Locked},
		{Key: "origin", Value: rec.Origin},
		{Key: "version", Value: rec.Version},
		{Key: "status", Value: rec.Status},
		{Key: "published_version", Value: rec.PublishedVersion},
		{Key: "content_hash", Value: rec.ContentHash},
		{Key: "updated_at", Value: time.Now().UTC()},
	}
	update := bson.D{{Key: "$set", Value: set}}
	// created_by = NULLIF(?, '').
	if rec.CreatedBy == "" {
		update = append(update, bson.E{Key: "$unset", Value: bson.D{{Key: "created_by", Value: ""}}})
	} else {
		update[0].Value = append(set, bson.E{Key: "created_by", Value: rec.CreatedBy})
	}

	// As with MCP: an unknown id was a silent no-op in SQL.
	if _, err := s.db.Collection(skillsCollection).UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}}, update); err != nil {
		return fmt.Errorf("update skill: %w", err)
	}
	return nil
}

// UpdateSkillState updates only the definition-plane default (default_enabled /
// locked) for a skill. It works for read-only bundled skills too, whose content
// cannot be edited but whose availability baseline is platform-managed.
func (s *Store) UpdateSkillState(id string, defaultEnabled *bool, locked *string) error {
	return s.updateCatalogState(skillsCollection, id, defaultEnabled, locked, ErrCapabilityNotFound)
}

func (s *Store) DeleteSkill(id string, ownerUserID, scope string) error {
	ctx, cancel := opCtx()
	defer cancel()

	filter := bson.D{{Key: "_id", Value: id}}
	if scope == "user" {
		filter = append(filter, bson.E{Key: "owner_user_id", Value: ownerUserID})
	}

	res, err := s.db.Collection(skillsCollection).DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("delete skill: %w", err)
	}
	if res.DeletedCount == 0 {
		return ErrCapabilityNotFound
	}
	return nil
}

func skillAckIdentity(userID, skillID string) bson.D {
	return bson.D{
		{Key: "user_id", Value: userID},
		{Key: "skill_id", Value: skillID},
	}
}

// AckSkill records that a user acknowledged a platform skill version.
func (s *Store) AckSkill(userID, skillID string, version int) error {
	ctx, cancel := opCtx()
	defer cancel()

	// $max is the whole of `GREATEST(acked_version, VALUES(acked_version))`, and
	// it is atomic on its own: an acknowledgement that arrives out of order
	// cannot lower the recorded version.
	err := retryOnDuplicateUpsert(func() error {
		_, err := s.db.Collection(userSkillAcksCollection).UpdateOne(ctx,
			skillAckIdentity(userID, skillID),
			bson.D{
				{Key: "$max", Value: bson.D{{Key: "acked_version", Value: version}}},
				{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now().UTC()}}},
			},
			options.UpdateOne().SetUpsert(true))
		return err
	})
	if err != nil {
		return fmt.Errorf("ack skill: %w", err)
	}
	return nil
}

// SkillAckVersion returns the acknowledged version for (user, skill), or 0.
func (s *Store) SkillAckVersion(userID, skillID string) (int, error) {
	ctx, cancel := opCtx()
	defer cancel()

	var doc struct {
		AckedVersion int `bson:"acked_version"`
	}
	err := s.db.Collection(userSkillAcksCollection).FindOne(ctx,
		skillAckIdentity(userID, skillID)).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get skill ack: %w", err)
	}
	return doc.AckedVersion, nil
}

// GetUserSkillByName returns a user-scoped skill owned by userID, else ErrNotFound.
func (s *Store) GetUserSkillByName(userID, name string) (*SkillRecord, error) {
	ctx, cancel := opCtx()
	defer cancel()

	var doc storedSkill
	err := s.db.Collection(skillsCollection).FindOne(ctx, bson.D{
		{Key: "scope", Value: "user"},
		{Key: "owner_user_id", Value: userID},
		{Key: "name", Value: name},
	}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrCapabilityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get skill by name: %w", err)
	}
	rec := doc.record()
	return &rec, nil
}

// BundledSkillID returns the stable catalog id for a bundled skill name. The
// id is deterministic so per-type baselines (agent_type_capabilities.ref) and
// per-user acks survive content updates across releases.
func BundledSkillID(name string) string {
	return "skill:bundled-" + name
}

// SeedPlatformSkills upserts the read-only bundled skill catalog from the
// nanobus binary. It is idempotent and safe to run on every boot:
//   - new skills are inserted and enabled on the default type baseline;
//   - a newer version replaces content (files + hash + version), preserving the
//     per-type baseline and per-user acks;
//   - an older version is ignored so a stale binary cannot downgrade content;
//   - a name owned by a non-bundled skill is skipped so it cannot be hijacked.
//
// It returns the number of skills inserted and updated.
func (s *Store) SeedPlatformSkills(skills []nanoprotocol.SkillDef) (inserted, updated int, err error) {
	for _, sk := range skills {
		if sk.Name == "" || len(sk.Files) == 0 {
			continue
		}
		id := BundledSkillID(sk.Name)
		version := sk.Version
		if version <= 0 {
			version = 1
		}

		existing, err := s.GetSkill(id)
		if err == ErrCapabilityNotFound {
			existing = nil
		} else if err != nil {
			return inserted, updated, fmt.Errorf("seed skill %q: %w", sk.Name, err)
		}

		if existing == nil {
			taken, terr := s.SkillNameTaken(sk.Name, "", id)
			if terr != nil {
				return inserted, updated, fmt.Errorf("seed skill %q: %w", sk.Name, terr)
			}
			if taken {
				// A platform/admin or user skill already owns the name.
				continue
			}
			rec := SkillRecord{
				Capability: Capability{
					ID:             id,
					Scope:          "platform",
					Name:           sk.Name,
					Description:    sk.Description,
					DefaultEnabled: true,
				},
				Files:            sk.Files,
				Origin:           "bundled",
				Status:           "published",
				Version:          version,
				PublishedVersion: version,
				ContentHash:      sk.ContentHash,
				CreatedBy:        "nanobus",
			}
			if cerr := s.CreateSkill(rec); cerr != nil {
				return inserted, updated, fmt.Errorf("seed skill %q: %w", sk.Name, cerr)
			}
			inserted++
			continue
		}

		if existing.Origin != "bundled" {
			continue
		}
		if version < existing.Version {
			continue
		}
		if version == existing.Version && sk.ContentHash == existing.ContentHash {
			continue
		}
		if uerr := s.updateBundledSkillContent(id, sk.Description, sk.Files, version, sk.ContentHash); uerr != nil {
			return inserted, updated, uerr
		}
		updated++
	}
	return inserted, updated, nil
}

// updateBundledSkillContent rewrites only the content fields of a bundled
// skill. The per-type baseline (agent_type_capabilities) and per-user acks are
// deliberately left untouched.
func (s *Store) updateBundledSkillContent(id, description string, files map[string]string, version int, hash string) error {
	ctx, cancel := opCtx()
	defer cancel()

	// The `origin = 'bundled'` condition is the SQL's guard, kept on the write
	// itself so a name that changed hands since the read cannot have its content
	// rewritten.
	res, err := s.db.Collection(skillsCollection).UpdateOne(ctx,
		bson.D{
			{Key: "_id", Value: id},
			{Key: "origin", Value: "bundled"},
		},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "description", Value: description},
			{Key: "files", Value: files},
			{Key: "version", Value: version},
			{Key: "published_version", Value: version},
			{Key: "status", Value: "published"},
			{Key: "content_hash", Value: hash},
			{Key: "updated_at", Value: time.Now().UTC()},
		}}},
	)
	if err != nil {
		return fmt.Errorf("update bundled skill %q: %w", id, err)
	}
	if res.MatchedCount == 0 {
		// Only reachable when the row stopped being a bundled skill between the
		// read above and this write. The SQL matched no row and reported no
		// error, and the seeder counted a skill it had not touched as updated;
		// this is the one place the old code could lie, so it says so instead.
		return fmt.Errorf("update bundled skill %q: no longer a bundled skill", id)
	}
	return nil
}
