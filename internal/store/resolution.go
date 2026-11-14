package store

// Resolving one agent's runtime from the catalogue.
//
// The bus used to do this itself: it read admin_agent_types, admin_models,
// admin_prompts, admin_skills, admin_mcp_servers, admin_builtin_tools and
// admin_agent_type_overrides straight out of the shared database and merged them
// into the snapshot an agent starts with. Those collections are this module's, so
// the merge lives here now and the bus asks for the answer.
//
// What this module cannot answer alone is which types one agent runs — that is the
// agent row's, and the rows are the portal's. The caller passes those facts in
// (AgentID, UserID, PrimaryType, SessionTypes, DefaultType) and gets back exactly
// what the runtime used to compute: the capability catalogue for its default mode,
// its effective settings, its definitions, and its primary type.

import (
	"fmt"

	"gitee.com/kobeyoung81/nanoprotocol"
)

// RuntimeInput is the agent-side facts the resolution needs.
type RuntimeInput struct {
	AgentID string `json:"agent_id"`
	// UserID is the account that owns the agent: catalogues are visible per owner.
	UserID string `json:"user_id"`
	// PrimaryType is the agent's own type ("" means the platform default).
	PrimaryType string `json:"primary_type"`
	// SessionTypes are the types the agent runs as switchable modes, in order.
	SessionTypes []string `json:"session_types"`
	// DefaultType is the mode a turn starts in ("" falls back to PrimaryType).
	DefaultType string `json:"default_type"`
}

// RuntimeResolution is the answer.
type RuntimeResolution struct {
	// DefaultModeTypeID is the mode the capability catalogue was resolved for.
	DefaultModeTypeID string                             `json:"default_mode_type_id"`
	Capabilities      *nanoprotocol.CapabilitiesResponse `json:"capabilities"`
	Settings          map[string]string                  `json:"settings"`
	Definitions       []nanoprotocol.AgentTypeDef        `json:"definitions"`
	Type              *nanoprotocol.AgentTypeDef         `json:"type"`
	// DefaultModeModelID/Name are the model that mode runs with: the mode's
	// override when it names one, else the type's default. The list views show
	// it, so it comes back with the resolution instead of a second round trip.
	DefaultModeModelID   int64  `json:"default_mode_model_id"`
	DefaultModeModelName string `json:"default_mode_model_name"`
}

// ResolveRuntime answers everything the runtime reads from the catalogue.
func (s *Store) ResolveRuntime(in RuntimeInput) (RuntimeResolution, error) {
	modeID := in.DefaultModeTypeID()
	caps, err := s.ResolveModeCapabilities(in.AgentID, in.UserID, modeID)
	if err != nil {
		return RuntimeResolution{}, fmt.Errorf("resolve capabilities: %w", err)
	}
	settings, err := s.effectiveSettings(in)
	if err != nil {
		return RuntimeResolution{}, fmt.Errorf("resolve settings: %w", err)
	}
	defs, err := s.resolveDefinitions(in)
	if err != nil {
		return RuntimeResolution{}, fmt.Errorf("resolve definitions: %w", err)
	}
	var primary *nanoprotocol.AgentTypeDef
	if in.PrimaryType != "" {
		primary, err = s.GetAgentType(in.PrimaryType)
		if err != nil {
			return RuntimeResolution{}, fmt.Errorf("resolve type: %w", err)
		}
	}
	modelID, modelName := s.DefaultModeModel(in.AgentID, modeID)
	return RuntimeResolution{
		DefaultModeTypeID:    modeID,
		Capabilities:         caps,
		Settings:             settings,
		Definitions:          defs,
		Type:                 primary,
		DefaultModeModelID:   modelID,
		DefaultModeModelName: modelName,
	}, nil
}

// DefaultModeModel answers the model one agent's mode runs with. The override
// names it when it does; otherwise the type's default binding decides.
func (s *Store) DefaultModeModel(agentID, typeID string) (int64, string) {
	var modelID int64
	if ov, err := s.GetAgentTypeOverride(agentID, typeID); err == nil && ov != nil && ov.ModelID != 0 {
		modelID = s.EffectiveModelIDForType(typeID, ov.ModelID)
	} else {
		modelID = s.DefaultModelForType(typeID)
	}
	if modelID == 0 {
		return 0, ""
	}
	if m, err := s.GetModel(modelID); err == nil && m != nil {
		return modelID, m.Name
	}
	return modelID, ""
}

// DefaultModeTypeID is the mode a turn starts in: the agent's default session type,
// else its primary type, else the platform default. The bus used to compute this
// from the agent row; the row's facts arrive in the input now.
func (in RuntimeInput) DefaultModeTypeID() string {
	if in.DefaultType != "" {
		return in.DefaultType
	}
	if in.PrimaryType != "" {
		return in.PrimaryType
	}
	return DefaultAgentTypeID
}

