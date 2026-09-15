package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nawariso/toem-here/services/api/internal/application"
	"github.com/nawariso/toem-here/services/api/internal/domain"
	httptransport "github.com/nawariso/toem-here/services/api/internal/transport/http"
)

type verifier struct{ reject bool }

func (v verifier) Verify(_ context.Context, token string) (domain.ExternalIdentity, error) {
	if v.reject || token != "valid" {
		return domain.ExternalIdentity{}, errors.New("invalid")
	}
	return domain.ExternalIdentity{Provider: "SUPABASE", Subject: "subject"}, nil
}

type app struct{ user domain.User }

func (a app) Bootstrap(context.Context, domain.ExternalIdentity) (domain.User, error) {
	return a.user, nil
}
func (a app) Current(context.Context, domain.ExternalIdentity) (domain.User, error) {
	return a.user, nil
}
func (a app) UpdateProfile(_ context.Context, _ domain.ExternalIdentity, patch domain.ProfilePatch) (domain.User, error) {
	u := a.user
	u.Username = patch.Username
	u.DisplayName = patch.DisplayName
	if patch.Locale != nil {
		u.Locale = *patch.Locale
	}
	return u, nil
}

type ready struct{ err error }

func (r ready) Ping(context.Context) error { return r.err }

func testServer() http.Handler {
	username, name := "mickey", "Mickey"
	return httptransport.NewServer(app{user: domain.User{ID: "internal-id", Username: &username, DisplayName: &name, Locale: "th", Status: "ACTIVE", Roles: []string{"USER"}}}, verifier{}, ready{}).Handler()
}

func TestHealthAndReadiness(t *testing.T) {
	for _, path := range []string{"/health", "/ready"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		testServer().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s=%d", path, rec.Code)
		}
	}
}
func TestUsersMeRequiresAuthentication(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
	rec := httptest.NewRecorder()
	testServer().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
	var body map[string]map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"]["code"] != "UNAUTHENTICATED" || body["error"]["requestId"] == "" {
		t.Fatalf("body=%s", rec.Body.String())
	}
}
func TestUsersMeReturnsInternalUser(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer valid")
	rec := httptest.NewRecorder()
	testServer().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"internal-id"`) || !strings.Contains(rec.Body.String(), `"profileComplete":true`) {
		t.Fatal(rec.Body.String())
	}
}
func TestBootstrapIsAuthenticated(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/bootstrap", nil)
	req.Header.Set("Authorization", "Bearer valid")
	rec := httptest.NewRecorder()
	testServer().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
}
func TestProfilePatchRejectsUnknownAndProtectedFields(t *testing.T) {
	for _, payload := range []string{`{"roles":["ADMIN"]}`, `{"id":"other"}`, `{"status":"DELETED"}`, `{"unknown":true}`} {
		req := httptest.NewRequest(http.MethodPatch, "/v1/users/me", strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer valid")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		testServer().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("payload=%s status=%d body=%s", payload, rec.Code, rec.Body.String())
		}
	}
}
func TestProfilePatchAcceptsAllowedFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPatch, "/v1/users/me", strings.NewReader(`{"username":"new_name","displayName":"New Name","locale":"en"}`))
	req.Header.Set("Authorization", "Bearer valid")
	rec := httptest.NewRecorder()
	testServer().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"username":"new_name"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

var _ application.IdentityVerifier = verifier{}
