package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"
	"time"

	"github.com/nawariso/toem-here/services/api/internal/application"
	"github.com/nawariso/toem-here/services/api/internal/domain"
	"github.com/nawariso/toem-here/services/api/internal/domain/encounter"
	"github.com/nawariso/toem-here/services/api/internal/domain/hia"
	"github.com/nawariso/toem-here/services/api/internal/domain/location"
	"github.com/nawariso/toem-here/services/api/internal/domain/park"
)

// WithWildlife enables the Requirement 002 park, hia, and encounter routes.
func WithWildlife(parks application.Parks, hias application.Hias, encounters application.Encounters) Option {
	return func(s *Server) {
		s.parks, s.hias, s.encounters = parks, hias, encounters
	}
}

func (s *Server) wildlifeRoutes() {
	if s.parks == nil || s.hias == nil || s.encounters == nil {
		return
	}
	// Public read APIs (no authentication).
	s.mux.HandleFunc("GET /v1/parks", s.listParks)
	s.mux.HandleFunc("GET /v1/parks/{id}", s.getPark)
	s.mux.HandleFunc("GET /v1/parks/{id}/zones", s.listZones)
	s.mux.HandleFunc("GET /v1/hias", s.listHias)
	s.mux.HandleFunc("GET /v1/hias/{publicCode}", s.getHia)
	// Encounter APIs (authenticated; owner-only).
	s.mux.Handle("POST /v1/encounters", s.auth(nethttp.HandlerFunc(s.createEncounter)))
	s.mux.Handle("GET /v1/encounters/{id}", s.auth(nethttp.HandlerFunc(s.getEncounter)))
	s.mux.Handle("PATCH /v1/encounters/{id}", s.auth(nethttp.HandlerFunc(s.updateEncounter)))
	s.mux.Handle("POST /v1/encounters/{id}/submit", s.auth(nethttp.HandlerFunc(s.submitEncounter)))
	s.mux.Handle("GET /v1/users/me/encounters", s.auth(nethttp.HandlerFunc(s.listMyEncounters)))
}

// ------------------------------------------------------------------ DTOs ---
// Response DTOs are explicit allowlists. Encounter DTOs have no coordinate,
// point, or accuracy field, so precise location cannot be serialised.

type parkResponse struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	City        string `json:"city"`
	CountryCode string `json:"countryCode"`
	Timezone    string `json:"timezone"`
}

