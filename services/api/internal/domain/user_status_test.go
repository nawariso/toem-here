package domain_test

import (
	"testing"

	"github.com/nawariso/toem-hia/services/api/internal/domain"
)

// Requirement 002 amendment 5: only ACTIVE users may perform protected writes.
func TestOnlyActiveUsersCanWrite(t *testing.T) {
	cases := map[string]bool{
		domain.UserStatusActive:    true,
		domain.UserStatusSuspended: false,
		domain.UserStatusDeleted:   false,
		"":                         false,
		"active":                   false,
	}
	for status, want := range cases {
		if got := (domain.User{Status: status}).CanWrite(); got != want {
			t.Fatalf("status %q: CanWrite=%t want %t", status, got, want)
		}
	}
	if !domain.NewUser("th").CanWrite() {
		t.Fatal("a newly bootstrapped user must be ACTIVE")
	}
}
