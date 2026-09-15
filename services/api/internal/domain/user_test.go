package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nawariso/toem-here/services/api/internal/domain"
)

func TestProfileCompleteRequiresUsernameAndDisplayName(t *testing.T) {
	blank, username, name := "   ", "mickey", "Mickey"
	cases := map[string]struct {
		user domain.User
		want bool
	}{
		"new user":            {domain.User{}, false},
		"username only":       {domain.User{Username: &username}, false},
		"display name only":   {domain.User{DisplayName: &name}, false},
		"whitespace username": {domain.User{Username: &blank, DisplayName: &name}, false},
		"complete":            {domain.User{Username: &username, DisplayName: &name}, true},
	}
	for label, tc := range cases {
		t.Run(label, func(t *testing.T) {
			if got := tc.user.ProfileComplete(); got != tc.want {
				t.Fatalf("ProfileComplete() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewUserStartsActiveWithIdentityAndNoRoles(t *testing.T) {
	user := domain.NewUser("th")
	if user.ID == "" {
		t.Fatal("new user must receive an internal id")
	}
	if user.Status != "ACTIVE" {
		t.Fatalf("status = %q, want ACTIVE", user.Status)
	}
	if user.DeletedAt != nil {
		t.Fatal("new user must not be soft deleted")
	}
	if user.ProfileComplete() {
		t.Fatal("new user must require passport setup")
	}
	if other := domain.NewUser("th"); other.ID == user.ID {
		t.Fatal("internal ids must be unique")
	}
}

func TestValidateIdentityRejectsMissingProviderOrSubject(t *testing.T) {
	cases := map[string]domain.ExternalIdentity{
		"empty":          {},
		"no provider":    {Subject: "abc"},
		"no subject":     {Provider: "SUPABASE"},
		"blank provider": {Provider: "  ", Subject: "abc"},
		"blank subject":  {Provider: "SUPABASE", Subject: "  "},
	}
	for label, identity := range cases {
		t.Run(label, func(t *testing.T) {
			if err := domain.ValidateIdentity(identity); !errors.Is(err, domain.ErrInvalidIdentity) {
				t.Fatalf("expected ErrInvalidIdentity, got %v", err)
			}
		})
	}
	if err := domain.ValidateIdentity(domain.ExternalIdentity{Provider: "SUPABASE", Subject: "abc"}); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}
}

func TestValidatePatchEnforcesUsernameRules(t *testing.T) {
	invalid := []string{"ab", strings.Repeat("a", 31), "has space", "hy-phen", "emoji🦎", "dots.here", ""}
	for _, candidate := range invalid {
		t.Run("reject_"+candidate, func(t *testing.T) {
			value := candidate
			if err := domain.ValidatePatch(domain.ProfilePatch{Username: &value}); !errors.Is(err, domain.ErrInvalidUsername) {
				t.Fatalf("expected ErrInvalidUsername for %q, got %v", candidate, err)
			}
		})
	}
	for _, candidate := range []string{"abc", "Mickey_01", strings.Repeat("a", 30), "___"} {
		t.Run("accept_"+candidate, func(t *testing.T) {
			value := candidate
			if err := domain.ValidatePatch(domain.ProfilePatch{Username: &value}); err != nil {
				t.Fatalf("expected %q to be valid, got %v", candidate, err)
			}
		})
	}
}

func TestValidatePatchEnforcesDisplayNameAndLocale(t *testing.T) {
	blank, tooLong := "   ", strings.Repeat("n", 81)
	if err := domain.ValidatePatch(domain.ProfilePatch{DisplayName: &blank}); !errors.Is(err, domain.ErrInvalidDisplayName) {
		t.Fatalf("blank display name: %v", err)
	}
	if err := domain.ValidatePatch(domain.ProfilePatch{DisplayName: &tooLong}); !errors.Is(err, domain.ErrInvalidDisplayName) {
		t.Fatalf("long display name: %v", err)
	}
	thai := "มิกกี้"
	if err := domain.ValidatePatch(domain.ProfilePatch{DisplayName: &thai}); err != nil {
		t.Fatalf("Thai display names must be accepted: %v", err)
	}

	for _, bad := range []string{"english", "TH", "th_TH", "t"} {
		value := bad
		if err := domain.ValidatePatch(domain.ProfilePatch{Locale: &value}); !errors.Is(err, domain.ErrInvalidLocale) {
			t.Fatalf("expected ErrInvalidLocale for %q, got %v", bad, err)
		}
	}
	for _, good := range []string{"th", "en", "en-US"} {
		value := good
		if err := domain.ValidatePatch(domain.ProfilePatch{Locale: &value}); err != nil {
			t.Fatalf("expected %q to be valid, got %v", good, err)
		}
	}
}

func TestEmptyPatchIsValidSoPartialUpdatesAreAllowed(t *testing.T) {
	if err := domain.ValidatePatch(domain.ProfilePatch{}); err != nil {
		t.Fatalf("empty patch must be valid: %v", err)
	}
}