// ResolveModeCapabilities returns the catalogue snapshot for one (agent, mode):
// builtin tools, MCP servers and skills visible to the owner, the models they may
// use, and that mode's resolved attachments and permissions.
func (s *Store) ResolveModeCapabilities(agentID, userID, typeID string) (*nanoprotocol.CapabilitiesResponse, error) {
	builtin, err := s.ListBuiltinTools()
	if err != nil {
		return nil, err
	}
	mcp, err := s.ListMcpServers(userID)
	if err != nil {
		return nil, err
	}
	skills, err := s.ListSkills(userID)
	if err != nil {
		return nil, err
	}
	models, err := s.ListVisibleModels(userID)
	if err != nil {
		return nil, err
	}
	at, err := s.GetAgentType(typeID)
	if err != nil {
		return nil, err
	}
	ov, err := s.GetAgentTypeOverride(agentID, typeID)
	if err != nil {
		return nil, err
	}
	var permRules []nanoprotocol.PermissionRule
	if at != nil {
		// Compose before mergeOverride folds the override into at.Permissions, so
		// the override lands last (last-match-wins) rather than being defeated by a
		// per-capability rule.
		permRules = effectivePermissions(at, ov)
		mergeOverride(at, ov)
	}

	resp := &nanoprotocol.CapabilitiesResponse{
		BuiltinTools: builtin,
		Models:       models,
		AgentType:    at,
		Permissions:  permRules,
	}
	if ov != nil {
		resp.AgentPermissions = ov.Permissions
		resp.Attachments = append(resp.Attachments, ov.Capabilities...)
	}
	for _, m := range mcp {
		resp.McpServers = append(resp.McpServers, nanoprotocol.McpServerDef{
			ID:             m.ID,
			Scope:          m.Scope,
			OwnerUserID:    m.OwnerUserID,
			Name:           m.Name,
			Description:    m.Description,
			Transport:      m.Transport,
			URL:            m.URL,
			Headers:        m.Headers,
			DefaultEnabled: m.DefaultEnabled,
			Locked:         m.Locked,
		})
	}
	for _, sk := range skills {
		def := nanoprotocol.SkillDef{
			ID:               sk.ID,
			Scope:            sk.Scope,
			OwnerUserID:      sk.OwnerUserID,
			Name:             sk.Name,
			Description:      sk.Description,
			Files:            sk.Files,
			DefaultEnabled:   sk.DefaultEnabled,
			Locked:           sk.Locked,
			Origin:           sk.Origin,
			Version:          sk.Version,
			Status:           sk.Status,
			PublishedVersion: sk.PublishedVersion,
			ContentHash:      sk.ContentHash,
			CreatedBy:        sk.CreatedBy,
		}
		if sk.Scope == "platform" {
			// One read per platform skill instead of the bus's page-wide map: the
			// platform's own skills are a handful, and a miss is not an error.
			acked, ackErr := s.SkillAckVersion(userID, sk.ID)
			if ackErr == nil {
				def.AckedVersion = acked
				if acked > 0 && sk.Status == "published" && sk.PublishedVersion > acked {
					def.UpdateAvailable = true
				}
			}
		}
		resp.Skills = append(resp.Skills, def)
	}
	return resp, nil
}

// effectivePermissions composes a definition's rules in last-match-wins order: the
// type baseline, then each capability's rules, then the user's per-(agent, mode)
// override. at must still carry its un-merged baseline.
func effectivePermissions(at *nanoprotocol.AgentTypeDef, ov *nanoprotocol.AgentTypeOverride) []nanoprotocol.PermissionRule {
	if at == nil {
		return nil
	}
	rules := append([]nanoprotocol.PermissionRule{}, at.Permissions...)
	for _, c := range at.Capabilities {
		rules = append(rules, c.Permissions...)
	}
	if ov != nil {
		rules = append(rules, ov.Permissions...)
	}
	return rules
}

// effectiveSettings is the type baseline overlaid by the override; empty values are
// dropped so the runtime falls back to its own defaults.
func (s *Store) effectiveSettings(in RuntimeInput) (map[string]string, error) {
	out := map[string]string{}
	typeID := in.PrimaryType
	if typeID == "" {
		typeID = DefaultAgentTypeID
	}
	t, err := s.GetAgentType(typeID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		// A missing type yields no settings *and* drops the override, which is what
		// the LEFT JOIN did when its driving row was absent.
		return out, nil
	}
	ov, err := s.GetAgentTypeOverride(in.AgentID, typeID)
	if err != nil {
		return nil, err
	}
	merge := func(m map[string]string) {
		for k, v := range m {
			if v != "" {
				out[k] = v
			}
		}
	}
	merge(t.Settings)
	if ov != nil {
		merge(ov.Settings)
	}
	return out, nil
}

// resolveDefinitions is every type one agent runs: its session modes in order, its
// primary type, and the helper definitions (subagent profiles) it may delegate to.
// Session modes share one head prompt — the mode's own prompt becomes the per-turn
// mode note — so a mode switch does not change the cached request prefix.
func (s *Store) resolveDefinitions(in RuntimeInput) ([]nanoprotocol.AgentTypeDef, error) {
	seen := map[string]bool{}
	out := []nanoprotocol.AgentTypeDef{}
	add := func(t *nanoprotocol.AgentTypeDef) error {
		if t == nil || t.ID == "" || seen[t.ID] {
			return nil
		}
		seen[t.ID] = true
		if err := s.applyOverride(in.AgentID, t); err != nil {
			return err
		}
		if IsSessionType(*t) {
			head, err := s.EffectiveSessionHead(in.AgentID)
			if err != nil {
				return err
			}
			if head != "" {
				if t.Settings == nil {
					t.Settings = map[string]string{}
				}
				t.Settings[SessionModeNoteSettingKey] = t.Prompt
				t.Prompt = head
			}
		}
		out = append(out, *t)
		return nil
	}

	for _, id := range in.SessionTypes {
		t, err := s.GetAgentType(id)
		if err != nil {
			return nil, err
		}
		if err := add(t); err != nil {
			return nil, err
		}
	}
	var primary *nanoprotocol.AgentTypeDef
	if in.PrimaryType != "" {
		var err error
		primary, err = s.GetAgentType(in.PrimaryType)
		if err != nil {
			return nil, err
		}
	}
	if err := add(primary); err != nil {
		return nil, err
	}
	all, err := s.ListAgentTypes(false)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if isHelperDefinition(all[i]) {
			if err := add(&all[i]); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