type zoneResponse struct {
	ID     string `json:"id"`
	ParkID string `json:"parkId"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
}

type hiaResponse struct {
	PublicCode           string     `json:"publicCode"`
	Nickname             *string    `json:"nickname"`
	Status               string     `json:"status"`
	HomeParkID           *string    `json:"homeParkId"`
	FirstSeenAt          *time.Time `json:"firstSeenAt"`
	ConfirmedAt          *time.Time `json:"confirmedAt"`
	MergedIntoPublicCode *string    `json:"mergedIntoPublicCode"`
}

type placeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type encounterResponse struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	CapturedAt  time.Time  `json:"capturedAt"`
	SubmittedAt *time.Time `json:"submittedAt"`
	Park        *placeRef  `json:"park"`
	Zone        *placeRef  `json:"zone"`
	Behavior    *string    `json:"behavior"`
	Notes       *string    `json:"notes"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type listResponse[T any] struct {
	Items []T `json:"items"`
}

func toPark(p park.Park) parkResponse {
	return parkResponse{ID: p.ID, Slug: p.Slug, Name: p.Name, City: p.City, CountryCode: p.CountryCode, Timezone: p.Timezone}
}

func toZone(z park.Zone) zoneResponse {
	return zoneResponse{ID: z.ID, ParkID: z.ParkID, Slug: z.Slug, Name: z.Name}
}

func toHia(v application.HiaView) hiaResponse {
	h := v.Hia
	return hiaResponse{PublicCode: h.PublicCode, Nickname: h.Nickname, Status: h.Status, HomeParkID: h.HomeParkID,
		FirstSeenAt: utcPtr(h.FirstSeenAt), ConfirmedAt: utcPtr(h.ConfirmedAt), MergedIntoPublicCode: v.MergedIntoPublicCode}
}

func toEncounter(v application.EncounterView) encounterResponse {
	e := v.Encounter
	out := encounterResponse{ID: e.ID, Status: e.Status, CapturedAt: e.CapturedAt.UTC(), SubmittedAt: utcPtr(e.SubmittedAt),
		Behavior: e.Behavior, Notes: e.Notes, CreatedAt: e.CreatedAt.UTC(), UpdatedAt: e.UpdatedAt.UTC()}
	if v.Park != nil {
		out.Park = &placeRef{ID: v.Park.ID, Name: v.Park.Name}
	}
	if v.Zone != nil {
		out.Zone = &placeRef{ID: v.Zone.ID, Name: v.Zone.Name}
	}
	return out
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// ------------------------------------------------------ public handlers ---

func (s *Server) listParks(w nethttp.ResponseWriter, r *nethttp.Request) {
	parks, err := s.parks.ListParks(r.Context())
	if err != nil {
		s.wildlifeError(w, r, err)
		return
	}
	out := listResponse[parkResponse]{Items: make([]parkResponse, 0, len(parks))}
	for _, p := range parks {
		out.Items = append(out.Items, toPark(p))
	}
	writeJSON(w, nethttp.StatusOK, out)
}

func (s *Server) getPark(w nethttp.ResponseWriter, r *nethttp.Request) {
	p, err := s.parks.GetPark(r.Context(), r.PathValue("id"))
	if err != nil {
		s.wildlifeError(w, r, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, toPark(p))
}

func (s *Server) listZones(w nethttp.ResponseWriter, r *nethttp.Request) {
	zones, err := s.parks.ListZones(r.Context(), r.PathValue("id"))
	if err != nil {
		s.wildlifeError(w, r, err)
		return
	}
	out := listResponse[zoneResponse]{Items: make([]zoneResponse, 0, len(zones))}
	for _, z := range zones {
		out.Items = append(out.Items, toZone(z))
	}
	writeJSON(w, nethttp.StatusOK, out)
}

func (s *Server) listHias(w nethttp.ResponseWriter, r *nethttp.Request) {
	query := r.URL.Query()
	for key := range query {
		if key != "parkId" {
			writeError(w, r, nethttp.StatusBadRequest, "INVALID_REQUEST", "Unsupported query parameter")
			return
		}
	}
	var parkID *string
	if values, ok := query["parkId"]; ok {
		if len(values) != 1 {
			writeError(w, r, nethttp.StatusBadRequest, "INVALID_REQUEST", "parkId may be given once")
			return
		}
		parkID = &values[0]
	}
	hias, err := s.hias.List(r.Context(), parkID)
	if err != nil {
		s.wildlifeError(w, r, err)
		return
	}
	out := listResponse[hiaResponse]{Items: make([]hiaResponse, 0, len(hias))}
	for _, h := range hias {
		out.Items = append(out.Items, toHia(h))
	}
	writeJSON(w, nethttp.StatusOK, out)
}

func (s *Server) getHia(w nethttp.ResponseWriter, r *nethttp.Request) {
	h, err := s.hias.Get(r.Context(), r.PathValue("publicCode"))
	if err != nil {
		s.wildlifeError(w, r, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, toHia(h))
}

// --------------------------------------------------- encounter handlers ---

type locationInput struct {
	Latitude       *float64 `json:"latitude"`
	Longitude      *float64 `json:"longitude"`
	AccuracyMeters *float64 `json:"accuracyMeters"`
	Source         *string  `json:"source"`
}

var errLocationIncomplete = errors.New("location requires latitude, longitude, and source")

func (in locationInput) toDomain() (location.Private, error) {
	if in.Latitude == nil || in.Longitude == nil || in.Source == nil {
		return location.Private{}, errLocationIncomplete
	}
	loc := location.Private{Latitude: *in.Latitude, Longitude: *in.Longitude, AccuracyMeters: in.AccuracyMeters, Source: *in.Source}
	return loc, loc.Validate()
}

// createEncounterRequest is the complete set of client-settable fields.
// observerUserId, status, hiaId, id, and timestamps are rejected as unknown.
type createEncounterRequest struct {
	CapturedAt *time.Time     `json:"capturedAt"`
	ParkID     *string        `json:"parkId"`
	ZoneID     *string        `json:"zoneId"`
	Behavior   *string        `json:"behavior"`
	Notes      *string        `json:"notes"`
	Location   *locationInput `json:"location"`
}

func (s *Server) createEncounter(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req createEncounterRequest
	if !decodeStrict(w, r, &req) {
		return
	}
	if req.CapturedAt == nil {
		s.wildlifeError(w, r, encounter.ErrCapturedAtRequired)
		return
	}
	in := application.CreateEncounterInput{Details: encounter.Details{
		CapturedAt: *req.CapturedAt, ParkID: req.ParkID, ZoneID: req.ZoneID, Behavior: req.Behavior, Notes: req.Notes,
	}}
	if req.Location != nil {
		loc, err := req.Location.toDomain()
		if err != nil {
			s.wildlifeError(w, r, err)
			return
		}
		in.Location = &loc
	}
	view, err := s.encounters.Create(r.Context(), getIdentity(r), in)
	if err != nil {
		s.wildlifeError(w, r, err)
		return
	}
	s.logEncounter(r, "encounter_created", view)
	writeJSON(w, nethttp.StatusCreated, toEncounter(view))
}

func (s *Server) getEncounter(w nethttp.ResponseWriter, r *nethttp.Request) {
	view, err := s.encounters.Get(r.Context(), getIdentity(r), r.PathValue("id"))
	if err != nil {
		s.wildlifeError(w, r, err)
		return
	}
	writeJSON(w, nethttp.StatusOK, toEncounter(view))
}

func (s *Server) listMyEncounters(w nethttp.ResponseWriter, r *nethttp.Request) {
	views, err := s.encounters.ListMine(r.Context(), getIdentity(r))
	if err != nil {
		s.wildlifeError(w, r, err)
		return
	}
	out := listResponse[encounterResponse]{Items: make([]encounterResponse, 0, len(views))}
	for _, v := range views {
		out.Items = append(out.Items, toEncounter(v))
	}
	writeJSON(w, nethttp.StatusOK, out)
}

// patchable lists every field PATCH /v1/encounters/{id} accepts. Anything
// else, including status and observerUserId, is rejected.
var patchable = map[string]bool{"capturedAt": true, "parkId": true, "zoneId": true, "behavior": true, "notes": true, "location": true}

func (s *Server) updateEncounter(w nethttp.ResponseWriter, r *nethttp.Request) {
	var raw map[string]json.RawMessage
	if !decodeStrict(w, r, &raw) {
		return
	}
	if raw == nil {
		writeError(w, r, nethttp.StatusBadRequest, "INVALID_REQUEST", "Request body must be a JSON object")
		return
	}
	for key := range raw {
		if !patchable[key] {
			writeError(w, r, nethttp.StatusBadRequest, "INVALID_REQUEST", "Request body contains a field that cannot be changed")
			return
		}
	}
	patch, err := parseEncounterPatch(raw)
	if err != nil {
		s.wildlifeError(w, r, err)
		return
	}
	view, err := s.encounters.Update(r.Context(), getIdentity(r), r.PathValue("id"), patch)
	if err != nil {
		s.wildlifeError(w, r, err)
		return
	}
	s.logEncounter(r, "encounter_updated", view)
	writeJSON(w, nethttp.StatusOK, toEncounter(view))
}

func (s *Server) submitEncounter(w nethttp.ResponseWriter, r *nethttp.Request) {
	if !requireEmptyBody(w, r) {
		return
	}
	view, changed, err := s.encounters.Submit(r.Context(), getIdentity(r), r.PathValue("id"))
	if err != nil {
		s.wildlifeError(w, r, err)
		return
	}
	if changed {
		s.logEncounter(r, "encounter_submitted", view)
	}
	writeJSON(w, nethttp.StatusOK, toEncounter(view))
}

// logEncounter writes only the fields Requirement 002 §49 allows: never
// coordinates, notes, tokens, or email.
func (s *Server) logEncounter(r *nethttp.Request, event string, v application.EncounterView) {
	parkID := ""
	if v.Encounter.ParkID != nil {
		parkID = *v.Encounter.ParkID
	}
	s.logger.Info(event, "request_id", requestID(r), "encounter_id", v.Encounter.ID,
		"user_id", v.Encounter.ObserverUserID, "park_id", parkID, "status", v.Encounter.Status)
}

var errNullNotAllowed = errors.New("capturedAt and location fields cannot be null")

func parseEncounterPatch(raw map[string]json.RawMessage) (application.EncounterPatch, error) {
	var p application.EncounterPatch
	if v, ok := raw["capturedAt"]; ok {
		var t *time.Time
		if err := json.Unmarshal(v, &t); err != nil {
			return p, errInvalidField
		}
		if t == nil {
			return p, errNullNotAllowed
		}
		p.CapturedAt = t
	}
	for key, target := range map[string]*application.Optional[string]{"parkId": &p.ParkID, "zoneId": &p.ZoneID, "behavior": &p.Behavior, "notes": &p.Notes} {
		if v, ok := raw[key]; ok {
			var value *string
			if err := json.Unmarshal(v, &value); err != nil {
				return p, errInvalidField
			}
			*target = application.Optional[string]{Set: true, Value: value}
		}
	}
	if v, ok := raw["location"]; ok {
		p.Location.Set = true
		if !bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			var in locationInput
			dec := json.NewDecoder(bytes.NewReader(v))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&in); err != nil {
				return p, errInvalidField
			}
			loc, err := in.toDomain()
			if err != nil {
				return p, err
			}
			p.Location.Value = &loc
		}
	}
	return p, nil
}

var errInvalidField = errors.New("request body has a field with the wrong type")

// decodeStrict decodes exactly one JSON value, rejecting unknown fields.
func decodeStrict(w nethttp.ResponseWriter, r *nethttp.Request, into any) bool {
	r.Body = nethttp.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		writeError(w, r, nethttp.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid")
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeError(w, r, nethttp.StatusBadRequest, "INVALID_REQUEST", "Request body must contain one JSON object")
		return false
	}
	return true
}

func requireEmptyBody(w nethttp.ResponseWriter, r *nethttp.Request) bool {
	r.Body = nethttp.MaxBytesReader(w, r.Body, 1<<10)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil || len(bytes.TrimSpace(body)) > 0 {
		writeError(w, r, nethttp.StatusBadRequest, "INVALID_REQUEST", "This endpoint takes no request body")
		return false
	}
	return true
}

// wildlifeError maps domain/application errors to the stable error contract.
// Unknown errors become a generic 500 so SQL/driver details never leak.
func (s *Server) wildlifeError(w nethttp.ResponseWriter, r *nethttp.Request, err error) {
	type mapping struct {
		status  int
		code    string
		message string
	}
	validation := func(e error) mapping { return mapping{nethttp.StatusBadRequest, "VALIDATION_ERROR", e.Error()} }
	var m mapping
	switch {
	case errors.Is(err, domain.ErrUserNotActive):
		m = mapping{nethttp.StatusForbidden, "USER_NOT_ACTIVE", "Only active users can create or change encounters"}
	case errors.Is(err, application.ErrUserNotBootstrapped):
		m = mapping{nethttp.StatusForbidden, "USER_NOT_BOOTSTRAPPED", "Call POST /v1/auth/bootstrap first"}
	case errors.Is(err, domain.ErrInvalidIdentity):
		m = mapping{nethttp.StatusUnauthorized, "UNAUTHENTICATED", "Authentication token is invalid or expired"}
	case errors.Is(err, encounter.ErrNotFound):
		m = mapping{nethttp.StatusNotFound, "ENCOUNTER_NOT_FOUND", "Encounter was not found"}
	case errors.Is(err, encounter.ErrNotEditable):
		m = mapping{nethttp.StatusConflict, "ENCOUNTER_NOT_EDITABLE", err.Error()}
	case errors.Is(err, encounter.ErrInvalidTransition), errors.Is(err, encounter.ErrUnknownStatus):
		m = mapping{nethttp.StatusConflict, "INVALID_STATUS_TRANSITION", encounter.ErrInvalidTransition.Error()}
	case errors.Is(err, park.ErrNotFound):
		m = mapping{nethttp.StatusNotFound, "PARK_NOT_FOUND", "Park was not found"}
	case errors.Is(err, hia.ErrNotFound):
		m = mapping{nethttp.StatusNotFound, "HIA_NOT_FOUND", "Hia was not found"}
	case errors.Is(err, application.ErrInvalidParkFilter),
		errors.Is(err, encounter.ErrCapturedAtRequired), errors.Is(err, encounter.ErrInvalidBehavior),
		errors.Is(err, encounter.ErrNotesTooLong), errors.Is(err, encounter.ErrZoneRequiresPark),
		errors.Is(err, encounter.ErrZoneNotInPark), errors.Is(err, encounter.ErrParkNotFound),
		errors.Is(err, encounter.ErrZoneNotFound), errors.Is(err, encounter.ErrInvalidReference),
		errors.Is(err, location.ErrInvalidLatitude), errors.Is(err, location.ErrInvalidLongitude),
		errors.Is(err, location.ErrInvalidAccuracy), errors.Is(err, location.ErrInvalidSource),
		errors.Is(err, errLocationIncomplete), errors.Is(err, errNullNotAllowed), errors.Is(err, errInvalidField):
		m = validation(err)
	default:
		s.logger.Error("request_failed", "request_id", requestID(r), "error_type", "internal")
		m = mapping{nethttp.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred"}
	}
	writeError(w, r, m.status, m.code, m.message)
}
