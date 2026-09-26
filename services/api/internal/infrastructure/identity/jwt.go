package identity

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/nawariso/toem-hia/services/api/internal/domain"
)

var ErrInvalidToken = errors.New("invalid identity token")

const (
	// cacheTTL bounds how long signing keys are trusted without a refetch.
	cacheTTL = 10 * time.Minute
	// refreshCooldown throttles refetches so unauthenticated callers cannot
	// convert unknown-key-id tokens into unbounded outbound JWKS requests.
	refreshCooldown = 30 * time.Second
	// minRSAModulusBits rejects undersized signing keys even if the JWKS
	// endpoint offers them.
	minRSAModulusBits = 2048
	// maxJWKSBytes bounds the response body read from the JWKS endpoint.
	maxJWKSBytes = 1 << 20
)

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

type Verifier struct {
	issuer, audience, url string
	client                *http.Client

	mu          sync.Mutex
	keys        map[string]*rsa.PublicKey
	expires     time.Time
	lastAttempt time.Time
}

func NewJWTVerifier(issuer, audience, url string, client *http.Client) (*Verifier, error) {
	if issuer == "" || audience == "" || url == "" {
		return nil, errors.New("issuer, audience, and JWKS URL are required")
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Verifier{issuer: issuer, audience: audience, url: url, client: client}, nil
}

func (v *Verifier) Verify(ctx context.Context, raw string) (domain.ExternalIdentity, error) {
	if raw == "" {
		return domain.ExternalIdentity{}, ErrInvalidToken
	}
	claims := struct {
		Email *string `json:"email"`
		jwt.RegisteredClaims
	}{}
	token, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, ErrInvalidToken
		}
		return v.key(ctx, kid)
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil || !token.Valid || claims.Subject == "" {
		return domain.ExternalIdentity{}, ErrInvalidToken
	}
	return domain.ExternalIdentity{Provider: "SUPABASE", Subject: claims.Subject, Email: claims.Email}, nil
}

// key returns the cached public key for kid. A miss only triggers a refetch
// when the cache is stale or the refresh cooldown has elapsed, so unknown key
// ids cannot be used to amplify traffic against the identity provider.
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now()
	if now.Before(v.expires) {
		if key := v.keys[kid]; key != nil {
			return key, nil
		}
		// Cache is fresh but the key id is unknown: refuse without refetching.
		return nil, ErrInvalidToken
	}
	if now.Sub(v.lastAttempt) < refreshCooldown {
		if key := v.keys[kid]; key != nil {
			return key, nil
		}
		return nil, ErrInvalidToken
	}

	v.lastAttempt = now
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	if key := v.keys[kid]; key != nil {
		return key, nil
	}
	return nil, ErrInvalidToken
}

// refresh must be called with v.mu held.
func (v *Verifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.url, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSBytes))
	if err != nil {
		return err
	}
	var set jwks
	if err = json.Unmarshal(body, &set); err != nil {
		return err
	}

	keys := map[string]*rsa.PublicKey{}
	for _, item := range set.Keys {
		if item.Kty != "RSA" || item.Alg != "RS256" || item.Kid == "" {
			continue
		}
		nBytes, eBytes := decode(item.N), decode(item.E)
		if len(nBytes) == 0 || len(eBytes) == 0 {
			continue
		}
		exponent := new(big.Int).SetBytes(eBytes)
		if !exponent.IsInt64() {
			continue
		}
		modulus := new(big.Int).SetBytes(nBytes)
		if modulus.BitLen() < minRSAModulusBits {
			continue
		}
		keys[item.Kid] = &rsa.PublicKey{N: modulus, E: int(exponent.Int64())}
	}
	if len(keys) == 0 {
		return errors.New("JWKS contains no supported signing keys")
	}
	v.keys = keys
	v.expires = time.Now().Add(cacheTTL)
	return nil
}

func decode(value string) []byte {
	decoded, _ := base64.RawURLEncoding.DecodeString(value)
	return decoded
}
