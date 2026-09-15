package postgres_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/nawariso/toem-here/services/api/internal/application"
	"github.com/nawariso/toem-here/services/api/internal/domain"
	persistence "github.com/nawariso/toem-here/services/api/internal/infrastructure/persistence/postgres"
	"github.com/nawariso/toem-here/services/api/internal/infrastructure/persistence/postgres/migrations"
	"github.com/nawariso/toem-here/services/api/internal/testsupport"
)

func TestMigrationsCreateRollBackAndReapplySchema(t *testing.T) {
	pool := testsupport.Database(t)

	for _, table := range []string{"users", "auth_identities", "user_roles"} {
		var found *string
		if err := pool.QueryRow(t.Context(), "SELECT to_regclass(current_schema() || '.' || $1)::text", table).Scan(&found); err != nil {
			t.Fatal(err)
		}
		if found == nil {
			t.Fatalf("migration did not create %q", table)
		}
	}

	if err := migrations.Down(t.Context(), pool); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	var afterDown *string
	if err := pool.QueryRow(t.Context(), "SELECT to_regclass(current_schema() || '.users')::text").Scan(&afterDown); err != nil {
		t.Fatal(err)
	}
	if afterDown != nil {
		t.Fatalf("rollback left %q behind", *afterDown)
	}

	if err := migrations.Up(t.Context(), pool); err != nil {
		t.Fatalf("re-apply failed: %v", err)
	}
	var afterUp *string
	if err := pool.QueryRow(t.Context(), "SELECT to_regclass(current_schema() || '.users')::text").Scan(&afterUp); err != nil {
		t.Fatal(err)
	}
	if afterUp == nil {
		t.Fatal("re-apply did not restore the schema")
	}
}

func TestBootstrapIsAtomicAndRepeatSafe(t *testing.T) {
	pool := testsupport.Database(t)
	repo := persistence.NewRepository(pool)
	svc := application.NewUserService(repo)
	identity := domain.ExternalIdentity{Provider: "SUPABASE", Subject: "subject-1"}

	first, err := svc.Bootstrap(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Bootstrap(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("repeat login created a duplicate user: %s != %s", first.ID, second.ID)
	}

	var users, identities, roles int
	if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM auth_identities").Scan(&identities); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM user_roles WHERE role = 'USER'").Scan(&roles); err != nil {
		t.Fatal(err)
	}
	if users != 1 || identities != 1 || roles != 1 {
		t.Fatalf("expected exactly one user/identity/role row, got %d/%d/%d", users, identities, roles)
	}
}

func TestBootstrapRollsBackWhenTheTransactionCannotComplete(t *testing.T) {
	pool := testsupport.Database(t)
	repo := persistence.NewRepository(pool)

	// A role value outside the CHECK constraint must abort the whole first-login
	// transaction, leaving no partially created user or identity behind.
	if _, err := pool.Exec(t.Context(), "ALTER TABLE user_roles DROP CONSTRAINT user_roles_user_id_fkey"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(),
		"ALTER TABLE user_roles ADD CONSTRAINT user_roles_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), "ALTER TABLE auth_identities ADD CONSTRAINT auth_identities_reject_all CHECK (false) NOT VALID"); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.Bootstrap(t.Context(), domain.ExternalIdentity{Provider: "SUPABASE", Subject: "subject-1"}); err == nil {
		t.Fatal("expected bootstrap to fail while the identity insert is rejected")
	}

	var users int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 0 {
		t.Fatalf("failed bootstrap left %d orphaned user row(s) behind", users)
	}
}

func TestConcurrentBootstrapPreventsDuplicates(t *testing.T) {
	pool := testsupport.Database(t)
	svc := application.NewUserService(persistence.NewRepository(pool))
	identity := domain.ExternalIdentity{Provider: "SUPABASE", Subject: "subject-concurrent"}

	const attempts = 8
	ids := make(chan string, attempts)
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			user, err := svc.Bootstrap(context.Background(), identity)
			if err != nil {
				errs <- err
				return
			}
			ids <- user.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent bootstrap failed: %v", err)
	}

	var expected string
	for id := range ids {
		if expected == "" {
			expected = id
		}
		if id != expected {
			t.Fatalf("concurrent bootstrap produced different internal users: %s != %s", id, expected)
		}
	}

	var users int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Fatalf("concurrent first login created %d users", users)
	}
}

func TestUsernameIsUniqueCaseInsensitively(t *testing.T) {
	pool := testsupport.Database(t)
	svc := application.NewUserService(persistence.NewRepository(pool))
	one := domain.ExternalIdentity{Provider: "SUPABASE", Subject: "one"}
	two := domain.ExternalIdentity{Provider: "SUPABASE", Subject: "two"}
	if _, err := svc.Bootstrap(t.Context(), one); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Bootstrap(t.Context(), two); err != nil {
		t.Fatal(err)
	}

	claimed, variant, name := "Mickey", "mIcKeY", "Name"
	if _, err := svc.UpdateProfile(t.Context(), one, domain.ProfilePatch{Username: &claimed, DisplayName: &name}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.UpdateProfile(t.Context(), two, domain.ProfilePatch{Username: &variant, DisplayName: &name})
	if !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatalf("expected ErrUsernameTaken, got %v", err)
	}
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "sqlstate") {
		t.Fatalf("raw SQL error leaked to the caller: %v", err)
	}
}

func TestSameUserMayKeepItsOwnUsernameWhileUpdatingOtherFields(t *testing.T) {
	pool := testsupport.Database(t)
	svc := application.NewUserService(persistence.NewRepository(pool))
	identity := domain.ExternalIdentity{Provider: "SUPABASE", Subject: "subject-1"}
	if _, err := svc.Bootstrap(t.Context(), identity); err != nil {
		t.Fatal(err)
	}

	username, name, locale := "mickey", "Mickey", "th"
	if _, err := svc.UpdateProfile(t.Context(), identity, domain.ProfilePatch{Username: &username, DisplayName: &name, Locale: &locale}); err != nil {
		t.Fatal(err)
	}
	english := "en"
	updated, err := svc.UpdateProfile(t.Context(), identity, domain.ProfilePatch{Locale: &english})
	if err != nil {
		t.Fatalf("updating locale must not collide with the user's own username: %v", err)
	}
	if updated.Locale != "en" || updated.Username == nil || *updated.Username != "mickey" {
		t.Fatalf("unexpected profile after partial update: %+v", updated)
	}
}
