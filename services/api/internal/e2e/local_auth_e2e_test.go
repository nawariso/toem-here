package e2e_test

import (
	"net/http"
	"testing"

	"github.com/nawariso/toem-here/services/api/internal/application"
	"github.com/nawariso/toem-here/services/api/internal/infrastructure/config"
	"github.com/nawariso/toem-here/services/api/internal/infrastructure/identity"
	persistence "github.com/nawariso/toem-here/services/api/internal/infrastructure/persistence/postgres"
	"github.com/nawariso/toem-here/services/api/internal/testsupport"
	httptransport "github.com/nawariso/toem-here/services/api/internal/transport/http"
)

// newLocalStack wires the real HTTP server, application service, and
// PostgreSQL repository behind the verifier selected by AUTH_MODE=local.
func newLocalStack(t *testing.T) *stack {
	t.Helper()
	pool := testsupport.Database(t)
	verifier, err := identity.FromConfig(config.Config{
		AppEnv: config.EnvTest, AuthMode: config.AuthModeLocal, HTTPPort: "8080", DatabaseURL: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := persistence.NewRepository(pool)
	api := httptransport.NewServer(application.NewUserService(repo), verifier, repo).Handler()
	return &stack{api: api, pool: pool}
}

// Requirement 001-B §14/§34: first local login creates User + AuthIdentity +
// USER role; repeated local bootstrap resolves to the same internal user.
func TestLocalAuthBootstrapsOneStableInternalUser(t *testing.T) {
	s := newLocalStack(t)

	status, first := s.call(t, http.MethodPost, "/v1/auth/bootstrap", identity.LocalDevCredential, nil)
	if status != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%v", status, first)
	}
	internalID, _ := first["id"].(string)
	if internalID == "" || internalID == identity.LocalDevSubject {
		t.Fatalf("bootstrap must return a server-generated internal id, got %v", first["id"])
	}
	if roles, _ := first["roles"].([]any); len(roles) != 1 || roles[0] != "USER" {
		t.Fatalf("expected the USER role, got %v", first["roles"])
	}
	if first["profileComplete"] != false {
		t.Fatalf("new local user must start with an incomplete passport: %v", first)
	}

	for range 3 {
		status, again := s.call(t, http.MethodPost, "/v1/auth/bootstrap", identity.LocalDevCredential, nil)
		if status != http.StatusOK || again["id"] != internalID {
			t.Fatalf("repeat local login must reuse the internal user: status=%d body=%v", status, again)
		}
	}

	var users, identities int
	var provider, subject string
	if err := s.pool.QueryRow(t.Context(), "SELECT count(*) FROM users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(t.Context(), "SELECT count(*), min(provider), min(provider_subject) FROM auth_identities").Scan(&identities, &provider, &subject); err != nil {
		t.Fatal(err)
	}
	if users != 1 || identities != 1 {
		t.Fatalf("repeat local login duplicated rows: users=%d identities=%d", users, identities)
	}
	if provider != "LOCAL_DEV" || subject != "developer-001" {
		t.Fatalf("auth identity must record the local provider and subject, got %s/%s", provider, subject)
	}

	status, updated := s.call(t, http.MethodPatch, "/v1/users/me", identity.LocalDevCredential,
		map[string]string{"username": "dev_user", "displayName": "Dev User"})
	if status != http.StatusOK || updated["profileComplete"] != true || updated["id"] != internalID {
		t.Fatalf("passport setup through local auth failed: status=%d body=%v", status, updated)
	}
}

// Requirement 001-B §9: even in local mode the client cannot assert identity.
func TestLocalAuthDoesNotTrustClientSuppliedIdentity(t *testing.T) {
	s := newLocalStack(t)
	_, first := s.call(t, http.MethodPost, "/v1/auth/bootstrap", identity.LocalDevCredential, nil)

	for name, token := range map[string]string{
		"missing":         "",
		"subject_only":    identity.LocalDevSubject,
		"other_developer": "toem-local-dev.developer-002",
		"user_id":         first["id"].(string),
		"garbage":         "not-a-credential",
	} {
		t.Run(name, func(t *testing.T) {
			if status, _ := s.call(t, http.MethodGet, "/v1/users/me", token, nil); status != http.StatusUnauthorized {
				t.Fatalf("expected 401 for %s, got %d", name, status)
			}
		})
	}

	for _, payload := range []map[string]any{
		{"roles": []string{"ADMIN"}},
		{"status": "SUSPENDED"},
		{"id": "00000000-0000-0000-0000-000000000000"},
		{"userId": "00000000-0000-0000-0000-000000000000"},
	} {
		if status, _ := s.call(t, http.MethodPatch, "/v1/users/me", identity.LocalDevCredential, payload); status != http.StatusBadRequest {
			t.Fatalf("local client must not send %v (status=%d)", payload, status)
		}
	}
	var elevated int
	if err := s.pool.QueryRow(t.Context(), "SELECT count(*) FROM user_roles WHERE role <> 'USER'").Scan(&elevated); err != nil {
		t.Fatal(err)
	}
	if elevated != 0 {
		t.Fatalf("client escalated roles in local mode: %d", elevated)
	}
}

// Requirement 001-B §10/§34: the development credential has no authority on a
// server running the Supabase JWT verifier.
func TestSupabaseStackRejectsLocalDevelopmentCredential(t *testing.T) {
	s := newStack(t)
	if status, _ := s.call(t, http.MethodPost, "/v1/auth/bootstrap", identity.LocalDevCredential, nil); status != http.StatusUnauthorized {
		t.Fatalf("supabase-mode API accepted the local credential: status=%d", status)
	}
	var users int
	if err := s.pool.QueryRow(t.Context(), "SELECT count(*) FROM users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 0 {
		t.Fatalf("rejected credential created %d users", users)
	}
}
