package server

// The container control plane.
//
// Restarting, disposing and reprovisioning are the console's Operations actions,
// and the target state puts them in this module: the path names the owner
// (/api/v1/admin/containers/…) and this service is the one that checks the
// browser's role. Each action is a desired-state write on the queue's owner (the
// provisioner), which then executes it in its poll loop — nothing about it goes
// through the agent container's tunnel, so it works while the container is
// unhealthy.
//
// The old console prefix (/api/admin/v1/containers/…) answers with the same
// handlers: tabs opened before a deploy keep working, and nginx can move the
// route without a front-end change.

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"gitee.com/kobeyoung81/nanoadmin/internal/auth"
	"gitee.com/kobeyoung81/nanoadmin/internal/provisionerclient"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
)

// containerAction is one of the three lifecycle operations.
type containerAction struct {
	name   string // path segment and audit verb
	status string // the state the queue moves to
	call   func(*provisionerclient.Client, context.Context, string) error
}

var containerActions = map[string]containerAction{
	"restart": {
		name: "restart", status: "restarting",
		call: (*provisionerclient.Client).Restart,
	},
	"dispose": {
		name: "dispose", status: "disposing",
		call: (*provisionerclient.Client).Dispose,
	},
	"reprovision": {
		name: "reprovision", status: "pending",
		call: (*provisionerclient.Client).Reprovision,
	},
}

// registerContainerRoutes mounts the three actions under one prefix.
func (s *Server) registerContainerRoutes(mux *http.ServeMux, prefix string) {
	for name := range containerActions {
		action := containerActions[name]
		mux.HandleFunc("POST "+prefix+"/containers/{userID}/"+name, func(w http.ResponseWriter, r *http.Request) {
			s.runContainerAction(w, r, action)
		})
	}
}

// runContainerAction asks the queue's owner to move one user's container into
// the action's state, then records who asked.
func (s *Server) runContainerAction(w http.ResponseWriter, r *http.Request, action containerAction) {
	userID := r.PathValue("userID")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "userID is required"})
		return
	}
	if s.provisioner == nil || !s.provisioner.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "the provisioner module is not configured"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	err := action.call(s.provisioner, ctx, userID)
	cancel()
	if err != nil {
		code := provisionerclient.CodeOf(err)
		switch {
		case code >= 400 && code < 500:
			// The owner's own verdict: 404 means the user has no container
			// request at all (a missing row, not a gateway failure), and 409
			// means the current state does not allow the action.
			writeJSON(w, code, map[string]string{"error": err.Error()})
		default:
			slog.Error("container action failed", "action", action.name, "user_id", userID, "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "the provisioner is unavailable"})
		}
		return
	}

	s.recordContainerAudit(r, action, userID)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": action.status})
}

// recordContainerAudit writes the trail for a browser-initiated action. A
// service caller (the portal restarting a user's own container) keeps that
// responsibility itself: it knows the identity, and it already appends through
// the internal audit endpoint — recording here as well would double the entry
// with a "system" actor. A failed append never fails the action: it happened.
func (s *Server) recordContainerAudit(r *http.Request, action containerAction, userID string) {
	if s.audit == nil {
		return
	}
	identity, ok := auth.FromContext(r.Context())
	if !ok {
		return
	}
	actor, role := identity.UserID, identity.Role
	if err := s.audit.RecordAudit(store.AuditEntry{
		ActorID:      actor,
		ActorRole:    role,
		Action:       "container." + action.name,
		ResourceType: "container",
		ResourceID:   userID,
		IP:           clientIP(r),
	}); err != nil {
		slog.Warn("record container audit", "action", action.name, "user_id", userID, "error", err)
	}
}
