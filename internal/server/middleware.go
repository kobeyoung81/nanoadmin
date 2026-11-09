// Internal-key authentication for this service's internal endpoints.
package server

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

// InternalKey authenticates the platform's own callers with one shared token —
// the same secret the deployment already gives every service. Everything inside
// the trust domain presents it; what it buys is that this service is not open to
// anything that can reach its port.
type InternalKey struct {
	key string
	// warnOnce keeps the "not configured" error to one line per process: it is
	// a deployment mistake, not a per-request condition.
	warnOnce sync.Once
}

// NewInternalKey returns a guard for the given token. An empty token is a
// configuration mistake: the guard then refuses every protected request rather
// than silently leaving the service open.
func NewInternalKey(key string) *InternalKey {
	return &InternalKey{key: key}
}

// Allow reports whether a request carries the configured token. The token
// travels in the Authorization header; the query parameter is accepted because
// a WebSocket handshake from some clients has no other place to put it.
func (k *InternalKey) Allow(r *http.Request) bool {
	if k.key == "" {
		return false
	}
	token := bearerToken(r)
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(k.key)) == 1
}

// Guard wraps next, requiring the token for requests protected() claims.
func (k *InternalKey) Guard(next http.Handler, protected func(*http.Request) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !protected(r) {
			next.ServeHTTP(w, r)
			return
		}
		if k.key == "" {
			k.warnOnce.Do(func() {
				slog.Error("nanoadmin: internal endpoints have no token configured and are refusing requests",
					"hint", "set NANOSEC_BUS_INTERNAL_TOKEN")
			})
			writeAuthError(w, http.StatusServiceUnavailable, "authentication is not configured")
			return
		}
		if !k.Allow(r) {
			writeAuthError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	const prefix = "bearer "
	if len(value) < len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(value[len(prefix):])
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + message + `"}`))
}
