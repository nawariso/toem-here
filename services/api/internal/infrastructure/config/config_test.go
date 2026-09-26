package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nawariso/toem-hia/services/api/internal/infrastructure/config"
)

const databaseURL = "postgres://user:s3cret-value@localhost:5432/toem_hia?sslmode=disable"

var supabaseSettings = map[string]string{
	"AUTH_ISSUER":   "https://project.supabase.co/auth/v1",
	"AUTH_AUDIENCE": "authenticated",
	"AUTH_JWKS_URL": "https://project.supabase.co/auth/v1/.well-known/jwks.json",
}

// clearEnv isolates each test from the developer's shell environment.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"APP_ENV", "AUTH_MODE", "HTTP_PORT", "DATABASE_URL", "AUTH_ISSUER", "AUTH_AUDIENCE", "AUTH_JWKS_URL"} {
		t.Setenv(key, "")
	}
}

func setLocal(t *testing.T) {
	t.Helper()
	clearEnv(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("AUTH_MODE", "local")
	t.Setenv("DATABASE_URL", databaseURL)
}

func setSupabase(t *testing.T, appEnv string) {
	t.Helper()
	clearEnv(t)
	t.Setenv("APP_ENV", appEnv)
	t.Setenv("AUTH_MODE", "supabase")
	t.Setenv("DATABASE_URL", databaseURL)
	for key, value := range supabaseSettings {
		t.Setenv(key, value)
	}
}

// Requirement 001-B §36: development + local -> PASS, without any Supabase value.
func TestDevelopmentLocalPassesWithoutSupabaseConfiguration(t *testing.T) {
	setLocal(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("development + local must start without Supabase config: %v", err)
	}
	if cfg.AuthMode != config.AuthModeLocal || cfg.AppEnv != config.EnvDevelopment || cfg.HTTPPort != "8080" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestTestEnvironmentMayUseLocalAuth(t *testing.T) {
	setLocal(t)
	t.Setenv("APP_ENV", "test")
	if _, err := config.Load(); err != nil {
		t.Fatal(err)
	}
}

// Requirement 001-B §36: production + local -> FAIL.
func TestProductionLocalIsRejected(t *testing.T) {
	setLocal(t)
	t.Setenv("APP_ENV", "production")
	// Supplying valid Supabase values must not make the combination acceptable.
	for key, value := range supabaseSettings {
		t.Setenv(key, value)
	}
	_, err := config.Load()
	if !errors.Is(err, config.ErrLocalAuthForbidden) {
		t.Fatalf("production + local must be a fatal startup error, got %v", err)
	}
}

func TestLocalAuthIsAllowlistedToDevelopmentAndTestOnly(t *testing.T) {
	for _, env := range []string{"production", "staging", "prod", "Production", "DEVELOPMENT", "dev"} {
		t.Run(env, func(t *testing.T) {
			setLocal(t)
			t.Setenv("APP_ENV", env)
			if _, err := config.Load(); err == nil {
				t.Fatalf("APP_ENV=%q with AUTH_MODE=local must fail startup", env)
			}
		})
	}
}

// Requirement 001-B §36: production + supabase + valid config -> PASS.
func TestProductionSupabaseWithValidConfigurationPasses(t *testing.T) {
	setSupabase(t, "production")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthIssuer != supabaseSettings["AUTH_ISSUER"] || cfg.AuthMode != config.AuthModeSupabase {
		t.Fatalf("configuration was not read from the environment: %+v", cfg)
	}
}

// Requirement 001-B §36: development + supabase + missing Supabase config -> FAIL.
func TestSupabaseModeFailsFastAndNamesEveryMissingValue(t *testing.T) {
	for missing := range supabaseSettings {
		t.Run(missing, func(t *testing.T) {
			setSupabase(t, "development")
			t.Setenv(missing, "")
			_, err := config.Load()
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("supabase mode must fail naming %s, got %v", missing, err)
			}
		})
	}
}

func TestSupabaseModeRejectsWhitespaceOnlyValues(t *testing.T) {
	setSupabase(t, "development")
	t.Setenv("AUTH_JWKS_URL", "   ")
	if _, err := config.Load(); err == nil {
		t.Fatal("whitespace must not satisfy a required value")
	}
}

func TestModeAndEnvironmentHaveNoImplicitDefault(t *testing.T) {
	for _, key := range []string{"APP_ENV", "AUTH_MODE", "DATABASE_URL"} {
		t.Run(key, func(t *testing.T) {
			setLocal(t)
			t.Setenv(key, "")
			_, err := config.Load()
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("missing %s must fail startup naming it, got %v", key, err)
			}
		})
	}
}

func TestUnknownAuthModeFailsStartup(t *testing.T) {
	for _, mode := range []string{"none", "disabled", "LOCAL", "dev", "Supabase"} {
		t.Run(mode, func(t *testing.T) {
			setLocal(t)
			t.Setenv("AUTH_MODE", mode)
			if _, err := config.Load(); err == nil {
				t.Fatalf("AUTH_MODE=%q must fail startup", mode)
			}
		})
	}
}

func TestReportsAllMissingValuesAtOnce(t *testing.T) {
	clearEnv(t)
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected failure when no configuration is present")
	}
	for _, key := range []string{"APP_ENV", "AUTH_MODE", "DATABASE_URL"} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("error must list %s: %v", key, err)
		}
	}
}

func TestValidateRejectsHandBuiltProductionLocalConfig(t *testing.T) {
	cfg := config.Config{AppEnv: "production", AuthMode: "local", HTTPPort: "8080", DatabaseURL: databaseURL}
	if !errors.Is(cfg.Validate(), config.ErrLocalAuthForbidden) {
		t.Fatal("Validate must refuse production + local even when Load was bypassed")
	}
}

func TestNeverEchoesSecretValuesInErrors(t *testing.T) {
	setLocal(t)
	t.Setenv("APP_ENV", "production")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "s3cret") || strings.Contains(err.Error(), "postgres://") {
		t.Fatalf("configuration error leaked credential material: %v", err)
	}
}
