package encounter_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nawariso/toem-hia/services/api/internal/domain/encounter"
)

var (
	captured = time.Date(2026, 9, 25, 7, 30, 0, 0, time.FixedZone("ICT", 7*3600))
	created  = time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
)

func ptr[T any](v T) *T { return &v }

func TestNewDraftSeparatesCapturedAtFromCreatedAt(t *testing.T) {
	observer := uuid.NewString()
	e, err := encounter.NewDraft(observer, encounter.Details{CapturedAt: captured, Behavior: ptr(encounter.BehaviorBasking), Notes: ptr("  near the lake ")}, created)
	if err != nil {
		t.Fatal(err)
	}
	if e.Status != encounter.StatusDraft || e.ObserverUserID != observer || e.SubmittedAt != nil {
		t.Fatalf("unexpected draft: %+v", e)
	}
	if !e.CapturedAt.Equal(captured) || e.CapturedAt.Location() != time.UTC || !e.CreatedAt.Equal(created) || e.CapturedAt.Equal(e.CreatedAt) {
		t.Fatalf("capturedAt must be the observation time, normalised to UTC, and distinct from createdAt: %+v", e)
	}
	if *e.Notes != "near the lake" {
		t.Fatalf("notes not trimmed: %q", *e.Notes)
	}
}

func TestNewDraftRequiresServerObserver(t *testing.T) {
	if _, err := encounter.NewDraft("", encounter.Details{CapturedAt: captured}, created); !errors.Is(err, encounter.ErrObserverRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestDetailsValidation(t *testing.T) {
	park, zone := uuid.NewString(), uuid.NewString()
	cases := map[string]struct {
		d    encounter.Details
		want error
	}{
		"missing capturedAt": {encounter.Details{}, encounter.ErrCapturedAtRequired},
		"invalid behavior":   {encounter.Details{CapturedAt: captured, Behavior: ptr("DANCING")}, encounter.ErrInvalidBehavior},
		"lowercase behavior": {encounter.Details{CapturedAt: captured, Behavior: ptr("basking")}, encounter.ErrInvalidBehavior},
		"zone without park":  {encounter.Details{CapturedAt: captured, ZoneID: &zone}, encounter.ErrZoneRequiresPark},
		"non-uuid park":      {encounter.Details{CapturedAt: captured, ParkID: ptr("lumpini")}, encounter.ErrInvalidReference},
		"notes too long":     {encounter.Details{CapturedAt: captured, Notes: ptr(strings.Repeat("ก", 2001))}, encounter.ErrNotesTooLong},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := tc.d.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
	ok := encounter.Details{CapturedAt: captured, ParkID: &park, ZoneID: &zone, Behavior: ptr(encounter.BehaviorOther), Notes: ptr(strings.Repeat("ก", 2000))}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid details rejected: %v", err)
	}
	for _, b := range []string{"BASKING", "SWIMMING", "WALKING", "RESTING", "EATING", "CLIMBING", "OTHER"} {
		if !encounter.ValidBehavior(b) {
			t.Fatalf("%s rejected", b)
		}
	}
}

func TestSubmitTransitions(t *testing.T) {
	e, err := encounter.NewDraft(uuid.NewString(), encounter.Details{CapturedAt: captured}, created)
	if err != nil {
		t.Fatal(err)
	}
	first := created.Add(time.Minute)
	changed, err := e.Submit(first)
	if err != nil || !changed || e.Status != encounter.StatusSubmitted || !e.SubmittedAt.Equal(first) {
		t.Fatalf("DRAFT -> SUBMITTED failed: %v %+v", err, e)
	}
	changed, err = e.Submit(first.Add(time.Hour))
	if err != nil || changed || !e.SubmittedAt.Equal(first) || !e.UpdatedAt.Equal(first) {
		t.Fatalf("SUBMITTED -> SUBMITTED must be a deterministic no-op: %v %+v", err, e)
	}
	if err = e.Edit(encounter.Details{CapturedAt: captured}, first); !errors.Is(err, encounter.ErrNotEditable) {
		t.Fatalf("a SUBMITTED encounter must not be editable, got %v", err)
	}
	for _, s := range []string{encounter.StatusProcessing, encounter.StatusNeedsReview, encounter.StatusConfirmed, encounter.StatusRejected} {
		other := e
		other.Status = s
		if _, err = other.Submit(first); !errors.Is(err, encounter.ErrInvalidTransition) {
			t.Fatalf("%s -> SUBMITTED must be rejected, got %v", s, err)
		}
	}
	unknown := e
	unknown.Status = "WHATEVER"
	if _, err = unknown.Submit(first); !errors.Is(err, encounter.ErrUnknownStatus) {
		t.Fatalf("unknown status must be rejected, got %v", err)
	}
}

func TestEditDraft(t *testing.T) {
	e, err := encounter.NewDraft(uuid.NewString(), encounter.Details{CapturedAt: captured}, created)
	if err != nil {
		t.Fatal(err)
	}
	later := created.Add(time.Hour)
	if err = e.Edit(encounter.Details{CapturedAt: captured, Behavior: ptr("SWIMMING"), Notes: ptr("   ")}, later); err != nil {
		t.Fatal(err)
	}
	if *e.Behavior != "SWIMMING" || e.Notes != nil || !e.UpdatedAt.Equal(later) || !e.CreatedAt.Equal(created) {
		t.Fatalf("unexpected edit result: %+v", e)
	}
	before := e
	if err = e.Edit(encounter.Details{CapturedAt: captured, Behavior: ptr("FLYING")}, later.Add(time.Hour)); !errors.Is(err, encounter.ErrInvalidBehavior) {
		t.Fatalf("got %v", err)
	}
	if !e.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("a rejected edit must not change updatedAt")
	}
}

func TestOwnership(t *testing.T) {
	owner := uuid.NewString()
	e, err := encounter.NewDraft(owner, encounter.Details{CapturedAt: captured}, created)
	if err != nil {
		t.Fatal(err)
	}
	if !e.OwnedBy(owner) || e.OwnedBy(uuid.NewString()) || e.OwnedBy("") {
		t.Fatal("ownership check is wrong")
	}
}
