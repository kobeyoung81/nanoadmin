package server

// The admin module used to trust whoever reached it, because the portal was the
// only door and did the authentication. With nginx routing browser traffic here
// the same three checks (signature, role, acting user) have to hold in this
// process, and a JWT must not buy more than the console would have granted.

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitee.com/kobeyoung81/nanoadmin/internal/auth"
	"gitee.com/kobeyoung81/nanoadmin/internal/config"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

const testKID = "test-key"

// jwksServer publishes the public half of key so the validator fetches it the
// way it will fetch nanoauth's.
func jwksServer(t *testing.T, key *rsa.PrivateKey) *httptest.Server {
	t.Helper()
	payload := map[string]any{"keys": []map[string]string{{
		"kty": "RSA",
		"kid": testKID,
		"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func signToken(t *testing.T, key *rsa.PrivateKey, userID, role string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": userID,
		"rol": "ignored",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	if role != "" {
		claims["role"] = role
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testKID
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func newAuthServer(t *testing.T) (*Server, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	srv := jwksServer(t, key)
	s := New(config.Config{InternalToken: "s3cret", JWKSURL: srv.URL}, fakeStore{})
	return s, key
}

// echoHandlers records what the middleware decided, so the policy can be tested
// without depending on a handler's data.
func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.FromContext(r.Context())
		_, _ = fmt.Fprintf(w, "identity=%s role=%s service=%v ok=%v",
			id.UserID, id.Role, auth.IsServiceCaller(r.Context()), ok)
	})
}

func doAuthRequest(t *testing.T, s *Server, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.authenticate(echoHandler()).ServeHTTP(rec, req)
	return rec
}

func TestAuthenticate_NoCredentialsIs401(t *testing.T) {
	s, _ := newAuthServer(t)

	if rec := doAuthRequest(t, s, http.MethodGet, "/api/v1/agent-types", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if rec := doAuthRequest(t, s, http.MethodGet, "/api/v1/agent-types", "not-a-token"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("garbage token: status = %d, want 401", rec.Code)
	}
}

// TestAuthenticate_PlainUserCannotUseTheCatalogue is the acceptance line: a
// signed-in user is a valid caller, but the catalogue is the Management
// section's (admin, operator) — otherwise a direct entry would be a privilege
// escalation.
func TestAuthenticate_PlainUserCannotUseTheCatalogue(t *testing.T) {
	s, key := newAuthServer(t)
	token := signToken(t, key, "user-1", auth.RoleUser)

	if rec := doAuthRequest(t, s, http.MethodGet, "/api/v1/agent-types", token); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if rec := doAuthRequest(t, s, http.MethodGet, "/api/v1/config/agent", token); rec.Code != http.StatusForbidden {
		t.Fatalf("config read: status = %d, want 403", rec.Code)
	}
}

func TestAuthenticate_AdminAndOperatorReachTheCatalogue(t *testing.T) {
	s, key := newAuthServer(t)

	for _, role := range []string{auth.RoleAdmin, auth.RoleOperator} {
		rec := doAuthRequest(t, s, http.MethodGet, "/api/v1/agent-types", signToken(t, key, "u-"+role, role))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", role, rec.Code)
		}
		if want := "role=" + role; !stringsContains(rec.Body.String(), want) {
			t.Fatalf("%s: the identity did not reach the handler: %q", role, rec.Body.String())
		}
	}
}

func TestAuthenticate_AuditorReadsTheAuditOnly(t *testing.T) {
	s, key := newAuthServer(t)
	token := signToken(t, key, "auditor-1", auth.RoleAuditor)

	if rec := doAuthRequest(t, s, http.MethodGet, "/api/v1/audit", token); rec.Code != http.StatusOK {
		t.Fatalf("audit read: status = %d, want 200", rec.Code)
	}
	if rec := doAuthRequest(t, s, http.MethodGet, "/api/v1/config/agent", token); rec.Code != http.StatusForbidden {
		t.Fatalf("config read: status = %d, want 403", rec.Code)
	}
}

// TestAuthenticate_InternalOperationsAreNotABrowserSurface: a token must never
// reach /api/v1/internal/*, whatever its role. Those endpoints carry no user
// semantics and skip every check the handlers expect a person behind.
func TestAuthenticate_InternalOperationsAreNotABrowserSurface(t *testing.T) {
	s, key := newAuthServer(t)

	rec := doAuthRequest(t, s, http.MethodPost, "/api/v1/internal/audit", signToken(t, key, "admin-1", auth.RoleAdmin))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAuthenticate_UserCatalogueAcceptsAnySignedInAccount(t *testing.T) {
	s, key := newAuthServer(t)

	rec := doAuthRequest(t, s, http.MethodGet, "/api/v1/user/models?user_id=someone-else", signToken(t, key, "user-1", auth.RoleUser))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if want := "identity=user-1"; !stringsContains(rec.Body.String(), want) {
		t.Fatalf("the identity did not reach the handler: %q", rec.Body.String())
	}
}

func TestAuthenticate_ServiceTokenStillWorksEverywhere(t *testing.T) {
	s, _ := newAuthServer(t)

	for _, path := range []string{"/api/v1/agent-types", "/api/v1/internal/audit"} {
		rec := doAuthRequest(t, s, http.MethodGet, path, "s3cret")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", path, rec.Code)
		}
		if want := "service=true"; !stringsContains(rec.Body.String(), want) {
			t.Fatalf("%s: not marked as a service caller: %q", path, rec.Body.String())
		}
	}
}

// TestActingUser_PrefersTheTokenOverTheQuery is the "?user_id= cannot escalate"
// acceptance: a signed-in caller naming someone else is ignored.
func TestActingUser_PrefersTheTokenOverTheQuery(t *testing.T) {
	s := New(config.Config{}, fakeStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/models?user_id=someone-else", nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{UserID: "user-1", Role: auth.RoleUser}))
	if got := s.actingUser(req); got != "user-1" {
		t.Fatalf("actingUser = %q, want the token subject", got)
	}

	// A service caller names the user explicitly; that path stays.
	serviceReq := httptest.NewRequest(http.MethodGet, "/api/v1/user/models?user_id=user-2", nil)
	serviceReq = serviceReq.WithContext(auth.WithServiceCaller(serviceReq.Context()))
	if got := s.actingUser(serviceReq); got != "user-2" {
		t.Fatalf("actingUser = %q, want the named user for a service caller", got)
	}
}

// TestConfigReads_MaskSecretsForBrowsers pins both halves: a browser never sees
// a secret value, and a service caller still does (nanoauth and the agent read
// the JWT keypair to work at all).
func TestConfigReads_MaskSecretsForBrowsers(t *testing.T) {
	st := fakeStore{
		items: []store.ConfigItem{
			{Key: "jwtPrivateKey", Value: "pem-secret", ValueType: "secret"},
			{Key: "jwksUrl", Value: "http://nanoauth/jwks", ValueType: "string"},
		},
		single:   store.ConfigItem{Key: "jwtPrivateKey", Value: "pem-secret", ValueType: "secret"},
		singleOK: true,
	}
	browser := New(config.Config{InternalToken: "s3cret"}, st)

	// Browser read (masked unconditionally). The handler is called directly so
	// the test isolates masking from token verification, which has its own tests.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/auth", nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{UserID: "admin-1", Role: auth.RoleAdmin}))
	rec := httptest.NewRecorder()
	browser.restGetConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var payload struct {
		Items []store.ConfigItem `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Items[0].Value != maskValue || payload.Items[1].Value == maskValue {
		t.Fatalf("masking is wrong: %+v", payload.Items)
	}
}

// TestConfigReads_TheConsoleAsksForTheMask: the portal still holds the service
// token, so it asks for the masked view explicitly on the console's behalf.
func TestConfigReads_TheConsoleAsksForTheMask(t *testing.T) {
	st := fakeStore{items: []store.ConfigItem{{Key: "jwtPrivateKey", Value: "pem-secret", ValueType: "secret"}}}
	handler := New(config.Config{InternalToken: "s3cret"}, st).Handler()

	for _, tc := range []struct {
		path string
		want string
	}{
		{"/api/v1/config/auth", "pem-secret"},
		{"/api/v1/config/auth?mask=1", maskValue},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Authorization", "Bearer s3cret")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if !stringsContains(rec.Body.String(), `"value":"`+tc.want+`"`) {
			t.Fatalf("%s: %q", tc.path, rec.Body.String())
		}
	}

	// Without the flag a service caller sees the real value.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/auth", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !stringsContains(rec.Body.String(), `"value":"pem-secret"`) {
		t.Fatalf("a service caller was masked: %q", rec.Body.String())
	}
}

// TestConfigWrites_DoNotStoreTheMask: a form that read "***" sends it back
// unchanged on every other field's save; writing it would destroy the key.
func TestConfigWrites_DoNotStoreTheMask(t *testing.T) {
	recording := &recordingStore{fakeStore: fakeStore{}}
	handler := New(config.Config{InternalToken: "s3cret"}, recording).Handler()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/config/auth/jwtPrivateKey", stringsReader(`{"value":"***"}`))
	req.Header.Set("Authorization", "Bearer s3cret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if recording.written {
		t.Fatal(`the mask was written as the value`)
	}
	if !stringsContains(rec.Body.String(), `"unchanged":true`) {
		t.Fatalf("the answer does not say it was left alone: %q", rec.Body.String())
	}
}

type recordingStore struct {
	fakeStore
	written bool
}

func (r *recordingStore) SetConfigItemTyped(_, _, _, _, _ string) error {
	r.written = true
	return nil
}

func stringsContains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

func stringsReader(s string) *strings.Reader { return strings.NewReader(s) }

// TestAuthenticate_CanonicalConsolePrefixKeepsTheSamePolicy: /api/v1/admin/ is
// the target-state console namespace. It must carry exactly the policy of the
// historical /api/admin/v1/, including the auditor's audit-only rule — a page
// that moves prefixes must not silently change who may call it.
func TestAuthenticate_CanonicalConsolePrefixKeepsTheSamePolicy(t *testing.T) {
	s, key := newAuthServer(t)

	if rec := doAuthRequest(t, s, http.MethodGet, "/api/v1/admin/configs", signToken(t, key, "admin-1", auth.RoleAdmin)); rec.Code != http.StatusOK {
		t.Fatalf("admin console: status = %d, want 200", rec.Code)
	}
	if rec := doAuthRequest(t, s, http.MethodGet, "/api/v1/admin/configs", signToken(t, key, "auditor-1", auth.RoleAuditor)); rec.Code != http.StatusForbidden {
		t.Fatalf("auditor console: status = %d, want 403", rec.Code)
	}
	if rec := doAuthRequest(t, s, http.MethodGet, "/api/v1/admin/configs", signToken(t, key, "user-1", auth.RoleUser)); rec.Code != http.StatusForbidden {
		t.Fatalf("user console: status = %d, want 403", rec.Code)
	}
	if rec := doAuthRequest(t, s, http.MethodGet, "/api/v1/admin/audit", signToken(t, key, "auditor-1", auth.RoleAuditor)); rec.Code != http.StatusOK {
		t.Fatalf("auditor audit: status = %d, want 200", rec.Code)
	}
}
