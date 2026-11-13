package server

import (
	"net/http"
	"strings"

	"gitee.com/kobeyoung81/nanoadmin/internal/auth"
)

// authenticate guards the whole API. Two kinds of caller exist:
//
//   - the platform's own modules, holding the shared internal token: they have
//     already authorised their user, they name them explicitly, and they need
//     unmasked values;
//   - browsers, holding a nanoauth-issued JWT: verified here, and narrowed by
//     role to the same sections the portal's console gate uses.
//
// Everything else is refused. /healthz stays outside (a probe must not need a
// secret), and /api/v1/internal/* accepts the internal token only: those are
// service-to-service operations with no browser meaning.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /healthz is the only open path; both API namespaces are guarded — the
		// module API (/api/v1/, which also contains /api/v1/admin/) and the
		// console's historical prefix (/api/admin/v1/).
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") && !strings.HasPrefix(r.URL.Path, "/api/admin/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		if s.key.Allow(r) {
			next.ServeHTTP(w, r.WithContext(auth.WithServiceCaller(r.Context())))
			return
		}

		// Nothing could authenticate this request: say so instead of pretending
		// the caller forgot a token.
		if s.key.key == "" && s.jwt == nil {
			writeAuthError(w, http.StatusServiceUnavailable, "authentication is not configured")
			return
		}

		// The token travels in the header only: this service has no WebSocket
		// handshake, and a token in a URL ends up in access logs.
		token := bearerToken(r)
		if s.jwt == nil || token == "" {
			writeAuthError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		identity, err := s.jwt.Validate(token)
		if err != nil {
			writeAuthError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if !mayCall(r, identity.Role) {
			writeAuthError(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithIdentity(r.Context(), identity)))
	})
}

// mayCall applies the section policy to a browser request.
func mayCall(r *http.Request, role string) bool {
	// Internal operations are never a browser action, whatever the role.
	if strings.HasPrefix(r.URL.Path, "/api/v1/internal/") {
		return false
	}
	// A user's own catalogue (their library, their models) admits any signed-in
	// account; the handlers act for the token's subject.
	if strings.HasPrefix(r.URL.Path, "/api/v1/user/") {
		return auth.KnownRole(role)
	}
	// Container lifecycle is an Operations action: support may restart or
	// dispose a container, operators manage the catalogue instead. Both console
	// prefixes carry the routes, so both carry the policy.
	if isContainerActionPath(r.URL.Path) {
		return auth.MayOperateContainers(role)
	}
	// The audit trail is readable by auditors. Both console prefixes carry it:
	// the historical /api/admin/v1/ and the target-state /api/v1/admin/.
	audit := r.URL.Path == "/api/v1/audit" ||
		r.URL.Path == "/api/admin/v1/audit" ||
		r.URL.Path == "/api/v1/admin/audit"
	if r.Method == http.MethodGet && audit {
		return auth.MayReadAudit(role)
	}
	return auth.MayUseCatalogue(role)
}

// isContainerActionPath reports whether the request is one of the console's
// container lifecycle actions.
func isContainerActionPath(path string) bool {
	rest, ok := strings.CutPrefix(path, "/api/v1/admin/containers/")
	if !ok {
		rest, ok = strings.CutPrefix(path, "/api/admin/v1/containers/")
	}
	if !ok {
		return false
	}
	// {userID}/{action}: the action is the last segment.
	return strings.HasSuffix(rest, "/restart") ||
		strings.HasSuffix(rest, "/dispose") ||
		strings.HasSuffix(rest, "/reprovision")
}

// actingUser is the user a user-facing call works for. A token's subject wins
// over the query parameter: a signed-in caller cannot name someone else, which
// is exactly the hole a direct entry would otherwise open.
func (s *Server) actingUser(r *http.Request) string {
	if identity, ok := auth.FromContext(r.Context()); ok {
		return identity.UserID
	}
	return r.URL.Query().Get("user_id")
}
