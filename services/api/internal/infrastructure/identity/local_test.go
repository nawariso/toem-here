package identity_test

import (
	"errors"
	"testing"

	"github.com/nawariso/toem-hia/services/api/internal/infrastructure/config"
	"github.com/nawariso/toem-hia/services/api/internal/infrastructure/identity"
)

func localConfig(appEnv string) config.Config {
	return config.Config{AppEnv: appEnv, AuthMode: config.AuthModeLocal, HTTPPort: "8080", DatabaseURL: "postgres://x"}
}

// Requirement 001-B §34: valid local credential -> deterministic identity.
func TestLocalDevVerifierResolvesDeterministicIdentity(t *testing.T) {
	v, err := identity.NewLocalDevVerifier(config.EnvDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		ext, err := v.Verify(t.Context(), identity.LocalDevCredential)
		if err != nil {
			t.Fatal(err)
		}
		if ext.Provider != "LOCAL_DEV" || ext.Subject != "developer-001" || ext.Email != nil {
			t.Fatalf("identity=%+v", ext)
		}
	}
}

// Requirement 001-B §34: invalid credential -> unauthorized.
func TestLocalDevVerifierRejectsAnythingElse(t *testing.T) {
	v, _ := identity.NewLocalDevVerifier(config.EnvTest)
	for _, raw := range []string{
		"", " ", "developer-001", "toem-hia-local-dev.developer-002",
		identity.LocalDevCredential + " ", " " + identity.LocalDevCredential,
		"TOEM-HIA-LOCAL-DEV.DEVELOPER-001", `{"userId":"00000000-0000-0000-0000-000000000000"}`,
	} {
		if _, err := v.Verify(t.Context(), raw); !errors.Is(err, identity.ErrInvalidToken) {
			t.Fatalf("credential %q must be rejected, got %v", raw, err)
		}
	}
}

// Requirement 001-B §34: local auth + production environment -> rejected.
func TestLocalDevVerifierCannotBeConstructedForProduction(t *testing.T) {
	for _, env := range []string{"production", "staging", "", "Development"} {
		if v, err := identity.NewLocalDevVerifier(env); err == nil || v != nil {
			t.Fatalf("local verifier must not exist for APP_ENV=%q", env)
		}
	}
	if _, err := identity.FromConfig(localConfig("production")); !errors.Is(err, config.ErrLocalAuthForbidden) {
		t.Fatalf("FromConfig must refuse production + local, got %v", err)
	}
}

func TestFromConfigSelectsExactlyTheConfiguredVerifier(t *testing.T) {
	v, err := identity.FromConfig(localConfig("development"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.(*identity.LocalDevVerifier); !ok {
		t.Fatalf("local mode must select LocalDevVerifier, got %T", v)
	}

	supabase := config.Config{AppEnv: "production", AuthMode: config.AuthModeSupabase, HTTPPort: "8080", DatabaseURL: "postgres://x",
		AuthIssuer: "https://issuer.example/auth/v1", AuthAudience: "authenticated", AuthJWKSURL: "http://127.0.0.1:1/jwks"}
	v, err = identity.FromConfig(supabase)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.(*identity.Verifier); !ok {
		t.Fatalf("supabase mode must select the JWT verifier, got %T", v)
	}

	// Requirement 001-B §34: supabase mode -> local credential rejected.
	if _, err := v.Verify(t.Context(), identity.LocalDevCredential); !errors.Is(err, identity.ErrInvalidToken) {
		t.Fatalf("supabase mode must reject the local development credential, got %v", err)
	}

	if _, err := identity.FromConfig(config.Config{AppEnv: "development", AuthMode: "none", HTTPPort: "8080"}); err == nil {
		t.Fatal("unknown mode must not yield a verifier")
	}
}
