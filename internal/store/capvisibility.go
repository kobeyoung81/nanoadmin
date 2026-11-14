package store

// Whether one catalog item is visible to a user.
//
// The bus used to answer this by reading admin_builtin_tools, admin_mcp_servers
// and admin_skills itself, on a write path (granting a per-agent capability). The
// collections are this module's, so it answers instead.

import (
	"go.mongodb.org/mongo-driver/v2/bson"
)

// CapabilityVisible reports whether a catalog item of the given kind is visible
// to (and resolvable by) the user: platform scope, or owned by the user.
func (s *Store) CapabilityVisible(kind, ref, userID string) (bool, error) {
	switch kind {
	case "builtin_tool":
		ctx, cancel := opCtx()
		defer cancel()
		count, err := s.db.Collection(builtinToolsCollection).CountDocuments(ctx, bson.D{{Key: "_id", Value: ref}})
		if err != nil {
			return false, err
		}
		return count > 0, nil
	case "mcp":
		rec, err := s.GetMcpServer(ref)
		if err != nil || rec == nil {
			return false, nil
		}
		if rec.Scope == "user" {
			return rec.OwnerUserID == userID, nil
		}
		return true, nil
	case "skill":
		rec, err := s.GetSkill(ref)
		if err != nil || rec == nil {
			return false, nil
		}
		if rec.Scope == "user" {
			return rec.OwnerUserID == userID, nil
		}
		return true, nil
	}
	return false, nil
}
