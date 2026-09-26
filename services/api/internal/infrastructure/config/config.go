package config

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Application environments. APP_ENV has no default: an unset value must never
// be read as "development", because development permits local authentication.
const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvProduction  = "production"
)

// Authentication modes. There is no implicit fallback between them.
const (
	AuthModeLocal    = "local"
	AuthModeSupabase = "supabase"
)

// ErrLocalAuthForbidden is returned when local development authentication is
// requested outside an environment on the local-auth allowlist.
var ErrLocalAuthForbidden = errors.New("AUTH_MODE=local is development-only and is forbidden unless APP_ENV is development or test")

type Config struct {
	AppEnv, AuthMode, HTTPPort, DatabaseURL string
	AuthIssuer, AuthAudience, AuthJWKSURL   string
}

// Load reads configuration from the environment and fails fast. Supabase
// verification settings are required only when AUTH_MODE=supabase.
func Load() (Config, error) {
	c := Config{
		AppEnv:       strings.TrimSpace(os.Getenv("APP_ENV")),
		AuthMode:     strings.TrimSpace(os.Getenv("AUTH_MODE")),
		HTTPPort:     get("HTTP_PORT", "8080"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		AuthIssuer:   os.Getenv("AUTH_ISSUER"),
		AuthAudience: os.Getenv("AUTH_AUDIENCE"),
		AuthJWKSURL:  os.Getenv("AUTH_JWKS_URL"),
	}
	required := map[string]string{"APP_ENV": c.AppEnv, "AUTH_MODE": c.AuthMode, "DATABASE_URL": c.DatabaseURL}
	if c.AuthMode == AuthModeSupabase {
		required["AUTH_ISSUER"] = c.AuthIssuer
		required["AUTH_AUDIENCE"] = c.AuthAudience
		required["AUTH_JWKS_URL"] = c.AuthJWKSURL
	}
	missing := []string{}
	for key, value := range required {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return Config{}, fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate enforces the environment/auth-mode rules. It is exported so every
// component built from a Config can re-assert them, not only Load.
func (c Config) Validate() error {
	switch c.AppEnv {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		return fmt.Errorf("APP_ENV must be one of %s, %s, %s", EnvDevelopment, EnvTest, EnvProduction)
	}
	switch c.AuthMode {
	case AuthModeLocal:
		if !LocalAuthPermitted(c.AppEnv) {
			return ErrLocalAuthForbidden
		}
	case AuthModeSupabase:
		if strings.TrimSpace(c.AuthIssuer) == "" || strings.TrimSpace(c.AuthAudience) == "" || strings.TrimSpace(c.AuthJWKSURL) == "" {
			return errors.New("AUTH_MODE=supabase requires AUTH_ISSUER, AUTH_AUDIENCE, and AUTH_JWKS_URL")
		}
	default:
		return fmt.Errorf("AUTH_MODE must be one of %s, %s", AuthModeLocal, AuthModeSupabase)
	}
	if strings.TrimSpace(c.HTTPPort) == "" {
		return errors.New("HTTP_PORT cannot be empty")
	}
	return nil
}

// LocalAuthPermitted is an allowlist: only these environments may run local
// development authentication. Every other value is refused.
func LocalAuthPermitted(appEnv string) bool {
	return appEnv == EnvDevelopment || appEnv == EnvTest
}

func get(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
