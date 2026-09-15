package identity_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/nawariso/toem-here/services/api/internal/infrastructure/identity"
)

// countingJWKS serves the given keys and records how many times it was fetched.
func countingJWKS(t *testing.T, keys ...jwkEntry) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	entries := make([]any, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, map[string]any{
			"kty": "RSA", "kid": key.kid, "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.pub.E)).Bytes()),
		})
	}
	body, _ := json.Marshal(map[string]any{"keys": entries})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server, &hits
}

type jwkEntry struct {
	kid string
	pub *rsa.PublicKey
}

func unknownKidToken(t *testing.T, key *rsa.PrivateKey, kid string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": "https://issuer.example/auth/v1",
		"aud": []string{"authenticated"},
		"sub": "subject",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Add(-time.Minute).Unix(),
	})
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// An unauthenticated caller must not be able to convert unknown-kid tokens into
// unbounded outbound JWKS fetches serialized behind the verifier mutex.
func TestUnknownKeyIdsDoNotCauseUnboundedJWKSFetches(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	server, hits := countingJWKS(t, jwkEntry{kid: "real-key", pub: &key.PublicKey})
	verifier, err := identity.NewJWTVerifier("https://issuer.example/auth/v1", "authenticated", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	// Prime the cache with a legitimate verification.
	if _, err = verifier.Verify(t.Context(), unknownKidToken(t, key, "real-key")); err != nil {
		t.Fatal(err)
	}
	primed := hits.Load()

	for i := range 50 {
		if _, err = verifier.Verify(t.Context(), unknownKidToken(t, key, "attacker-kid-"+string(rune('a'+i%26)))); err == nil {
			t.Fatal("tokens with an unknown key id must be rejected")
		}
	}

	extra := hits.Load() - primed
	if extra > 2 {
		t.Fatalf("50 unknown-kid tokens triggered %d JWKS fetches; refresh must be throttled", extra)
	}
}

func TestConcurrentUnknownKeyIdsStayThrottled(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	server, hits := countingJWKS(t, jwkEntry{kid: "real-key", pub: &key.PublicKey})
	verifier, _ := identity.NewJWTVerifier("https://issuer.example/auth/v1", "authenticated", server.URL, server.Client())
	if _, err := verifier.Verify(t.Context(), unknownKidToken(t, key, "real-key")); err != nil {
		t.Fatal(err)
	}
	primed := hits.Load()

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = verifier.Verify(t.Context(), unknownKidToken(t, key, "unknown"))
		}()
	}
	wg.Wait()

	if extra := hits.Load() - primed; extra > 2 {
		t.Fatalf("concurrent unknown-kid tokens triggered %d JWKS fetches", extra)
	}
}

// A rotated key must still be picked up once the throttle window elapses,
// otherwise throttling would break legitimate key rotation.
func TestRotatedKeyIsAdoptedAfterCacheExpiry(t *testing.T) {
	oldKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	newKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	var serveNew atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		active := oldKey
		kid := "old-key"
		if serveNew.Load() {
			active, kid = newKey, "new-key"
		}
		body, _ := json.Marshal(map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(active.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(active.PublicKey.E)).Bytes()),
		}}})
		_, _ = w.Write(body)
	}))
	defer server.Close()

	verifier, _ := identity.NewJWTVerifier("https://issuer.example/auth/v1", "authenticated", server.URL, server.Client())
	if _, err := verifier.Verify(t.Context(), unknownKidToken(t, oldKey, "old-key")); err != nil {
		t.Fatal(err)
	}

	serveNew.Store(true)
	identity.ExpireCacheForTest(verifier)

	if _, err := verifier.Verify(t.Context(), unknownKidToken(t, newKey, "new-key")); err != nil {
		t.Fatalf("rotated signing key must be adopted after the cache expires: %v", err)
	}
}

// A JWKS endpoint offering an undersized RSA key must not be trusted.
func TestUndersizedRSAKeysAreRejected(t *testing.T) {
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	server, _ := countingJWKS(t, jwkEntry{kid: "weak-key", pub: &weak.PublicKey})
	verifier, _ := identity.NewJWTVerifier("https://issuer.example/auth/v1", "authenticated", server.URL, server.Client())

	if _, err := verifier.Verify(t.Context(), unknownKidToken(t, weak, "weak-key")); err == nil {
		t.Fatal("an undersized RSA signing key must not be accepted")
	}
}
