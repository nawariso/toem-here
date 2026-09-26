// Package testsupport provides PostgreSQL fixtures for integration tests.
//
// Go runs different test packages in parallel. Sharing one schema makes those
// packages drop and recreate each other's tables mid-run, so every test gets a
// private schema instead and drops it during cleanup.
package testsupport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nawariso/toem-here/services/api/internal/infrastructure/persistence/postgres/migrations"
)

// Database returns a fully migrated pool bound to an isolated schema. It
// skips the test when TEST_DATABASE_URL is absent so unit-only runs stay green.
func Database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := EmptyDatabase(t)
	if err := migrations.Up(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

// EmptyDatabase returns a pool bound to an isolated, unmigrated schema so a
// test can drive migrations version by version.
func EmptyDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}

	schema := "test_" + randomSuffix(t)
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("TEST_DATABASE_URL is not a valid URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()

	config, err := pgxpool.ParseConfig(parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 10

	admin, err := pgxpool.New(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err = admin.Exec(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}

	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cleanup, err := pgxpool.New(ctx, raw)
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	})
	return pool
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(buf)
}
