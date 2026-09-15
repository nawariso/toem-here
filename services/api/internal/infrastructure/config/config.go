package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct{ AppEnv, HTTPPort, DatabaseURL, AuthIssuer, AuthAudience, AuthJWKSURL string }

func Load() (Config, error) {
	c := Config{AppEnv: get("APP_ENV", "development"), HTTPPort: get("HTTP_PORT", "8080"), DatabaseURL: os.Getenv("DATABASE_URL"), AuthIssuer: os.Getenv("AUTH_ISSUER"), AuthAudience: os.Getenv("AUTH_AUDIENCE"), AuthJWKSURL: os.Getenv("AUTH_JWKS_URL")}
	missing := []string{}
	for key, value := range map[string]string{"DATABASE_URL": c.DatabaseURL, "AUTH_ISSUER": c.AuthIssuer, "AUTH_AUDIENCE": c.AuthAudience, "AUTH_JWKS_URL": c.AuthJWKSURL} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}
	if c.HTTPPort == "" {
		return Config{}, errors.New("HTTP_PORT cannot be empty")
	}
	return c, nil
}
func get(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
