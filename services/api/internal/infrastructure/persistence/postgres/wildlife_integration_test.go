package postgres_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nawariso/toem-here/services/api/internal/application"
	"github.com/nawariso/toem-here/services/api/internal/domain"
	"github.com/nawariso/toem-here/services/api/internal/domain/encounter"
	"github.com/nawariso/toem-here/services/api/internal/domain/hia"
	"github.com/nawariso/toem-here/services/api/internal/domain/location"
	"github.com/nawariso/toem-here/services/api/internal/domain/park"
	persistence "github.com/nawariso/toem-here/services/api/internal/infrastructure/persistence/postgres"
	"github.com/nawariso/toem-here/services/api/internal/infrastructure/persistence/postgres/migrations"
	"github.com/nawariso/toem-here/services/api/internal/infrastructure/seed"
	"github.com/nawariso/toem-here/services/api/internal/testsupport"
)

func ptr[T any](v T) *T { return &v }

func tableExists(t *testing.T, pool *pgxpool.Pool, table string) bool {
	t.Helper()
	var found *string
	if err := pool.QueryRow(t.Context(), "SELECT to_regclass(current_schema() || '.' || $1)::text", table).Scan(&found); err != nil {
		t.Fatal(err)
	}
	return found != nil
}

var wildlifeTables = []string{"parks", "zones", "hias", "encounters", "encounter_locations"}

// Requirement 002 §37: 000001 -> 000002 up -> down -> up, and the identity
// data written under 000001 survives every step.
func TestWildlifeMigrationUpgradesAndRollsBackWithoutTouchingIdentity(t *testing.T) {
	pool := testsupport.EmptyDatabase(t)
	ctx := t.Context()
	if err := migrations.UpTo(ctx, pool, 1); err != nil {
		t.Fatal(err)
	}
	for _, table := range wildlifeTables {
		if tableExists(t, pool, table) {
			t.Fatalf("%s exists at version 1", table)
		}
	}
	users := persistence.NewRepository(pool)
	identity := domain.ExternalIdentity{Provider: "LOCAL_DEV", Subject: "developer-001"}
	before, err := users.Bootstrap(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}

	check := func(step string, wantWildlife bool) {
		t.Helper()
		for _, table := range wildlifeTables {
			if tableExists(t, pool, table) != wantWildlife {
				t.Fatalf("%s: table %s exists=%t", step, table, !wantWildlife)
			}
		}
		after, err := users.FindByIdentity(ctx, identity)
		if err != nil || after.ID != before.ID {
			t.Fatalf("%s: identity data lost: %v %+v", step, err, after)
		}
	}
	if err = migrations.Up(ctx, pool); err != nil {
		t.Fatalf("up to 2: %v", err)
	}
	check("up", true)
	if err = migrations.Up(ctx, pool); err != nil {
		t.Fatalf("re-running up must be idempotent: %v", err)
	}
	check("up again", true)
	if err = migrations.DownTo(ctx, pool, 1); err != nil {
		t.Fatalf("down to 1: %v", err)
	}
	check("down", false)
	if err = migrations.Up(ctx, pool); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	check("re-up", true)

	var postgis string
	if err = pool.QueryRow(ctx, "SELECT public.postgis_lib_version()").Scan(&postgis); err != nil || postgis == "" {
		t.Fatalf("PostGIS unavailable: %v", err)
	}
}

type world struct {
	pool        *pgxpool.Pool
	parks       *persistence.ParkRepository
	hias        *persistence.HiaRepository
	encounters  *persistence.EncounterRepository
	userID      string
	park, park2 park.Park
	zone        park.Zone
}

