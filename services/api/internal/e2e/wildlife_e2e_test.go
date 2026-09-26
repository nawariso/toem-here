package e2e_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nawariso/toem-here/services/api/internal/application"
	"github.com/nawariso/toem-here/services/api/internal/domain"
	"github.com/nawariso/toem-here/services/api/internal/domain/hia"
	"github.com/nawariso/toem-here/services/api/internal/infrastructure/config"
	"github.com/nawariso/toem-here/services/api/internal/infrastructure/identity"
	persistence "github.com/nawariso/toem-here/services/api/internal/infrastructure/persistence/postgres"
	"github.com/nawariso/toem-here/services/api/internal/infrastructure/seed"
	"github.com/nawariso/toem-here/services/api/internal/testsupport"
	httptransport "github.com/nawariso/toem-here/services/api/internal/transport/http"
)

// Distinctive coordinates and notes, so a leak anywhere is easy to detect.
const (
	secretLat   = 13.7311234
	secretLon   = 100.5423456
	secretNotes = "private-note-7f3a near the old banyan"
)

// valueMarkers detect the coordinate values themselves (decimal or PostGIS
// WKB/WKT); fieldMarkers detect location fields in a response or log line.
// Validation error messages may name a field but must never echo a value.
var (
	valueMarkers = []string{"13.731", "100.542", "200.542", "POINT", "0101000020"}
	fieldMarkers = []string{"latitude", "longitude", "accuracyMeters"}
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type wildlifeStack struct {
	api    http.Handler
	pool   *pgxpool.Pool
	logs   *syncBuffer
	parkID string
	zones  map[string]string // zone name -> id
}

// newWildlifeStack wires the real server, services, and PostGIS repositories
// behind the AUTH_MODE=local verifier, with the development seed loaded.
func newWildlifeStack(t *testing.T, verifier application.IdentityVerifier) *wildlifeStack {
	t.Helper()
	pool := testsupport.Database(t)
	if verifier == nil {
		var err error
		verifier, err = identity.FromConfig(config.Config{AppEnv: config.EnvTest, AuthMode: config.AuthModeLocal, HTTPPort: "8080", DatabaseURL: "unused"})
		if err != nil {
			t.Fatal(err)
		}
	}
	parks := persistence.NewParkRepository(pool)
	seeded, err := seed.Run(t.Context(), config.EnvTest, parks, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	zones, err := parks.ListActiveZones(t.Context(), seeded.ParkID)
	if err != nil {
		t.Fatal(err)
	}
	s := &wildlifeStack{pool: pool, logs: &syncBuffer{}, parkID: seeded.ParkID, zones: map[string]string{}}
	for _, z := range zones {
		s.zones[z.Name] = z.ID
	}
	repo := persistence.NewRepository(pool)
	users := application.NewUserService(repo)
	logger := slog.New(slog.NewJSONHandler(s.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s.api = httptransport.NewServer(users, verifier, repo, httptransport.WithLogger(logger), httptransport.WithWildlife(
		application.NewParkService(parks),
		application.NewHiaService(persistence.NewHiaRepository(pool)),
		application.NewEncounterService(users, parks, persistence.NewEncounterRepository(pool)),
	)).Handler()
	return s
}

// raw sends body verbatim (string) or JSON-encoded (anything else) and
// returns the status and the undecoded response body.
func (s *wildlifeStack) raw(t *testing.T, method, path, token string, body any) (int, string) {
	t.Helper()
	var reader io.Reader = http.NoBody
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	default:
		encoded, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.api.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func (s *wildlifeStack) call(t *testing.T, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	status, raw := s.raw(t, method, path, token, body)
	out := map[string]any{}
	_ = json.Unmarshal([]byte(raw), &out)
	return status, out
}

func (s *wildlifeStack) bootstrap(t *testing.T, token string) string {
	t.Helper()
	status, body := s.call(t, http.MethodPost, "/v1/auth/bootstrap", token, nil)
	if status != http.StatusOK {
		t.Fatalf("bootstrap: %d %v", status, body)
	}
	return body["id"].(string)
}

func (s *wildlifeStack) encounterBody() map[string]any {
	return map[string]any{
		"capturedAt": "2026-09-25T07:30:00+07:00",
		"parkId":     s.parkID,
		"zoneId":     s.zones["Lake Zone"],
		"behavior":   "BASKING",
		"notes":      secretNotes,
		"location":   map[string]any{"latitude": secretLat, "longitude": secretLon, "accuracyMeters": 4.5, "source": "GPS"},
	}
}

func assertNoLocation(t *testing.T, where, text string, markers ...[]string) {
	t.Helper()
	if len(markers) == 0 {
		markers = [][]string{valueMarkers, fieldMarkers}
	}
	for _, set := range markers {
		for _, marker := range set {
			if strings.Contains(text, marker) {
				t.Fatalf("%s leaked precise location (%q): %s", where, marker, text)
			}
		}
	}
}

const localToken = identity.LocalDevCredential

// Requirement 002 §47: ACTIVE user create -> read -> update draft -> submit ->
// second submit is deterministic, all through AUTH_MODE=local.
func TestWildlifeEncounterLifecycleWithLocalAuth(t *testing.T) {
	s := newWildlifeStack(t, nil)
	userID := s.bootstrap(t, localToken)

	status, created := s.call(t, http.MethodPost, "/v1/encounters", localToken, s.encounterBody())
	if status != http.StatusCreated {
		t.Fatalf("create: %d %v", status, created)
	}
	id := created["id"].(string)
	if created["status"] != "DRAFT" || created["capturedAt"] != "2026-09-25T00:30:00Z" || created["submittedAt"] != nil {
		t.Fatalf("create body: %v", created)
	}
	if p := created["park"].(map[string]any); p["id"] != s.parkID || p["name"] != "Lumpini Park" {
		t.Fatalf("park: %v", created["park"])
	}
	if z := created["zone"].(map[string]any); z["name"] != "Lake Zone" {
		t.Fatalf("zone: %v", created["zone"])
	}
	for _, forbidden := range []string{"observerUserId", "location", "hiaId", "latitude", "longitude"} {
		if _, ok := created[forbidden]; ok {
			t.Fatalf("response exposes %s: %v", forbidden, created)
		}
	}
	var observer string
	if err := s.pool.QueryRow(t.Context(), "SELECT observer_user_id::text FROM encounters WHERE id=$1", id).Scan(&observer); err != nil || observer != userID {
		t.Fatalf("observer must be the server-derived internal user: %v %s", err, observer)
	}

	status, read := s.call(t, http.MethodGet, "/v1/encounters/"+id, localToken, nil)
	if status != http.StatusOK || read["id"] != id {
		t.Fatalf("read: %d %v", status, read)
	}
	status, patched := s.call(t, http.MethodPatch, "/v1/encounters/"+id, localToken,
		map[string]any{"behavior": "SWIMMING", "zoneId": s.zones["South Pond"], "notes": nil})
	if status != http.StatusOK || patched["behavior"] != "SWIMMING" || patched["notes"] != nil ||
		patched["zone"].(map[string]any)["name"] != "South Pond" || patched["capturedAt"] != created["capturedAt"] {
		t.Fatalf("update: %d %v", status, patched)
	}

	status, submitted := s.call(t, http.MethodPost, "/v1/encounters/"+id+"/submit", localToken, nil)
	if status != http.StatusOK || submitted["status"] != "SUBMITTED" || submitted["submittedAt"] == nil {
		t.Fatalf("submit: %d %v", status, submitted)
	}
	status, again := s.call(t, http.MethodPost, "/v1/encounters/"+id+"/submit", localToken, nil)
	if status != http.StatusOK || again["status"] != "SUBMITTED" || again["submittedAt"] != submitted["submittedAt"] || again["updatedAt"] != submitted["updatedAt"] {
		t.Fatalf("second submit must return the same result: %d %v vs %v", status, again, submitted)
	}
	status, body := s.call(t, http.MethodPatch, "/v1/encounters/"+id, localToken, map[string]any{"notes": "too late"})
	if status != http.StatusConflict || body["error"].(map[string]any)["code"] != "ENCOUNTER_NOT_EDITABLE" {
		t.Fatalf("editing SUBMITTED: %d %v", status, body)
	}

	status, list := s.call(t, http.MethodGet, "/v1/users/me/encounters", localToken, nil)
	items, _ := list["items"].([]any)
	if status != http.StatusOK || len(items) != 1 || items[0].(map[string]any)["id"] != id {
		t.Fatalf("my encounters: %d %v", status, list)
	}
}

// Requirement 002 §25/§26 MANDATORY: responses never contain precise
// coordinates even though the database holds a GPS point; logs never contain
// coordinates or notes.
func TestPublicResponsesAndLogsNeverContainPreciseLocation(t *testing.T) {
	s := newWildlifeStack(t, nil)
	s.bootstrap(t, localToken)
	status, rawCreate := s.raw(t, http.MethodPost, "/v1/encounters", localToken, s.encounterBody())
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, rawCreate)
	}
	var created map[string]any
	_ = json.Unmarshal([]byte(rawCreate), &created)
	id := created["id"].(string)

	var lat, lon float64
	if err := s.pool.QueryRow(t.Context(), `SELECT public.ST_Y(point::public.geometry), public.ST_X(point::public.geometry)
		FROM encounter_locations WHERE encounter_id=$1`, id).Scan(&lat, &lon); err != nil || lat != secretLat || lon != secretLon {
		t.Fatalf("precondition: the database must hold the GPS point: %v %v %v", err, lat, lon)
	}

	_, rawPatch := s.raw(t, http.MethodPatch, "/v1/encounters/"+id, localToken,
		map[string]any{"location": map[string]any{"latitude": secretLat, "longitude": secretLon, "source": "MANUAL"}})
	_, rawGet := s.raw(t, http.MethodGet, "/v1/encounters/"+id, localToken, nil)
	_, rawList := s.raw(t, http.MethodGet, "/v1/users/me/encounters", localToken, nil)
	_, rawSubmit := s.raw(t, http.MethodPost, "/v1/encounters/"+id+"/submit", localToken, nil)
	_, rawParks := s.raw(t, http.MethodGet, "/v1/parks", "", nil)
	_, rawZones := s.raw(t, http.MethodGet, "/v1/parks/"+s.parkID+"/zones", "", nil)
	_, rawHias := s.raw(t, http.MethodGet, "/v1/hias", "", nil)
	_, rawBadLoc := s.raw(t, http.MethodPost, "/v1/encounters", localToken,
		`{"capturedAt":"2026-09-25T07:30:00Z","location":{"latitude":13.7319999,"longitude":200.5429999,"source":"GPS"}}`)
	for where, text := range map[string]string{
		"create": rawCreate, "patch": rawPatch, "get": rawGet, "list": rawList, "submit": rawSubmit,
		"parks": rawParks, "zones": rawZones, "hias": rawHias,
	} {
		assertNoLocation(t, where, text)
	}
	if !strings.Contains(rawBadLoc, "VALIDATION_ERROR") {
		t.Fatalf("expected a validation error: %s", rawBadLoc)
	}
	assertNoLocation(t, "validation error", rawBadLoc, valueMarkers)

	logs := s.logs.String()
	if !strings.Contains(logs, `"encounter_created"`) || !strings.Contains(logs, `"encounter_submitted"`) || !strings.Contains(logs, id) {
		t.Fatalf("expected structured encounter events in logs: %s", logs)
	}
	assertNoLocation(t, "logs", logs)
	for _, secret := range []string{secretNotes, "private-note", localToken, "@"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("logs contain %q: %s", secret, logs)
		}
	}
}

// Requirement 002 §31/§47: another user cannot read, edit, or submit an
// encounter they do not own. The second user is a real internal user created
// through the normal bootstrap path.
func TestEncounterOwnershipAcrossUsers(t *testing.T) {
	jwt := newStack(t) // offline RS256 verifier with a local JWKS server
	s := newWildlifeStack(t, jwtVerifier(t, jwt))
	alice, bob := jwt.token(t, "alice"), jwt.token(t, "bob")
	s.bootstrap(t, alice)
	s.bootstrap(t, bob)

	status, created := s.call(t, http.MethodPost, "/v1/encounters", alice, s.encounterBody())
	if status != http.StatusCreated {
		t.Fatalf("create: %d %v", status, created)
	}
	id := created["id"].(string)
	for name, req := range map[string]struct {
		method, path string
		body         any
	}{
		"read":   {http.MethodGet, "/v1/encounters/" + id, nil},
		"edit":   {http.MethodPatch, "/v1/encounters/" + id, map[string]any{"notes": "bob was here"}},
		"submit": {http.MethodPost, "/v1/encounters/" + id + "/submit", nil},
	} {
		status, body := s.call(t, req.method, req.path, bob, req.body)
		if status != http.StatusNotFound || body["error"].(map[string]any)["code"] != "ENCOUNTER_NOT_FOUND" {
			t.Fatalf("bob %s: %d %v", name, status, body)
		}
	}
	status, list := s.call(t, http.MethodGet, "/v1/users/me/encounters", bob, nil)
	if items, _ := list["items"].([]any); status != http.StatusOK || len(items) != 0 {
		t.Fatalf("bob's list: %d %v", status, list)
	}
	status, still := s.call(t, http.MethodGet, "/v1/encounters/"+id, alice, nil)
	if status != http.StatusOK || still["status"] != "DRAFT" || still["notes"] != secretNotes {
		t.Fatalf("alice's encounter changed: %d %v", status, still)
	}
}

func jwtVerifier(t *testing.T, s *stack) application.IdentityVerifier {
	t.Helper()
	v, err := identity.NewJWTVerifier(issuer, audience, s.jwksed.URL, s.jwksed.Client())
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// Requirement 002 amendment 5 / §20 / §47: SUSPENDED and DELETED users cannot
// create, edit, or submit; they can still read their own history.
func TestNonActiveUsersCannotWriteEncounters(t *testing.T) {
	for _, status := range []string{domain.UserStatusSuspended, domain.UserStatusDeleted} {
		t.Run(status, func(t *testing.T) {
			s := newWildlifeStack(t, nil)
			userID := s.bootstrap(t, localToken)
			code, created := s.call(t, http.MethodPost, "/v1/encounters", localToken, s.encounterBody())
			if code != http.StatusCreated {
				t.Fatalf("create while ACTIVE: %d %v", code, created)
			}
			id := created["id"].(string)
			if _, err := s.pool.Exec(t.Context(), "UPDATE users SET status=$2 WHERE id=$1", userID, status); err != nil {
				t.Fatal(err)
			}
			for name, req := range map[string]struct {
				method, path string
				body         any
			}{
				"create": {http.MethodPost, "/v1/encounters", s.encounterBody()},
				"edit":   {http.MethodPatch, "/v1/encounters/" + id, map[string]any{"notes": "x"}},
				"submit": {http.MethodPost, "/v1/encounters/" + id + "/submit", nil},
			} {
				code, body := s.call(t, req.method, req.path, localToken, req.body)
				if code != http.StatusForbidden || body["error"].(map[string]any)["code"] != "USER_NOT_ACTIVE" {
					t.Fatalf("%s as %s: %d %v", name, status, code, body)
				}
			}
			var count int
			var encounterStatus string
			if err := s.pool.QueryRow(t.Context(), "SELECT count(*), min(status) FROM encounters").Scan(&count, &encounterStatus); err != nil {
				t.Fatal(err)
			}
			if count != 1 || encounterStatus != "DRAFT" {
				t.Fatalf("non-active user changed data: count=%d status=%s", count, encounterStatus)
			}
			if code, _ := s.call(t, http.MethodGet, "/v1/encounters/"+id, localToken, nil); code != http.StatusOK {
				t.Fatalf("read own encounter as %s: %d", status, code)
			}
		})
	}
}

// Requirement 002 §48: no mass assignment of server-controlled fields.
func TestEncounterRequestsRejectServerControlledFields(t *testing.T) {
	s := newWildlifeStack(t, nil)
	s.bootstrap(t, localToken)
	base := s.encounterBody()
	for _, field := range []string{"observerUserId", "status", "hiaId", "id", "submittedAt", "createdAt", "updatedAt", "userId"} {
		body := map[string]any{}
		for k, v := range base {
			body[k] = v
		}
		body[field] = "00000000-0000-0000-0000-000000000000"
		if code, resp := s.call(t, http.MethodPost, "/v1/encounters", localToken, body); code != http.StatusBadRequest {
			t.Fatalf("create accepted %s: %d %v", field, code, resp)
		}
	}
	code, created := s.call(t, http.MethodPost, "/v1/encounters", localToken, base)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, created)
	}
	id := created["id"].(string)
	for _, patch := range []map[string]any{
		{"status": "SUBMITTED"}, {"observerUserId": "00000000-0000-0000-0000-000000000000"}, {"hiaId": "HIA-000001"},
		{"submittedAt": "2026-01-01T00:00:00Z"}, {"location": map[string]any{"latitude": 1, "longitude": 2, "source": "GPS", "srid": 3857}},
	} {
		if code, resp := s.call(t, http.MethodPatch, "/v1/encounters/"+id, localToken, patch); code != http.StatusBadRequest {
			t.Fatalf("patch accepted %v: %d %v", patch, code, resp)
		}
	}
	if code, _ := s.raw(t, http.MethodPost, "/v1/encounters/"+id+"/submit", localToken, `{"status":"CONFIRMED"}`); code != http.StatusBadRequest {
		t.Fatalf("submit accepted a body: %d", code)
	}
	var status string
	if err := s.pool.QueryRow(t.Context(), "SELECT status FROM encounters WHERE id=$1", id).Scan(&status); err != nil || status != "DRAFT" {
		t.Fatalf("status changed: %v %s", err, status)
	}
}

func TestEncounterValidationErrors(t *testing.T) {
	s := newWildlifeStack(t, nil)
	s.bootstrap(t, localToken)
	benjakitti := "5f7c1e2a-0000-4000-8000-000000000001"
	cases := map[string]struct {
		body any
		want int
		code string
	}{
		"missing capturedAt": {map[string]any{"behavior": "BASKING"}, 400, "VALIDATION_ERROR"},
		"invalid behavior":   {map[string]any{"capturedAt": "2026-09-25T07:30:00Z", "behavior": "DANCING"}, 400, "VALIDATION_ERROR"},
		"latitude 91":        {map[string]any{"capturedAt": "2026-09-25T07:30:00Z", "location": map[string]any{"latitude": 91, "longitude": 0, "source": "GPS"}}, 400, "VALIDATION_ERROR"},
		"longitude -181":     {map[string]any{"capturedAt": "2026-09-25T07:30:00Z", "location": map[string]any{"latitude": 0, "longitude": -181, "source": "GPS"}}, 400, "VALIDATION_ERROR"},
		"negative accuracy":  {map[string]any{"capturedAt": "2026-09-25T07:30:00Z", "location": map[string]any{"latitude": 0, "longitude": 0, "accuracyMeters": -1, "source": "GPS"}}, 400, "VALIDATION_ERROR"},
		"unknown source":     {map[string]any{"capturedAt": "2026-09-25T07:30:00Z", "location": map[string]any{"latitude": 0, "longitude": 0, "source": "SATELLITE"}}, 400, "VALIDATION_ERROR"},
		"location w/o lat":   {map[string]any{"capturedAt": "2026-09-25T07:30:00Z", "location": map[string]any{"longitude": 0, "source": "GPS"}}, 400, "VALIDATION_ERROR"},
		"zone without park":  {map[string]any{"capturedAt": "2026-09-25T07:30:00Z", "zoneId": s.zones["Lake Zone"]}, 400, "VALIDATION_ERROR"},
		"unknown park":       {map[string]any{"capturedAt": "2026-09-25T07:30:00Z", "parkId": benjakitti, "zoneId": s.zones["Lake Zone"]}, 400, "VALIDATION_ERROR"},
		"not json":           {"{", 400, "INVALID_REQUEST"},
		"two objects":        {`{"capturedAt":"2026-09-25T07:30:00Z"}{}`, 400, "INVALID_REQUEST"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			code, body := s.call(t, http.MethodPost, "/v1/encounters", localToken, tc.body)
			errBody, _ := body["error"].(map[string]any)
			if code != tc.want || errBody == nil || errBody["code"] != tc.code {
				t.Fatalf("%d %v", code, body)
			}
		})
	}
	var count int
	if err := s.pool.QueryRow(t.Context(), "SELECT count(*) FROM encounters").Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid requests persisted rows: %v %d", err, count)
	}
	if code, _ := s.call(t, http.MethodGet, "/v1/encounters/not-a-uuid", localToken, nil); code != http.StatusNotFound {
		t.Fatalf("malformed id: %d", code)
	}
	if code, _ := s.call(t, http.MethodPost, "/v1/encounters", "", s.encounterBody()); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated create: %d", code)
	}
	fresh := newWildlifeStack(t, nil)
	if code, body := fresh.call(t, http.MethodPost, "/v1/encounters", localToken, fresh.encounterBody()); code != http.StatusForbidden || body["error"].(map[string]any)["code"] != "USER_NOT_BOOTSTRAPPED" {
		t.Fatalf("create before bootstrap: %d %v", code, body)
	}
}

