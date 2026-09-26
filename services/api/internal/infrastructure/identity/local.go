package identity

import (
	"context"
	"crypto/subtle"
	"fmt"

	"github.com/nawariso/toem-here/services/api/internal/application"
	"github.com/nawariso/toem-here/services/api/internal/domain"
	"github.com/nawariso/toem-here/services/api/internal/infrastructure/config"
)

// LOCAL DEVELOPMENT ONLY. MUST NEVER BE ENABLED IN PRODUCTION.
//
// LocalDevProvider is the provider recorded on auth_identities rows created
// through local development authentication. It can never collide with a real
// provider's subjects because (provider, provider_subject) is the unique key.
const LocalDevProvider = "LOCAL_DEV"

// LocalDevCredential is the deterministic, non-sensitive bearer credential for
// the default development user. It grants nothing outside a server started
// with AUTH_MODE=local in a development/test environment, and the Supabase JWT
// verifier rejects it because it is not a signed JWT. The mobile
// LocalDevAuthProvider sends the same value (apps/mobile/src/auth/local-dev-adapter.ts).
const LocalDevCredential = "toem-local-dev.developer-001"

// LocalDevSubject is the stable subject of the default development user.
const LocalDevSubject = "developer-001"

// localDevIdentities maps accepted credentials to subjects. Additional
// deterministic identities (for example a second user or a moderator) can be
// added here later; roles are never derived from the credential.
var localDevIdentities = map[string]string{
	LocalDevCredential: LocalDevSubject,
}

// LocalDevVerifier implements application.IdentityVerifier for local
// development. It resolves a known development credential to a deterministic
// ExternalIdentity; the request still flows through bootstrap, AuthIdentity,
// and the internal User exactly as with a real provider.
type LocalDevVerifier struct{}

// NewLocalDevVerifier refuses construction outside the local-auth allowlist so
// that no wiring mistake can produce a production server that accepts it.
func NewLocalDevVerifier(appEnv string) (*LocalDevVerifier, error) {
	if !config.LocalAuthPermitted(appEnv) {
		return nil, config.ErrLocalAuthForbidden
	}
	return &LocalDevVerifier{}, nil
}

func (*LocalDevVerifier) Verify(_ context.Context, raw string) (domain.ExternalIdentity, error) {
	for credential, subject := range localDevIdentities {
		if subtle.ConstantTimeCompare([]byte(raw), []byte(credential)) == 1 {
			return domain.ExternalIdentity{Provider: LocalDevProvider, Subject: subject}, nil
		}
	}
	return domain.ExternalIdentity{}, ErrInvalidToken
}

// FromConfig re-validates cfg and returns the application.IdentityVerifier for
// its AUTH_MODE. Exactly one implementation is returned; there is no fallback.
func FromConfig(cfg config.Config) (application.IdentityVerifier, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	switch cfg.AuthMode {
	case config.AuthModeLocal:
		return NewLocalDevVerifier(cfg.AppEnv)
	case config.AuthModeSupabase:
		return NewJWTVerifier(cfg.AuthIssuer, cfg.AuthAudience, cfg.AuthJWKSURL, nil)
	default:
		return nil, fmt.Errorf("unsupported AUTH_MODE %q", cfg.AuthMode)
	}
}
