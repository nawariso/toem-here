package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nawariso/toem-hia/services/api/internal/application"
	"github.com/nawariso/toem-hia/services/api/internal/domain"
	"github.com/nawariso/toem-hia/services/api/internal/domain/encounter"
	"github.com/nawariso/toem-hia/services/api/internal/domain/hia"
	"github.com/nawariso/toem-hia/services/api/internal/domain/location"
	"github.com/nawariso/toem-hia/services/api/internal/domain/park"
)

// ------------------------------------------------------------------ fakes ---

type fakeUsers map[string]domain.User // keyed by subject

func (f fakeUsers) Current(_ context.Context, id domain.ExternalIdentity) (domain.User, error) {
	u, ok := f[id.Subject]
	if !ok {
		return domain.User{}, application.ErrNotFound
	}
	return u, nil
}

type fakeParks struct {
	parks map[string]park.Park
	zones map[string]park.Zone
}

func (f *fakeParks) CreatePark(context.Context, park.Park) error { return nil }
func (f *fakeParks) CreateZone(context.Context, park.Zone) error { return nil }
func (f *fakeParks) ListActiveParks(context.Context) ([]park.Park, error) {
	out := []park.Park{}
	for _, p := range f.parks {
		if p.Status == park.StatusActive {
			out = append(out, p)
		}
	}
	return out, nil
}
func (f *fakeParks) FindActivePark(_ context.Context, id string) (park.Park, error) {
	p, ok := f.parks[id]
	if !ok || p.Status != park.StatusActive {
		return park.Park{}, park.ErrNotFound
	}
	return p, nil
}
func (f *fakeParks) ListActiveZones(_ context.Context, parkID string) ([]park.Zone, error) {
	out := []park.Zone{}
	for _, z := range f.zones {
		if z.ParkID == parkID && z.Status == park.StatusActive {
			out = append(out, z)
		}
	}
	return out, nil
}
func (f *fakeParks) FindParks(_ context.Context, ids []string) (map[string]park.Park, error) {
	out := map[string]park.Park{}
	for _, id := range ids {
		if p, ok := f.parks[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}
func (f *fakeParks) FindZones(_ context.Context, ids []string) (map[string]park.Zone, error) {
	out := map[string]park.Zone{}
	for _, id := range ids {
		if z, ok := f.zones[id]; ok {
			out[id] = z
		}
	}
	return out, nil
}

type fakeEncounters struct {
	mu        sync.Mutex
	rows      map[string]encounter.Encounter
	locations map[string]location.Private
}

func (f *fakeEncounters) Create(_ context.Context, e encounter.Encounter, loc *location.Private) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[e.ID] = e
	if loc != nil {
		f.locations[e.ID] = *loc
	}
	return nil
}
func (f *fakeEncounters) FindByID(_ context.Context, id string) (encounter.Encounter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.rows[id]
	if !ok {
		return encounter.Encounter{}, encounter.ErrNotFound
	}
	return e, nil
}
func (f *fakeEncounters) ListByObserver(_ context.Context, observer string) ([]encounter.Encounter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []encounter.Encounter{}
	for _, e := range f.rows {
		if e.ObserverUserID == observer {
			out = append(out, e)
		}
	}
	return out, nil
}
func (f *fakeEncounters) Update(_ context.Context, id string, mutate func(*encounter.Encounter) (application.LocationChange, error)) (encounter.Encounter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.rows[id]
	if !ok {
		return encounter.Encounter{}, encounter.ErrNotFound
	}
	change, err := mutate(&e)
	if err != nil {
		return encounter.Encounter{}, err
	}
	f.rows[id] = e
	if change.Set {
		if change.Value == nil {
			delete(f.locations, id)
		} else {
			f.locations[id] = *change.Value
		}
	}
	return e, nil
}

// ---------------------------------------------------------------- fixture ---

type fixture struct {
	svc                *application.EncounterService
	repo               *fakeEncounters
	parkA, parkB       park.Park
	zoneA              park.Zone
	activeUserID       string
	active, other      domain.ExternalIdentity
	suspended, deleted domain.ExternalIdentity
	notBootstrapped    domain.ExternalIdentity
}

func ident(subject string) domain.ExternalIdentity {
	return domain.ExternalIdentity{Provider: "LOCAL_DEV", Subject: subject}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	now := time.Now()
	pa, _ := park.NewPark(park.ParkInput{Slug: "a", Name: "A", City: "Bangkok", CountryCode: "TH", Timezone: "Asia/Bangkok", Status: park.StatusActive}, now)
	pb, _ := park.NewPark(park.ParkInput{Slug: "b", Name: "B", City: "Bangkok", CountryCode: "TH", Timezone: "Asia/Bangkok", Status: park.StatusActive}, now)
	za, _ := park.NewZone(park.ZoneInput{ParkID: pa.ID, Slug: "lake", Name: "Lake", Status: park.StatusActive}, now)
	users := fakeUsers{
		"active":    {ID: uuid.NewString(), Status: domain.UserStatusActive},
		"other":     {ID: uuid.NewString(), Status: domain.UserStatusActive},
		"suspended": {ID: uuid.NewString(), Status: domain.UserStatusSuspended},
		"deleted":   {ID: uuid.NewString(), Status: domain.UserStatusDeleted},
	}
	parks := &fakeParks{parks: map[string]park.Park{pa.ID: pa, pb.ID: pb}, zones: map[string]park.Zone{za.ID: za}}
	repo := &fakeEncounters{rows: map[string]encounter.Encounter{}, locations: map[string]location.Private{}}
	return &fixture{
		svc: application.NewEncounterService(users, parks, repo), repo: repo, parkA: pa, parkB: pb, zoneA: za,
		activeUserID: users["active"].ID,
		active:       ident("active"), other: ident("other"), suspended: ident("suspended"), deleted: ident("deleted"),
		notBootstrapped: ident("nobody"),
	}
}

func ptr[T any](v T) *T { return &v }

func (f *fixture) input() application.CreateEncounterInput {
	return application.CreateEncounterInput{
		Details:  encounter.Details{CapturedAt: time.Now().Add(-time.Hour), ParkID: &f.parkA.ID, ZoneID: &f.zoneA.ID, Behavior: ptr("BASKING")},
		Location: &location.Private{Latitude: 13.73, Longitude: 100.54, AccuracyMeters: ptr(5.0), Source: location.SourceGPS},
	}
}

// ------------------------------------------------------------------ tests ---

func TestCreateDerivesObserverFromAuthenticatedUser(t *testing.T) {
	f := newFixture(t)
	view, err := f.svc.Create(t.Context(), f.active, f.input())
	if err != nil {
		t.Fatal(err)
	}
	if view.Encounter.ObserverUserID != f.activeUserID {
		t.Fatalf("observer must be the authenticated internal user, got %s", view.Encounter.ObserverUserID)
	}
	if view.Encounter.Status != encounter.StatusDraft || view.Park == nil || view.Zone == nil || view.Park.ID != f.parkA.ID {
		t.Fatalf("unexpected view: %+v", view)
	}
	if _, ok := f.repo.locations[view.Encounter.ID]; !ok {
		t.Fatal("private location was not stored")
	}
}

func TestNonActiveUsersCannotWrite(t *testing.T) {
	f := newFixture(t)
	owned, err := f.svc.Create(t.Context(), f.active, f.input())
	if err != nil {
		t.Fatal(err)
	}
	for name, who := range map[string]domain.ExternalIdentity{"SUSPENDED": f.suspended, "DELETED": f.deleted} {
		t.Run(name, func(t *testing.T) {
			if _, err := f.svc.Create(t.Context(), who, f.input()); !errors.Is(err, domain.ErrUserNotActive) {
				t.Fatalf("create: got %v", err)
			}
			if _, err := f.svc.Update(t.Context(), who, owned.Encounter.ID, application.EncounterPatch{}); !errors.Is(err, domain.ErrUserNotActive) {
				t.Fatalf("update: got %v", err)
			}
			if _, _, err := f.svc.Submit(t.Context(), who, owned.Encounter.ID); !errors.Is(err, domain.ErrUserNotActive) {
				t.Fatalf("submit: got %v", err)
			}
		})
	}
	if len(f.repo.rows) != 1 {
		t.Fatalf("non-active users created rows: %d", len(f.repo.rows))
	}
}

// A suspended user may still read their own history; only writes are blocked.
func TestSuspendedUserCanStillReadOwnEncounters(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.ListMine(t.Context(), f.suspended); err != nil {
		t.Fatalf("read was blocked: %v", err)
	}
}

func TestNotBootstrappedUserIsRejected(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Create(t.Context(), f.notBootstrapped, f.input()); !errors.Is(err, application.ErrUserNotBootstrapped) {
		t.Fatalf("got %v", err)
	}
}

func TestOtherUsersCannotReadEditOrSubmit(t *testing.T) {
	f := newFixture(t)
	owned, err := f.svc.Create(t.Context(), f.active, f.input())
	if err != nil {
		t.Fatal(err)
	}
	id := owned.Encounter.ID
	if _, err = f.svc.Get(t.Context(), f.other, id); !errors.Is(err, encounter.ErrNotFound) {
		t.Fatalf("get: %v", err)
	}
	if _, err = f.svc.Update(t.Context(), f.other, id, application.EncounterPatch{Notes: application.Optional[string]{Set: true, Value: ptr("mine now")}}); !errors.Is(err, encounter.ErrNotFound) {
		t.Fatalf("update: %v", err)
	}
	if _, _, err = f.svc.Submit(t.Context(), f.other, id); !errors.Is(err, encounter.ErrNotFound) {
		t.Fatalf("submit: %v", err)
	}
	list, err := f.svc.ListMine(t.Context(), f.other)
	if err != nil || len(list) != 0 {
		t.Fatalf("other user sees encounters: %v %d", err, len(list))
	}
	if got := f.repo.rows[id]; got.Notes != nil || got.Status != encounter.StatusDraft {
		t.Fatalf("encounter changed by another user: %+v", got)
	}
}

func TestParkZoneMismatchIsRejected(t *testing.T) {
	f := newFixture(t)
	in := f.input()
	in.Details.ParkID = &f.parkB.ID
	if _, err := f.svc.Create(t.Context(), f.active, in); !errors.Is(err, encounter.ErrZoneNotInPark) {
		t.Fatalf("got %v", err)
	}
	in = f.input()
	in.Details.ParkID = ptr(uuid.NewString())
	in.Details.ZoneID = nil
	if _, err := f.svc.Create(t.Context(), f.active, in); !errors.Is(err, encounter.ErrParkNotFound) {
		t.Fatalf("unknown park: got %v", err)
	}
	in = f.input()
	in.Details.ZoneID = ptr(uuid.NewString())
	if _, err := f.svc.Create(t.Context(), f.active, in); !errors.Is(err, encounter.ErrZoneNotFound) {
		t.Fatalf("unknown zone: got %v", err)
	}
}

func TestInvalidCoordinatesAreRejectedBeforePersistence(t *testing.T) {
	f := newFixture(t)
	in := f.input()
	in.Location.Latitude = 91
	if _, err := f.svc.Create(t.Context(), f.active, in); !errors.Is(err, location.ErrInvalidLatitude) {
		t.Fatalf("got %v", err)
	}
	if len(f.repo.rows) != 0 {
		t.Fatal("invalid encounter was persisted")
	}
}

func TestUpdateSubmitLifecycle(t *testing.T) {
	f := newFixture(t)
	created, err := f.svc.Create(t.Context(), f.active, f.input())
	if err != nil {
		t.Fatal(err)
	}
	id := created.Encounter.ID
	// Clearing parkId without clearing zoneId leaves a zone without a park.
	if _, err = f.svc.Update(t.Context(), f.active, id, application.EncounterPatch{ParkID: application.Optional[string]{Set: true}}); !errors.Is(err, encounter.ErrZoneRequiresPark) {
		t.Fatalf("got %v", err)
	}
	updated, err := f.svc.Update(t.Context(), f.active, id, application.EncounterPatch{
		Behavior: application.Optional[string]{Set: true, Value: ptr("SWIMMING")},
		Location: application.LocationChange{Set: true},
	})
	if err != nil || *updated.Encounter.Behavior != "SWIMMING" {
		t.Fatalf("update failed: %v", err)
	}
	if _, ok := f.repo.locations[id]; ok {
		t.Fatal("location: null must remove the private location")
	}
	submitted, changed, err := f.svc.Submit(t.Context(), f.active, id)
	if err != nil || !changed || submitted.Encounter.Status != encounter.StatusSubmitted {
		t.Fatalf("submit failed: %v %t", err, changed)
	}
	again, changed, err := f.svc.Submit(t.Context(), f.active, id)
	if err != nil || changed || !again.Encounter.SubmittedAt.Equal(*submitted.Encounter.SubmittedAt) {
		t.Fatalf("second submit must be a deterministic no-op: %v %t", err, changed)
	}
	if _, err = f.svc.Update(t.Context(), f.active, id, application.EncounterPatch{Notes: application.Optional[string]{Set: true, Value: ptr("late")}}); !errors.Is(err, encounter.ErrNotEditable) {
		t.Fatalf("SUBMITTED edit: got %v", err)
	}
}

func TestMalformedIDsAreNotFound(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.Get(t.Context(), f.active, "../etc"); !errors.Is(err, encounter.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	parks := application.NewParkService(&fakeParks{parks: map[string]park.Park{}, zones: map[string]park.Zone{}})
	if _, err := parks.GetPark(t.Context(), "nope"); !errors.Is(err, park.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestInactiveParksAreNotPublic(t *testing.T) {
	now := time.Now()
	archived, _ := park.NewPark(park.ParkInput{Slug: "old", Name: "Old", City: "X", CountryCode: "TH", Timezone: "Asia/Bangkok", Status: park.StatusArchived}, now)
	svc := application.NewParkService(&fakeParks{parks: map[string]park.Park{archived.ID: archived}, zones: map[string]park.Zone{}})
	if list, _ := svc.ListParks(t.Context()); len(list) != 0 {
		t.Fatal("archived park listed")
	}
	if _, err := svc.GetPark(t.Context(), archived.ID); !errors.Is(err, park.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := svc.ListZones(t.Context(), archived.ID); !errors.Is(err, park.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

type fakeHias struct{ rows []hia.Hia }

func (f *fakeHias) Create(_ context.Context, h hia.Hia) (hia.Hia, error) { return h, nil }
func (f *fakeHias) List(context.Context, *string) ([]hia.Hia, error)     { return f.rows, nil }
func (f *fakeHias) FindByPublicCode(_ context.Context, code string) (hia.Hia, error) {
	for _, h := range f.rows {
		if h.PublicCode == code {
			return h, nil
		}
	}
	return hia.Hia{}, hia.ErrNotFound
}
func (f *fakeHias) PublicCodes(_ context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, h := range f.rows {
		for _, id := range ids {
			if h.ID == id {
				out[id] = h.PublicCode
			}
		}
	}
	return out, nil
}

func TestHiaViewsExposeMergeTargetByPublicCode(t *testing.T) {
	target := hia.Hia{ID: uuid.NewString(), PublicCode: "HIA-000001", Status: hia.StatusConfirmed}
	merged := hia.Hia{ID: uuid.NewString(), PublicCode: "HIA-000002", Status: hia.StatusMerged, MergedIntoHiaID: &target.ID}
	svc := application.NewHiaService(&fakeHias{rows: []hia.Hia{target, merged}})
	v, err := svc.Get(t.Context(), "HIA-000002")
	if err != nil || v.MergedIntoPublicCode == nil || *v.MergedIntoPublicCode != "HIA-000001" {
		t.Fatalf("got %+v %v", v, err)
	}
	if _, err = svc.Get(t.Context(), "not-a-code"); !errors.Is(err, hia.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err = svc.List(t.Context(), ptr("nope")); !errors.Is(err, application.ErrInvalidParkFilter) {
		t.Fatalf("got %v", err)
	}
}