// Requirement 002 §32/§33: public park and hia reads, ACTIVE parks only, no
// write endpoints for hias.
func TestPublicParkAndHiaReads(t *testing.T) {
	s := newWildlifeStack(t, nil)
	code, parks := s.call(t, http.MethodGet, "/v1/parks", "", nil)
	items, _ := parks["items"].([]any)
	if code != http.StatusOK || len(items) != 1 || items[0].(map[string]any)["slug"] != "lumpini-park" || items[0].(map[string]any)["timezone"] != "Asia/Bangkok" {
		t.Fatalf("parks: %d %v", code, parks)
	}
	code, zones := s.call(t, http.MethodGet, "/v1/parks/"+s.parkID+"/zones", "", nil)
	if zi, _ := zones["items"].([]any); code != http.StatusOK || len(zi) != 3 {
		t.Fatalf("zones: %d %v", code, zones)
	}
	if _, err := s.pool.Exec(t.Context(), "UPDATE parks SET status='INACTIVE' WHERE id=$1", s.parkID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/parks/" + s.parkID, "/v1/parks/" + s.parkID + "/zones"} {
		if code, body := s.call(t, http.MethodGet, path, "", nil); code != http.StatusNotFound || body["error"].(map[string]any)["code"] != "PARK_NOT_FOUND" {
			t.Fatalf("inactive park %s: %d %v", path, code, body)
		}
	}
	if _, err := s.pool.Exec(t.Context(), "UPDATE parks SET status='ACTIVE' WHERE id=$1", s.parkID); err != nil {
		t.Fatal(err)
	}

	// Synthetic hias exist only in tests (Requirement 002 §35).
	hias := persistence.NewHiaRepository(s.pool)
	h1, _ := hia.NewProvisional(ptr("Test Lizard"), &s.parkID, time.Now())
	first, err := hias.Create(t.Context(), h1)
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := hia.NewProvisional(nil, nil, time.Now())
	if _, err = hias.Create(t.Context(), h2); err != nil {
		t.Fatal(err)
	}
	code, all := s.call(t, http.MethodGet, "/v1/hias", "", nil)
	if ai, _ := all["items"].([]any); code != http.StatusOK || len(ai) != 2 {
		t.Fatalf("hias: %d %v", code, all)
	}
	code, filtered := s.call(t, http.MethodGet, "/v1/hias?parkId="+s.parkID, "", nil)
	fi, _ := filtered["items"].([]any)
	if code != http.StatusOK || len(fi) != 1 || fi[0].(map[string]any)["publicCode"] != first.PublicCode {
		t.Fatalf("filtered hias: %d %v", code, filtered)
	}
	code, one := s.call(t, http.MethodGet, "/v1/hias/"+first.PublicCode, "", nil)
	if code != http.StatusOK || one["publicCode"] != "HIA-000001" || one["nickname"] != "Test Lizard" {
		t.Fatalf("hia: %d %v", code, one)
	}
	if _, leaked := one["id"]; leaked {
		t.Fatalf("internal hia id exposed: %v", one)
	}
	for path, want := range map[string]int{"/v1/hias/HIA-999999": 404, "/v1/hias/lizard": 404, "/v1/hias?parkId=nope": 400, "/v1/hias?sort=name": 400} {
		if code, _ := s.call(t, http.MethodGet, path, "", nil); code != want {
			t.Fatalf("%s: %d want %d", path, code, want)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete} {
		if code, _ := s.call(t, method, "/v1/hias/"+first.PublicCode, localToken, map[string]any{"nickname": "x"}); code != http.StatusMethodNotAllowed {
			t.Fatalf("%s /v1/hias/{code} must not exist: %d", method, code)
		}
	}
	if code, _ := s.call(t, http.MethodPost, "/v1/hias", localToken, map[string]any{}); code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /v1/hias must not exist: %d", code)
	}
}

func ptr[T any](v T) *T { return &v }