func newWorld(t *testing.T) *world {
	t.Helper()
	pool := testsupport.Database(t)
	w := &world{pool: pool, parks: persistence.NewParkRepository(pool), hias: persistence.NewHiaRepository(pool), encounters: persistence.NewEncounterRepository(pool)}
	u, err := persistence.NewRepository(pool).Bootstrap(t.Context(), domain.ExternalIdentity{Provider: "LOCAL_DEV", Subject: "developer-001"})
	if err != nil {
		t.Fatal(err)
	}
	w.userID = u.ID
	now := time.Now()
	w.park, _ = park.NewPark(park.ParkInput{Slug: "lumpini-park", Name: "Lumpini Park", City: "Bangkok", CountryCode: "TH", Timezone: "Asia/Bangkok", Status: park.StatusActive}, now)
	w.park2, _ = park.NewPark(park.ParkInput{Slug: "benjakitti-park", Name: "Benjakitti Park", City: "Bangkok", CountryCode: "TH", Timezone: "Asia/Bangkok", Status: park.StatusActive}, now)
	w.zone, _ = park.NewZone(park.ZoneInput{ParkID: w.park.ID, Slug: "lake-zone", Name: "Lake Zone", Status: park.StatusActive}, now)
	for _, p := range []park.Park{w.park, w.park2} {
		if err = w.parks.CreatePark(t.Context(), p); err != nil {
			t.Fatal(err)
		}
	}
	if err = w.parks.CreateZone(t.Context(), w.zone); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestParkAndZoneConstraints(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	dup, _ := park.NewPark(park.ParkInput{Slug: "LUMPINI-PARK", Name: "Dup", City: "Bangkok", CountryCode: "TH", Timezone: "Asia/Bangkok", Status: park.StatusActive}, time.Now())
	dup.Slug = "LUMPINI-PARK" // bypass normalisation: the unique index must be case-insensitive by itself
	if err := w.parks.CreatePark(ctx, dup); err == nil {
		t.Fatal("upper-case slug must be rejected (format) or collide (uniqueness)")
	}
	dup.Slug = "lumpini-park"
	if err := w.parks.CreatePark(ctx, dup); !errors.Is(err, park.ErrSlugTaken) {
		t.Fatalf("duplicate slug: got %v", err)
	}
	sameInPark, _ := park.NewZone(park.ZoneInput{ParkID: w.park.ID, Slug: "lake-zone", Name: "Again", Status: park.StatusActive}, time.Now())
	if err := w.parks.CreateZone(ctx, sameInPark); !errors.Is(err, park.ErrZoneSlugTaken) {
		t.Fatalf("duplicate zone slug in the same park: got %v", err)
	}
	otherPark, _ := park.NewZone(park.ZoneInput{ParkID: w.park2.ID, Slug: "lake-zone", Name: "Lake Zone", Status: park.StatusActive}, time.Now())
	if err := w.parks.CreateZone(ctx, otherPark); err != nil {
		t.Fatalf("the same zone slug in a different park must be allowed: %v", err)
	}
	orphan, _ := park.NewZone(park.ZoneInput{ParkID: uuid.NewString(), Slug: "x", Name: "X", Status: park.StatusActive}, time.Now())
	if err := w.parks.CreateZone(ctx, orphan); !errors.Is(err, park.ErrNotFound) {
		t.Fatalf("zone for a missing park: got %v", err)
	}
	if _, err := w.pool.Exec(ctx, `UPDATE parks SET status='OPEN' WHERE id=$1`, w.park.ID); err == nil {
		t.Fatal("invalid park status accepted by the database")
	}
	if _, err := w.pool.Exec(ctx, `UPDATE parks SET country_code='tha' WHERE id=$1`, w.park.ID); err == nil {
		t.Fatal("invalid country code accepted by the database")
	}
	// Optional MultiPolygon boundary in SRID 4326.
	if _, err := w.pool.Exec(ctx, `UPDATE zones SET boundary = public.ST_GeomFromText('MULTIPOLYGON(((100.54 13.73,100.55 13.73,100.55 13.74,100.54 13.73)))', 4326) WHERE id=$1`, w.zone.ID); err != nil {
		t.Fatalf("valid 4326 boundary rejected: %v", err)
	}
	if _, err := w.pool.Exec(ctx, `UPDATE zones SET boundary = public.ST_GeomFromText('MULTIPOLYGON(((0 0,1 0,1 1,0 0)))', 3857) WHERE id=$1`, w.zone.ID); err == nil {
		t.Fatal("boundary with the wrong SRID accepted")
	}
	inactive, _ := park.NewPark(park.ParkInput{Slug: "closed-park", Name: "Closed", City: "Bangkok", CountryCode: "TH", Timezone: "Asia/Bangkok", Status: park.StatusInactive}, time.Now())
	if err := w.parks.CreatePark(ctx, inactive); err != nil {
		t.Fatal(err)
	}
	active, err := w.parks.ListActiveParks(ctx)
	if err != nil || len(active) != 2 {
		t.Fatalf("only ACTIVE parks are public: %v %d", err, len(active))
	}
	if _, err = w.parks.FindActivePark(ctx, inactive.ID); !errors.Is(err, park.ErrNotFound) {
		t.Fatalf("inactive park returned: %v", err)
	}
}

func TestHiaPublicCodesAreSequentialUniqueAndImmutable(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	h1, _ := hia.NewProvisional(ptr("First"), &w.park.ID, time.Now())
	h1.PublicCode = "HIA-999999" // a caller-supplied code must be ignored
	first, err := w.hias.Create(ctx, h1)
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := hia.NewProvisional(nil, nil, time.Now())
	second, err := w.hias.Create(ctx, h2)
	if err != nil {
		t.Fatal(err)
	}
	if first.PublicCode != "HIA-000001" || second.PublicCode != "HIA-000002" {
		t.Fatalf("codes: %s %s", first.PublicCode, second.PublicCode)
	}
	if first.PublicCode != hia.FormatPublicCode(first.PublicNumber) {
		t.Fatal("database and Go formatting disagree")
	}
	if _, err = w.pool.Exec(ctx, `UPDATE hias SET public_number = 77 WHERE id=$1`, first.ID); err == nil {
		t.Fatal("public number must be immutable")
	}
	if _, err = w.pool.Exec(ctx, `UPDATE hias SET nickname = 'Renamed' WHERE id=$1`, first.ID); err != nil {
		t.Fatalf("mutable fields must still update: %v", err)
	}
	// Deleting a hia must not free its code for reuse.
	if _, err = w.pool.Exec(ctx, `DELETE FROM hias WHERE id=$1`, second.ID); err != nil {
		t.Fatal(err)
	}
	h3, _ := hia.NewProvisional(nil, nil, time.Now())
	third, err := w.hias.Create(ctx, h3)
	if err != nil || third.PublicCode != "HIA-000003" {
		t.Fatalf("codes must never be reused: %v %s", err, third.PublicCode)
	}
	// Merge constraints at the database level.
	if _, err = w.pool.Exec(ctx, `UPDATE hias SET status='MERGED', merged_into_hia_id=id WHERE id=$1`, first.ID); err == nil {
		t.Fatal("merge into self accepted")
	}
	if _, err = w.pool.Exec(ctx, `UPDATE hias SET status='MERGED' WHERE id=$1`, first.ID); err == nil {
		t.Fatal("MERGED without target accepted")
	}
	if _, err = w.pool.Exec(ctx, `UPDATE hias SET status='MERGED', merged_into_hia_id=$2 WHERE id=$1`, third.ID, uuid.NewString()); err == nil {
		t.Fatal("merge into a missing hia accepted")
	}
	if _, err = w.pool.Exec(ctx, `UPDATE hias SET status='MERGED', merged_into_hia_id=$2 WHERE id=$1`, third.ID, first.ID); err != nil {
		t.Fatalf("valid merge rejected: %v", err)
	}
	codes, err := w.hias.PublicCodes(ctx, []string{first.ID})
	if err != nil || codes[first.ID] != "HIA-000001" {
		t.Fatalf("public codes: %v %v", err, codes)
	}
	byPark, err := w.hias.List(ctx, &w.park.ID)
	if err != nil || len(byPark) != 1 || byPark[0].ID != first.ID {
		t.Fatalf("parkId filter: %v %+v", err, byPark)
	}
	if _, err = w.hias.FindByPublicCode(ctx, "HIA-000002"); !errors.Is(err, hia.ErrNotFound) {
		t.Fatalf("deleted code: got %v", err)
	}
}

func TestConcurrentHiaCreationNeverDuplicatesPublicCodes(t *testing.T) {
	w := newWorld(t)
	const n = 40
	codes := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			h, err := hia.NewProvisional(nil, nil, time.Now())
			if err != nil {
				errs[i] = err
				return
			}
			created, err := w.hias.Create(context.Background(), h)
			codes[i], errs[i] = created.PublicCode, err
		})
	}
	wg.Wait()
	seen := map[string]bool{}
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("create %d: %v", i, errs[i])
		}
		if seen[codes[i]] {
			t.Fatalf("duplicate public code %s", codes[i])
		}
		seen[codes[i]] = true
	}
	sort.Strings(codes)
	if codes[0] != "HIA-000001" || codes[n-1] != hia.FormatPublicCode(n) {
		t.Fatalf("codes are not a gap-free 1..%d range: %s..%s", n, codes[0], codes[n-1])
	}
}

