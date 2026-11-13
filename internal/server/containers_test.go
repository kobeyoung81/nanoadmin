package server

// The container control plane: the console's three lifecycle actions are served
// by this module, which checks the caller's role and asks the queue's owner to
// move the container. What must hold: the path maps to the right desired state,
// the owner's 404 is passed through (a missing row, not a gateway failure), an
// unconfigured owner fails loudly, and the trail records who asked.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gitee.com/kobeyoung81/nanoadmin/internal/auth"
	"gitee.com/kobeyoung81/nanoadmin/internal/config"
	"gitee.com/kobeyoung81/nanoadmin/internal/provisionerclient"
	"gitee.com/kobeyoung81/nanoadmin/internal/store"
)

type recordedAudit struct {
	mu      sync.Mutex
	entries []store.AuditEntry
}

func (r *recordedAudit) RecordAudit(entry store.AuditEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entry)
	return nil
}

func (r *recordedAudit) all() []store.AuditEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]store.AuditEntry(nil), r.entries...)
}

func TestContainerActionsReachTheProvisioner(t *testing.T) {
	var calls []string
	provisioner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer provisioner.Close()

	s := New(config.Config{}, fakeStore{}).WithProvisioner(provisionerclient.New(provisioner.URL, "tok"))
	audit := &recordedAudit{}
	s.audit = audit

	for _, tc := range []struct {
		action string
		want   string
		status string
	}{
		{"restart", "POST /api/v1/internal/containers/alice/restart", "restarting"},
		{"dispose", "POST /api/v1/internal/containers/alice/dispose", "disposing"},
		{"reprovision", "POST /api/v1/internal/containers/alice/reprovision", "pending"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			calls = nil
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/containers/alice/"+tc.action, strings.NewReader(`{}`))
			req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{UserID: "support-1", Role: auth.RoleSupport}))
			req.SetPathValue("userID", "alice")
			rec := httptest.NewRecorder()
			s.runContainerAction(rec, req, containerActions[tc.action])

			if rec.Code != http.StatusAccepted {
				t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
			}
			if len(calls) != 1 || calls[0] != tc.want {
				t.Fatalf("calls = %v, want %q", calls, tc.want)
			}
			if !strings.Contains(rec.Body.String(), tc.status) {
				t.Fatalf("body = %s, want status %q", rec.Body.String(), tc.status)
			}
		})
	}

	entries := audit.all()
	if len(entries) != 3 {
		t.Fatalf("audit entries = %d, want one per action", len(entries))
	}
	if entries[0].Action != "container.restart" || entries[0].ActorID != "support-1" || entries[0].ResourceID != "alice" {
		t.Fatalf("audit entry = %+v", entries[0])
	}
}

// TestContainerActionPassesAMissingRowThrough: the owner answers 404 when the
// user has no container request; the console shows that as a missing row.
func TestContainerActionPassesAMissingRowThrough(t *testing.T) {
	provisioner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"container request not found"}`, http.StatusNotFound)
	}))
	defer provisioner.Close()

	s := New(config.Config{}, fakeStore{}).WithProvisioner(provisionerclient.New(provisioner.URL, ""))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/containers/missing/dispose", nil)
	req.SetPathValue("userID", "missing")
	rec := httptest.NewRecorder()
	s.runContainerAction(rec, req, containerActions["dispose"])

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

// TestContainerActionWithoutAProvisionerFails: no configured owner means the
// action is refused, not silently accepted.
func TestContainerActionWithoutAProvisionerFails(t *testing.T) {
	s := New(config.Config{}, fakeStore{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/containers/alice/restart", nil)
	req.SetPathValue("userID", "alice")
	rec := httptest.NewRecorder()
	s.runContainerAction(rec, req, containerActions["restart"])

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

// TestContainerActionNeedsAUserID keeps an empty path segment from reaching the
// owner as "the container with an empty id".
func TestContainerActionNeedsAUserID(t *testing.T) {
	s := New(config.Config{}, fakeStore{}).WithProvisioner(provisionerclient.New("http://127.0.0.1:1", ""))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/containers//restart", nil)
	rec := httptest.NewRecorder()
	s.runContainerAction(rec, req, containerActions["restart"])

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestContainerRoutesExistUnderBothPrefixes pins the alias: the console path of
// the target state and the historical one reach the same handler.
func TestContainerRoutesExistUnderBothPrefixes(t *testing.T) {
	s := New(config.Config{InternalToken: "s3cret"}, fakeStore{})
	h := s.Handler()

	for _, prefix := range []string{"/api/v1/admin", "/api/admin/v1"} {
		req := httptest.NewRequest(http.MethodPost, prefix+"/containers/alice/restart", nil)
		req.Header.Set("Authorization", "Bearer s3cret")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		// 503 is the handler's "no owner configured": the route is there.
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: status = %d, want the handler's 503 (%s)", prefix, rec.Code, rec.Body.String())
		}
	}
}
