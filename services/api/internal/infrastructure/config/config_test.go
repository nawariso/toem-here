package config_test

import (
	"strings"
	"testing"

	"github.com/nawariso/toem-here/services/api/internal/infrastructure/config"
)

var required = map[string]string{
	"DATABASE_URL":  "postgres://user:pass@localhost:5432/toem_here?sslmode=disable",
	"AUTH_ISSUER":   "https://project.supabase.co/auth/v1",
	"AUTH_AUDIENCE": "authenticated",
	"AUTH_JWKS_URL": "https://project.supabase.co/auth/v1/.well-known/jwks.json",
}

func setRequired(t *testing.T) {
	t.Helper()
	for key, value := range required {
		t.Setenv(key, value)
	}
}

func TestLoadAppliesSafeDefaultsForOptionalValues(t *testing.T) {
	setRequired(t)
	t.Setenv("APP_ENV", "")
	t.Setenv("HTTP_PORT", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppEnv != "development" || cfg.HTTPPort != "8080" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.DatabaseURL != required["DATABASE_URL"] || cfg.AuthIssuer != required["AUTH_ISSUER"] {
		t.Fatalf("configuration was not read from the environment: %+v", cfg)
	}
}

func TestLoadFailsFastAndNamesEveryMissingCriticalValue(t *testing.T) {
	for missing := range required {
		t.Run(missing, func(t *testing.T) {
			setRequired(t)
			t.Setenv(missing, "")

			_, err := config.Load()
			if err == nil {
				t.Fatalf("startup must fail when %s is absent", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Fatalf("error must name %s, got %v", missing, err)
			}
		})
	}
}

func TestLoadReportsAllMissingValuesAtOnce(t *testing.T) {
	for key := range required {
		t.Setenv(key, "")
	}
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected failure when no configuration is present")
	}
	for key := range required {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("error must list %s: %v", key, err)
		}
	}
}

func TestLoadRejectsWhitespaceOnlyConfiguration(t *testing.T) {
	setRequired(t)
	t.Setenv("AUTH_JWKS_URL", "   ")
	if _, err := config.Load(); err == nil {
		t.Fatal("whitespace must not satisfy a required value")
	}
}

func TestLoadNeverEchoesSecretValuesInErrors(t *testing.T) {
	setRequired(t)
	t.Setenv("DATABASE_URL", "")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "pass") {
		t.Fatalf("configuration error leaked credential material: %v", err)
	}
}