func newDraft(t *testing.T, w *world, withZone bool) encounter.Encounter {
	t.Helper()
	d := encounter.Details{CapturedAt: time.Now().Add(-time.Hour), ParkID: &w.park.ID, Behavior: ptr("BASKING")}
	if withZone {
		d.ZoneID = &w.zone.ID
	}
	e, err := encounter.NewDraft(w.userID, d, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestEncounterWithPrivateLocationRoundTrips(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	e := newDraft(t, w, true)
	loc := location.Private{Latitude: 13.7307, Longitude: 100.5418, AccuracyMeters: ptr(4.5), Source: location.SourceGPS}
	if err := w.encounters.Create(ctx, e, &loc); err != nil {
		t.Fatal(err)
	}
	var lat, lon, accuracy float64
	var srid int
	var source, geomType string
	if err := w.pool.QueryRow(ctx, `SELECT public.ST_Y(point::public.geometry), public.ST_X(point::public.geometry), public.ST_SRID(point),
		public.GeometryType(point::public.geometry), accuracy_meters, source FROM encounter_locations WHERE encounter_id=$1`, e.ID).
		Scan(&lat, &lon, &srid, &geomType, &accuracy, &source); err != nil {
		t.Fatal(err)
	}
	if lat != 13.7307 || lon != 100.5418 || srid != 4326 || geomType != "POINT" || accuracy != 4.5 || source != "GPS" {
		t.Fatalf("stored location: lat=%v lon=%v srid=%d type=%s acc=%v source=%s", lat, lon, srid, geomType, accuracy, source)
	}
	found, err := w.encounters.FindByID(ctx, e.ID)
	if err != nil || found.ObserverUserID != w.userID || !found.CapturedAt.Equal(e.CapturedAt) || found.Status != encounter.StatusDraft {
		t.Fatalf("encounter round trip: %v %+v", err, found)
	}
	// A spatial query works against the geography column.
	var within bool
	if err = w.pool.QueryRow(ctx, `SELECT public.ST_DWithin(point, public.ST_SetSRID(public.ST_MakePoint(100.5418, 13.7308), 4326)::public.geography, 50)
		FROM encounter_locations WHERE encounter_id=$1`, e.ID).Scan(&within); err != nil || !within {
		t.Fatalf("ST_DWithin: %v %t", err, within)
	}
}

func TestEncounterDatabaseConstraints(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	otherZone, _ := park.NewZone(park.ZoneInput{ParkID: w.park2.ID, Slug: "north", Name: "North", Status: park.StatusActive}, time.Now())
	if err := w.parks.CreateZone(ctx, otherZone); err != nil {
		t.Fatal(err)
	}
	mismatch := newDraft(t, w, false)
	mismatch.ZoneID = &otherZone.ID
	if err := w.encounters.Create(ctx, mismatch, nil); !errors.Is(err, encounter.ErrZoneNotInPark) {
		t.Fatalf("composite FK must reject a zone from another park, got %v", err)
	}
	bad := newDraft(t, w, false)
	bad.Behavior = ptr("DANCING")
	if err := w.encounters.Create(ctx, bad, nil); !errors.Is(err, encounter.ErrInvalidBehavior) {
		t.Fatalf("behavior check: got %v", err)
	}
	e := newDraft(t, w, false)
	outOfRange := location.Private{Latitude: 91, Longitude: 0, Source: "GPS"}
	if err := w.encounters.Create(ctx, e, &outOfRange); err == nil {
		t.Fatal("out-of-range latitude accepted")
	}
	if _, err := w.encounters.FindByID(ctx, e.ID); !errors.Is(err, encounter.ErrNotFound) {
		t.Fatal("encounter insert was not rolled back with its invalid location")
	}
	for _, stmt := range []string{
		`INSERT INTO encounter_locations (encounter_id, point, source) VALUES ($1, encounter_location_point(0, 95), 'GPS')`,
		`INSERT INTO encounter_locations (encounter_id, point, source) VALUES ($1, encounter_location_point(190, 10), 'GPS')`,
		`INSERT INTO encounter_locations (encounter_id, point, source) VALUES ($1, encounter_location_point('NaN', 10), 'GPS')`,
		`INSERT INTO encounter_locations (encounter_id, point, accuracy_meters, source) VALUES ($1, encounter_location_point(0, 0), -1, 'GPS')`,
		`INSERT INTO encounter_locations (encounter_id, point, accuracy_meters, source) VALUES ($1, encounter_location_point(0, 0), 'Infinity', 'GPS')`,
		`INSERT INTO encounter_locations (encounter_id, point, source) VALUES ($1, encounter_location_point(0, 0), 'SATELLITE')`,
		`INSERT INTO encounter_locations (encounter_id, point, source) VALUES ($1, public.ST_SetSRID(public.ST_MakePoint(0, 0), 3857)::public.geography, 'GPS')`,
	} {
		ok := newDraft(t, w, false)
		if err := w.encounters.Create(ctx, ok, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := w.pool.Exec(ctx, stmt, ok.ID); err == nil {
			t.Fatalf("database accepted: %s", stmt)
		}
	}
	if _, err := w.pool.Exec(ctx, `INSERT INTO encounters (id, observer_user_id, captured_at, status) VALUES ($1, $2, now(), 'DRAFT')`, uuid.NewString(), uuid.NewString()); err == nil {
		t.Fatal("encounter for a missing user accepted")
	}
	if _, err := w.pool.Exec(ctx, `INSERT INTO encounters (id, observer_user_id, captured_at, status) VALUES ($1, $2, now(), 'SUBMITTED')`, uuid.NewString(), w.userID); err == nil {
		t.Fatal("SUBMITTED without submitted_at accepted")
	}
	var hasHiaColumn bool
	if err := w.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema()
		AND table_name = 'encounters' AND column_name LIKE '%hia%')`).Scan(&hasHiaColumn); err != nil || hasHiaColumn {
		t.Fatalf("encounters must not carry a hia reference: %v %t", err, hasHiaColumn)
	}
	var timestamptz int
	if err := w.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema()
		AND table_name IN ('parks','zones','hias','encounters','encounter_locations') AND column_name LIKE '%\_at'
		AND data_type <> 'timestamp with time zone'`).Scan(&timestamptz); err != nil || timestamptz != 0 {
		t.Fatalf("every *_at column must be TIMESTAMPTZ: %v %d", err, timestamptz)
	}
}

func TestUpdateReplacesAndRemovesPrivateLocation(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	e := newDraft(t, w, false)
	if err := w.encounters.Create(ctx, e, &location.Private{Latitude: 1, Longitude: 2, Source: "GPS"}); err != nil {
		t.Fatal(err)
	}
	count := func() (n int) {
		_ = w.pool.QueryRow(ctx, `SELECT count(*) FROM encounter_locations WHERE encounter_id=$1`, e.ID).Scan(&n)
		return n
	}
	if _, err := w.encounters.Update(ctx, e.ID, func(*encounter.Encounter) (application.LocationChange, error) {
		return application.LocationChange{Set: true, Value: &location.Private{Latitude: 3, Longitude: 4, Source: "MANUAL"}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	var lat float64
	if err := w.pool.QueryRow(ctx, `SELECT public.ST_Y(point::public.geometry) FROM encounter_locations WHERE encounter_id=$1`, e.ID).Scan(&lat); err != nil || lat != 3 || count() != 1 {
		t.Fatalf("replace: %v lat=%v rows=%d", err, lat, count())
	}
	if _, err := w.encounters.Update(ctx, e.ID, func(*encounter.Encounter) (application.LocationChange, error) {
		return application.LocationChange{Set: true}, nil
	}); err != nil || count() != 0 {
		t.Fatalf("remove: %v rows=%d", err, count())
	}
	boom := errors.New("abort")
	if _, err := w.encounters.Update(ctx, e.ID, func(x *encounter.Encounter) (application.LocationChange, error) {
		x.Notes = ptr("should not persist")
		return application.LocationChange{}, boom
	}); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if got, _ := w.encounters.FindByID(ctx, e.ID); got.Notes != nil {
		t.Fatal("aborted update was persisted")
	}
}

// Two submits racing on the same DRAFT: exactly one transitions, the other
// observes SUBMITTED and returns the same submittedAt.
func TestConcurrentSubmitTransitionsExactlyOnce(t *testing.T) {
	w := newWorld(t)
	e := newDraft(t, w, false)
	if err := w.encounters.Create(t.Context(), e, nil); err != nil {
		t.Fatal(err)
	}
	const n = 8
	changed := make([]bool, n)
	results := make([]encounter.Encounter, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			results[i], errs[i] = w.encounters.Update(context.Background(), e.ID, func(x *encounter.Encounter) (application.LocationChange, error) {
				var err error
				changed[i], err = x.Submit(time.Now())
				return application.LocationChange{}, err
			})
		})
	}
	wg.Wait()
	transitions := 0
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("submit %d: %v", i, errs[i])
		}
		if changed[i] {
			transitions++
		}
		if !results[i].SubmittedAt.Equal(*results[0].SubmittedAt) {
			t.Fatal("submits disagree on submittedAt")
		}
	}
	if transitions != 1 {
		t.Fatalf("expected exactly one DRAFT -> SUBMITTED transition, got %d", transitions)
	}
}

