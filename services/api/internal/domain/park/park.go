// Package park models the places where encounters happen: a Park and the
// Zones inside it. It has no persistence or transport dependencies.
package park

import (
	"errors"
	"regexp"
	"strings"
	"time"
	// Embed the IANA database so timezone validation does not depend on the
	// host (Windows and minimal containers ship without zoneinfo).
	_ "time/tzdata"

	"github.com/google/uuid"
)

// Park and Zone statuses. Only ACTIVE records appear in public APIs.
const (
	StatusActive   = "ACTIVE"
	StatusInactive = "INACTIVE"
	StatusArchived = "ARCHIVED"
)

var (
	ErrInvalidSlug        = errors.New("slug must be 1-64 characters of letters, numbers, and single hyphens")
	ErrInvalidName        = errors.New("name must be 1-120 characters")
	ErrInvalidCity        = errors.New("city must be 1-120 characters")
	ErrInvalidCountryCode = errors.New("countryCode must be a 2-letter ISO 3166-1 alpha-2 style code")
	ErrInvalidTimezone    = errors.New("timezone must be a valid IANA timezone name")
	ErrInvalidStatus      = errors.New("status must be ACTIVE, INACTIVE, or ARCHIVED")
	ErrInvalidParkID      = errors.New("zone must belong to a park")

	// ErrSlugTaken is returned when a park slug collides case-insensitively.
	ErrSlugTaken = errors.New("park slug is already taken")
	// ErrZoneSlugTaken is returned when a zone slug collides within its park.
	ErrZoneSlugTaken = errors.New("zone slug is already taken in this park")
	// ErrNotFound is returned when a park or zone does not exist (or is not public).
	ErrNotFound = errors.New("park or zone not found")

	slugPattern    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)
)

type Park struct {
	ID          string
	Slug        string
	Name        string
	City        string
	CountryCode string
	Timezone    string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Zone struct {
	ID        string
	ParkID    string
	Slug      string
	Name      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ParkInput holds client- or seed-supplied park attributes.
type ParkInput struct {
	Slug, Name, City, CountryCode, Timezone, Status string
}

// ZoneInput holds client- or seed-supplied zone attributes.
type ZoneInput struct {
	ParkID, Slug, Name, Status string
}

// NewPark validates input and returns a park with a server-generated ID.
// Slugs are normalised to lower case; uniqueness is case-insensitive.
func NewPark(in ParkInput, now time.Time) (Park, error) {
	p := Park{
		ID:          uuid.NewString(),
		Slug:        NormalizeSlug(in.Slug),
		Name:        strings.TrimSpace(in.Name),
		City:        strings.TrimSpace(in.City),
		CountryCode: strings.TrimSpace(in.CountryCode),
		Timezone:    strings.TrimSpace(in.Timezone),
		Status:      in.Status,
		CreatedAt:   now.UTC().Truncate(time.Microsecond),
	}
	p.UpdatedAt = p.CreatedAt
	return p, p.Validate()
}

// NewZone validates input and returns a zone with a server-generated ID.
func NewZone(in ZoneInput, now time.Time) (Zone, error) {
	z := Zone{
		ID:        uuid.NewString(),
		ParkID:    strings.TrimSpace(in.ParkID),
		Slug:      NormalizeSlug(in.Slug),
		Name:      strings.TrimSpace(in.Name),
		Status:    in.Status,
		CreatedAt: now.UTC().Truncate(time.Microsecond),
	}
	z.UpdatedAt = z.CreatedAt
	return z, z.Validate()
}

func NormalizeSlug(slug string) string { return strings.ToLower(strings.TrimSpace(slug)) }

func (p Park) Validate() error {
	if err := validateSlug(p.Slug); err != nil {
		return err
	}
	if !lengthBetween(p.Name, 1, 120) {
		return ErrInvalidName
	}
	if !lengthBetween(p.City, 1, 120) {
		return ErrInvalidCity
	}
	if !countryPattern.MatchString(p.CountryCode) {
		return ErrInvalidCountryCode
	}
	if err := ValidateTimezone(p.Timezone); err != nil {
		return err
	}
	return validateStatus(p.Status)
}

func (z Zone) Validate() error {
	if _, err := uuid.Parse(z.ParkID); err != nil {
		return ErrInvalidParkID
	}
	if err := validateSlug(z.Slug); err != nil {
		return err
	}
	if !lengthBetween(z.Name, 1, 120) {
		return ErrInvalidName
	}
	return validateStatus(z.Status)
}

// ValidateTimezone accepts IANA names only. time.LoadLocation also accepts ""
// (UTC) and "Local" (host dependent); both are rejected here.
func ValidateTimezone(name string) error {
	if name == "" || name == "Local" || strings.TrimSpace(name) != name {
		return ErrInvalidTimezone
	}
	if _, err := time.LoadLocation(name); err != nil {
		return ErrInvalidTimezone
	}
	return nil
}

func validateSlug(slug string) error {
	if len(slug) > 64 || !slugPattern.MatchString(slug) {
		return ErrInvalidSlug
	}
	return nil
}

func validateStatus(status string) error {
	switch status {
	case StatusActive, StatusInactive, StatusArchived:
		return nil
	}
	return ErrInvalidStatus
}

func lengthBetween(value string, lo, hi int) bool {
	n := len([]rune(value))
	return n >= lo && n <= hi
}
