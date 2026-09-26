package e2e_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nawariso/toem-hia/services/api/internal/application"
	"github.com/nawariso/toem-hia/services/api/internal/infrastructure/identity"
	persistence "github.com/nawariso/toem-hia/services/api/internal/infrastructure/persistence/postgres"
	"github.com/nawariso/toem-hia/services/api/internal/testsupport"
	httptransport "github.com/nawariso/toem-hia/services/api/internal/transport/http"
)

const (
	issuer   = "https://project.example.supabase.co/auth/v1"
	audience = "authenticated"
)

type stack struct {
	api    http.Handler
	key    *rsa.PrivateKey
	pool   *pgxpool.Pool
	jwksed *httptest.Server
}

func newStack(t *testing.T) *stack {
	t.Helper()
	pool := testsupport.Database(t)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	n := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())
	jwksBody, _ := json.Marshal(map[string]any{"keys": []any{map[string]any{
		"kty": "RSA", "kid": "e2e-key", "use": "sig", "alg": "RS256", "n": n, "e": e,
	}}})
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksBody)
	}))

	verifier, err := identity.NewJWTVerifier(issuer, audience, jwksServer.URL, jwksServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	repo := persistence.NewRepository(pool)
	api := httptransport.NewServer(application.NewUserService(repo), verifier, repo).Handler()

	t.Cleanup(func() {
		jwksServer.Close()
	})
	return &stack{api: api, key: key, pool: pool, jwksed: jwksServer}
}

func (s *stack) token(t *testing.T, subject string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer, "aud": []string{audience}, "sub": subject,
		"email": "person@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Add(-time.Minute).Unix(),
	})
	tok.Header["kid"] = "e2e-key"
	signed, err := tok.SignedString(s.key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func (s *stack) call(t *testing.T, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.api.ServeHTTP(rec, req)
	decoded := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	return rec.Code, decoded
}

func TestEndToEndFirstLoginPassportSetupAndRepeatLogin(t *testing.T) {
	s := newStack(t)
	token := s.token(t, "external-subject-1")

	status, first := s.call(t, http.MethodPost, "/v1/auth/bootstrap", token, nil)
	if status != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%v", status, first)
	}
	if first["profileComplete"] != false || first["username"] != nil {
		t.Fatalf("new user must start with an incomplete passport: %v", first)
	}
	internalID, _ := first["id"].(string)
	if internalID == "" {
		t.Fatal("bootstrap must return an internal user id")
	}
	if roles, _ := first["roles"].([]any); len(roles) != 1 || roles[0] != "USER" {
		t.Fatalf("expected the USER role, got %v", first["roles"])
	}

	status, me := s.call(t, http.MethodGet, "/v1/users/me", token, nil)
	if status != http.StatusOK || me["id"] != internalID {
		t.Fatalf("users/me status=%d body=%v", status, me)
	}

	status, updated := s.call(t, http.MethodPatch, "/v1/users/me", token,
		map[string]string{"username": "Mickey", "displayName": "Mickey", "locale": "th"})
	if status != http.StatusOK {
		t.Fatalf("passport setup status=%d body=%v", status, updated)
	}
	if updated["profileComplete"] != true || updated["username"] != "Mickey" {
		t.Fatalf("passport setup did not complete the profile: %v", updated)
	}

	status, repeat := s.call(t, http.MethodPost, "/v1/auth/bootstrap", token, nil)
	if status != http.StatusOK || repeat["id"] != internalID {
		t.Fatalf("repeat login must reuse the same internal user: status=%d body=%v", status, repeat)
	}
	var users int
	if err := s.pool.QueryRow(t.Context(), "SELECT count(*) FROM users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Fatalf("repeat login created duplicate users: %d", users)
	}
}

func TestEndToEndUsernameIsUniqueCaseInsensitivelyAcrossUsers(t *testing.T) {
	s := newStack(t)
	one, two := s.token(t, "external-subject-1"), s.token(t, "external-subject-2")
	s.call(t, http.MethodPost, "/v1/auth/bootstrap", one, nil)
	s.call(t, http.MethodPost, "/v1/auth/bootstrap", two, nil)

	if status, body := s.call(t, http.MethodPatch, "/v1/users/me", one,
		map[string]string{"username": "Mickey", "displayName": "Mickey"}); status != http.StatusOK {
		t.Fatalf("first claim status=%d body=%v", status, body)
	}
	status, body := s.call(t, http.MethodPatch, "/v1/users/me", two,
		map[string]string{"username": "MICKEY", "displayName": "Impostor"})
	if status != http.StatusConflict {
		t.Fatalf("case-insensitive duplicate must conflict: status=%d body=%v", status, body)
	}
	errBody, _ := body["error"].(map[string]any)
	if errBody["code"] != "USERNAME_TAKEN" || errBody["requestId"] == "" {
		t.Fatalf("unexpected error contract: %v", body)
	}
}

func TestEndToEndRejectsForgedAndProtectedFieldRequests(t *testing.T) {
	s := newStack(t)
	valid := s.token(t, "external-subject-1")
	s.call(t, http.MethodPost, "/v1/auth/bootstrap", valid, nil)

	forged, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer, "aud": []string{audience}, "sub": "external-subject-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = "e2e-key"
	forgedToken, _ := tok.SignedString(forged)

	for name, token := range map[string]string{"forged_signature": forgedToken, "garbage": "not-a-jwt", "empty": ""} {
		t.Run(name, func(t *testing.T) {
			if status, _ := s.call(t, http.MethodGet, "/v1/users/me", token, nil); status != http.StatusUnauthorized {
				t.Fatalf("expected 401 for %s, got %d", name, status)
			}
		})
	}

	for _, payload := range []map[string]any{
		{"roles": []string{"ADMIN"}},
		{"status": "DELETED"},
		{"id": "00000000-0000-0000-0000-000000000000"},
	} {
		if status, _ := s.call(t, http.MethodPatch, "/v1/users/me", valid, payload); status != http.StatusBadRequest {
			t.Fatalf("client must not be able to send %v (status=%d)", payload, status)
		}
	}

	var roles int
	if err := s.pool.QueryRow(t.Context(), "SELECT count(*) FROM user_roles WHERE role <> 'USER'").Scan(&roles); err != nil {
		t.Fatal(err)
	}
	if roles != 0 {
		t.Fatalf("client escalated roles: %d non-USER rows", roles)
	}
}
