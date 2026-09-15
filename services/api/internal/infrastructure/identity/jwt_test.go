package identity_test

import (
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
	"github.com/nawariso/toem-here/services/api/internal/infrastructure/identity"
)

func jwkServer(t *testing.T, pub *rsa.PublicKey) *httptest.Server {
	t.Helper()
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	payload, _ := json.Marshal(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "test-key", "use": "sig", "alg": "RS256", "n": n, "e": e}}})
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
}

func token(t *testing.T, key *rsa.PrivateKey, issuer string, audience []string, expiry time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{"iss": issuer, "aud": audience, "sub": "external-subject", "email": "private@example.com", "exp": expiry.Unix(), "iat": time.Now().Add(-time.Minute).Unix()}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "test-key"
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestVerifierValidatesSignatureIssuerAudienceExpiry(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	server := jwkServer(t, &key.PublicKey)
	defer server.Close()
	verifier, err := identity.NewJWTVerifier("https://issuer.example/auth/v1", "authenticated", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ext, err := verifier.Verify(t.Context(), token(t, key, "https://issuer.example/auth/v1", []string{"authenticated"}, time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	if ext.Provider != "SUPABASE" || ext.Subject != "external-subject" {
		t.Fatalf("identity=%+v", ext)
	}
}

func TestVerifierRejectsInvalidClaimsAndSignature(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	server := jwkServer(t, &key.PublicKey)
	defer server.Close()
	verifier, _ := identity.NewJWTVerifier("https://issuer.example/auth/v1", "authenticated", server.URL, server.Client())
	cases := map[string]string{
		"expired":  token(t, key, "https://issuer.example/auth/v1", []string{"authenticated"}, time.Now().Add(-time.Minute)),
		"issuer":   token(t, key, "https://evil.example", []string{"authenticated"}, time.Now().Add(time.Hour)),
		"audience": token(t, key, "https://issuer.example/auth/v1", []string{"other"}, time.Now().Add(time.Hour)),
	}
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	cases["signature"] = token(t, other, "https://issuer.example/auth/v1", []string{"authenticated"}, time.Now().Add(time.Hour))
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := verifier.Verify(t.Context(), raw); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestVerifierRejectsNonRS256(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	server := jwkServer(t, &key.PublicKey)
	defer server.Close()
	verifier, _ := identity.NewJWTVerifier("https://issuer.example/auth/v1", "authenticated", server.URL, server.Client())
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"iss": "https://issuer.example/auth/v1", "aud": "authenticated", "sub": "x", "exp": time.Now().Add(time.Hour).Unix()})
	tok.Header["kid"] = "test-key"
	raw, _ := tok.SignedString([]byte("secret"))
	if _, err := verifier.Verify(t.Context(), raw); err == nil {
		t.Fatal("expected algorithm rejection")
	}
}
