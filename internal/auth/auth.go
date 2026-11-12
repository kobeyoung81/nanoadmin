// Package auth lets nanoadmin verify browser requests itself.
//
// Until now the console was the only caller and the portal was the
// authentication boundary: it validated the JWT, checked the role, and told
// this service which user a call acted for. The target state has nginx route
// browser traffic straight here, so the same three checks have to exist in this
// process: verify the signature against nanoauth's JWKS, decide whether the
// role may use the catalogue, and take the acting user from the token rather
// than from a query parameter.
package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Roles mirror the platform's RBAC model. They are strings on the token; this
// package deliberately does not import the portal's copy (the two services are
// independent, and a shared constant here would mean a shared module).
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleSupport  = "support"
	RoleAuditor  = "auditor"
	RoleUser     = "user"
)

var knownRoles = map[string]struct{}{
	RoleAdmin:    {},
	RoleOperator: {},
	RoleSupport:  {},
	RoleAuditor:  {},
	RoleUser:     {},
}

// Identity is the authenticated caller of a browser request.
type Identity struct {
	UserID string
	Role   string
}

type contextKey string

const (
	identityKey contextKey = "admin-identity"
	serviceKey  contextKey = "admin-service-caller"
)

// WithIdentity stores the caller in the request context.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// FromContext returns the caller, if the request carried a valid token.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey).(Identity)
	return id, ok
}

// WithServiceCaller marks a request authenticated by the shared internal token.
// Those callers are other modules (the portal, the agent, nanoauth); they have
// already checked their own users, they act for whoever they name, and they
// need unmasked values to do their job.
func WithServiceCaller(ctx context.Context) context.Context {
	return context.WithValue(ctx, serviceKey, true)
}

// IsServiceCaller reports whether the request arrived with the internal token.
func IsServiceCaller(ctx context.Context) bool {
	service, _ := ctx.Value(serviceKey).(bool)
	return service
}

// KnownRole reports whether the token carries a role this platform understands.
// An unknown role is treated as unauthenticated rather than as "some user".
func KnownRole(role string) bool {
	_, ok := knownRoles[role]
	return ok
}

// MayUseCatalogue is the Management section's policy: configuration, models,
// agent types, prompts, capabilities, permission keys, alert rules. It matches
// the portal's SectionRoles so a direct call cannot do more than a proxied one.
func MayUseCatalogue(role string) bool {
	return role == RoleAdmin || role == RoleOperator
}

// MayReadAudit is the Audit section's policy.
func MayReadAudit(role string) bool {
	return role == RoleAdmin || role == RoleAuditor
}

// Validator checks nanoauth-issued RS256 tokens against its JWKS endpoint.
type Validator struct {
	jwksURL   string
	client    *http.Client
	keyCache  map[string]*rsa.PublicKey
	lastFetch time.Time
	cacheTTL  time.Duration
	mu        sync.RWMutex
}

// NewValidator returns a validator, or nil when no JWKS URL is configured —
// in that case only the internal token can be used, which is what a deployment
// that has not rolled the console over yet needs.
func NewValidator(jwksURL string) *Validator {
	if strings.TrimSpace(jwksURL) == "" {
		return nil
	}
	return &Validator{
		jwksURL:  jwksURL,
		client:   &http.Client{Timeout: 10 * time.Second},
		keyCache: make(map[string]*rsa.PublicKey),
		cacheTTL: 5 * time.Minute,
	}
}

// Validate parses and verifies a token and returns the caller's identity.
func (v *Validator) Validate(tokenStr string) (Identity, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing kid in token header")
		}
		return v.fetchKey(kid)
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		return Identity{}, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return Identity{}, fmt.Errorf("invalid token")
	}
	userID, _ := claims["sub"].(string)
	if strings.TrimSpace(userID) == "" {
		return Identity{}, fmt.Errorf("token has no subject")
	}
	role, _ := claims["role"].(string)
	if role == "" {
		role = RoleUser
	}
	if !KnownRole(role) {
		return Identity{}, fmt.Errorf("token carries an unknown role %q", role)
	}
	return Identity{UserID: userID, Role: role}, nil
}

func (v *Validator) fetchKey(kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	if key, ok := v.keyCache[kid]; ok && time.Since(v.lastFetch) < v.cacheTTL {
		v.mu.RUnlock()
		return key, nil
	}
	v.mu.RUnlock()

	v.mu.Lock()
	defer v.mu.Unlock()

	if key, ok := v.keyCache[kid]; ok && time.Since(v.lastFetch) < v.cacheTTL {
		return key, nil
	}

	resp, err := v.client.Get(v.jwksURL)
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch jwks: HTTP %d", resp.StatusCode)
	}

	var jwks struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("decode jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, key := range jwks.Keys {
		pub, err := parseJWKKey(key.Kty, key.N, key.E)
		if err != nil {
			continue
		}
		keys[key.Kid] = pub
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("jwks returned no usable keys")
	}
	v.keyCache = keys
	v.lastFetch = time.Now()

	if key, ok := v.keyCache[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("no matching key in jwks for kid %q", kid)
}

func parseJWKKey(kty, n, e string) (*rsa.PublicKey, error) {
	if kty != "RSA" {
		return nil, fmt.Errorf("unsupported kty %q", kty)
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(n)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(e)
	if err != nil {
		return nil, err
	}
	if len(eBytes) == 0 {
		eBytes = []byte{0}
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}, nil
}
