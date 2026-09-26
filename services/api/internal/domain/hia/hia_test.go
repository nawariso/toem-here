package hia_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nawariso/toem-hia/services/api/internal/domain/hia"
)

var now = time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func TestNewProvisionalHasNoClientPublicCode(t *testing.T) {
	h, err := hia.NewProvisional(ptr(" Somchai "), nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if h.Status != hia.StatusProvisional || h.PublicCode != "" || *h.Nickname != "Somchai" {
		t.Fatalf("unexpected hia: %+v", h)
	}
}

func TestStatusValidation(t *testing.T) {
	for _, s := range []string{hia.StatusProvisional, hia.StatusConfirmed, hia.StatusInactive, hia.StatusArchived, hia.StatusMerged} {
		if err := hia.ValidateStatus(s); err != nil {
			t.Fatalf("%s rejected: %v", s, err)
		}
	}
	for _, s := range []string{"", "provisional", "DELETED", "ACTIVE"} {
		if err := hia.ValidateStatus(s); !errors.Is(err, hia.ErrInvalidStatus) {
			t.Fatalf("%q accepted", s)
		}
	}
}

func TestMergeRules(t *testing.T) {
	base, err := hia.NewProvisional(nil, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	self := base
	self.Status, self.MergedIntoHiaID = hia.StatusMerged, ptr(base.ID)
	if err = self.Validate(); !errors.Is(err, hia.ErrMergeIntoSelf) {
		t.Fatalf("merge into self must be rejected, got %v", err)
	}
	noTarget := base
	noTarget.Status = hia.StatusMerged
	if err = noTarget.Validate(); !errors.Is(err, hia.ErrMergeTargetMissing) {
		t.Fatalf("MERGED without target must be rejected, got %v", err)
	}
	notMerged := base
	notMerged.MergedIntoHiaID = ptr(uuid.NewString())
	if err = notMerged.Validate(); !errors.Is(err, hia.ErrMergeTargetMissing) {
		t.Fatalf("target without MERGED must be rejected, got %v", err)
	}
	ok := base
	ok.Status, ok.MergedIntoHiaID = hia.StatusMerged, ptr(uuid.NewString())
	if err = ok.Validate(); err != nil {
		t.Fatalf("valid merge rejected: %v", err)
	}
}

func TestNicknameLength(t *testing.T) {
	if _, err := hia.NewProvisional(ptr("   "), nil, now); !errors.Is(err, hia.ErrInvalidNickname) {
		t.Fatalf("blank nickname accepted: %v", err)
	}
	long := make([]rune, 61)
	for i := range long {
		long[i] = 'ก'
	}
	if _, err := hia.NewProvisional(ptr(string(long)), nil, now); !errors.Is(err, hia.ErrInvalidNickname) {
		t.Fatalf("61-character nickname accepted: %v", err)
	}
}

func TestPublicCodeFormat(t *testing.T) {
	cases := map[int64]string{1: "HIA-000001", 42: "HIA-000042", 999999: "HIA-999999", 1000000: "HIA-1000000"}
	for n, want := range cases {
		if got := hia.FormatPublicCode(n); got != want {
			t.Fatalf("FormatPublicCode(%d)=%s want %s", n, got, want)
		}
		if err := hia.ValidatePublicCode(want); err != nil {
			t.Fatalf("%s rejected: %v", want, err)
		}
	}
	for _, bad := range []string{"", "HIA-1", "hia-000001", "HIA-00000A", "HIA-000001 ", "000001"} {
		if err := hia.ValidatePublicCode(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}
