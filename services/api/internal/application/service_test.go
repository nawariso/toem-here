package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nawariso/toem-hia/services/api/internal/application"
	"github.com/nawariso/toem-hia/services/api/internal/domain"
)

type fakeRepo struct {
	users          map[string]domain.User
	bootstrapCalls int
}

func (r *fakeRepo) Bootstrap(_ context.Context, identity domain.ExternalIdentity) (domain.User, error) {
	r.bootstrapCalls++
	key := identity.Provider + ":" + identity.Subject
	if user, ok := r.users[key]; ok {
		return user, nil
	}
	user := domain.NewUser("th")
	user.Roles = []string{domain.RoleUser}
	r.users[key] = user
	return user, nil
}
func (r *fakeRepo) FindByIdentity(_ context.Context, identity domain.ExternalIdentity) (domain.User, error) {
	user, ok := r.users[identity.Provider+":"+identity.Subject]
	if !ok {
		return domain.User{}, application.ErrNotFound
	}
	return user, nil
}
func (r *fakeRepo) UpdateProfile(_ context.Context, identity domain.ExternalIdentity, patch domain.ProfilePatch) (domain.User, error) {
	key := identity.Provider + ":" + identity.Subject
	user, ok := r.users[key]
	if !ok {
		return domain.User{}, application.ErrNotFound
	}
	if patch.Username != nil {
		user.Username = patch.Username
	}
	if patch.DisplayName != nil {
		user.DisplayName = patch.DisplayName
	}
	if patch.Locale != nil {
		user.Locale = *patch.Locale
	}
	r.users[key] = user
	return user, nil
}

func TestBootstrapCreatesInternalUserAndUserRole(t *testing.T) {
	repo := &fakeRepo{users: map[string]domain.User{}}
	svc := application.NewUserService(repo)
	user, err := svc.Bootstrap(context.Background(), domain.ExternalIdentity{Provider: "SUPABASE", Subject: "subject-1"})
	if err != nil {
		t.Fatal(err)
	}
	if user.ID == "" {
		t.Fatal("expected internal user ID")
	}
	if len(user.Roles) != 1 || user.Roles[0] != domain.RoleUser {
		t.Fatalf("roles=%v", user.Roles)
	}
	if user.ProfileComplete() {
		t.Fatal("new profile must be incomplete")
	}
}

func TestRepeatBootstrapReturnsSameInternalUser(t *testing.T) {
	repo := &fakeRepo{users: map[string]domain.User{}}
	svc := application.NewUserService(repo)
	identity := domain.ExternalIdentity{Provider: "SUPABASE", Subject: "subject-1"}
	first, _ := svc.Bootstrap(context.Background(), identity)
	second, _ := svc.Bootstrap(context.Background(), identity)
	if first.ID != second.ID {
		t.Fatalf("duplicate users: %s != %s", first.ID, second.ID)
	}
}

func TestIdentityIsRequired(t *testing.T) {
	svc := application.NewUserService(&fakeRepo{users: map[string]domain.User{}})
	_, err := svc.Bootstrap(context.Background(), domain.ExternalIdentity{})
	if !errors.Is(err, domain.ErrInvalidIdentity) {
		t.Fatalf("got %v", err)
	}
}

func TestProfilePatchValidatesUsernameAndDisplayName(t *testing.T) {
	svc := application.NewUserService(&fakeRepo{users: map[string]domain.User{}})
	identity := domain.ExternalIdentity{Provider: "SUPABASE", Subject: "subject-1"}
	_, _ = svc.Bootstrap(context.Background(), identity)
	short := "ab"
	_, err := svc.UpdateProfile(context.Background(), identity, domain.ProfilePatch{Username: &short})
	if !errors.Is(err, domain.ErrInvalidUsername) {
		t.Fatalf("got %v", err)
	}
	valid, name, locale := "Mickey_01", "Mickey", "th"
	user, err := svc.UpdateProfile(context.Background(), identity, domain.ProfilePatch{Username: &valid, DisplayName: &name, Locale: &locale})
	if err != nil {
		t.Fatal(err)
	}
	if !user.ProfileComplete() {
		t.Fatal("profile should be complete")
	}
}