func TestDevelopmentSeedIsIdempotentAndRefusesProduction(t *testing.T) {
	pool := testsupport.Database(t)
	ctx := t.Context()
	parks := persistence.NewParkRepository(pool)
	for _, env := range []string{"production", "", "staging", "Development"} {
		if _, err := seed.Run(ctx, env, parks, time.Now()); !errors.Is(err, seed.ErrForbiddenEnvironment) {
			t.Fatalf("APP_ENV=%q: got %v", env, err)
		}
	}
	first, err := seed.Run(ctx, "development", parks, time.Now())
	if err != nil || !first.ParkCreated || first.ZonesCreated != 3 {
		t.Fatalf("first run: %v %+v", err, first)
	}
	second, err := seed.Run(ctx, "test", parks, time.Now())
	if err != nil || second.ParkCreated || second.ZonesCreated != 0 || second.ZonesExisting != 3 || second.ParkID != first.ParkID {
		t.Fatalf("second run must insert nothing: %v %+v", err, second)
	}
	zones, err := parks.ListActiveZones(ctx, first.ParkID)
	if err != nil || len(zones) != 3 {
		t.Fatalf("zones: %v %d", err, len(zones))
	}
	names := []string{zones[0].Name, zones[1].Name, zones[2].Name}
	if names[0] != "Lake Zone" || names[1] != "North Path" || names[2] != "South Pond" {
		t.Fatalf("zones: %v", names)
	}
	var hiaCount, parkCount int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM hias), (SELECT count(*) FROM parks)`).Scan(&hiaCount, &parkCount); err != nil {
		t.Fatal(err)
	}
	if hiaCount != 0 || parkCount != 1 {
		t.Fatalf("seed must create one park and no hias: parks=%d hias=%d", parkCount, hiaCount)
	}
}
