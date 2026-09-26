package park_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nawariso/toem-hia/services/api/internal/domain/park"
)

var now = time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)

func validPark() park.ParkInput {
	return park.ParkInput{Slug: "lumpini-park", Name: "Lumpini Park", City: "Bangkok", CountryCode: "TH", Timezone: "Asia/Bangkok", Status: park.StatusActive}
}

func TestNewParkAcceptsValidInputAndNormalisesSlug(t *testing.T) {
	in := validPark()
	in.Slug = "  Lumpini-Park "
	p, err := park.NewPark(in, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Slug != "lumpini-park" || p.ID == "" || !p.CreatedAt.Equal(now) || !p.UpdatedAt.Equal(now) {
		t.Fatalf("unexpected park: %+v", p)
	}
}

func TestNewParkIsNotTiedToBangkok(t *testing.T) {
	in := park.ParkInput{Slug: "hyde-park", Name: "Hyde Park", City: "London", CountryCode: "GB", Timezone: "Europe/London", Status: park.StatusActive}
	if _, err := park.NewPark(in, now); err != nil {
		t.Fatalf("a non-Thai park must be valid: %v", err)
	}
}

func TestNewParkRejectsInvalidFields(t *testing.T) {
	cases := map[string]struct {
		mutate func(*park.ParkInput)
		want   error
	}{
		"empty slug":         {func(p *park.ParkInput) { p.Slug = "" }, park.ErrInvalidSlug},
		"slug with space":    {func(p *park.ParkInput) { p.Slug = "lumpini park" }, park.ErrInvalidSlug},
		"slug double hyphen": {func(p *park.ParkInput) { p.Slug = "a--b" }, park.ErrInvalidSlug},
		"slug too long":      {func(p *park.ParkInput) { p.Slug = strings.Repeat("a", 65) }, park.ErrInvalidSlug},
		"empty name":         {func(p *park.ParkInput) { p.Name = "  " }, park.ErrInvalidName},
		"empty city":         {func(p *park.ParkInput) { p.City = "" }, park.ErrInvalidCity},
		"lowercase country":  {func(p *park.ParkInput) { p.CountryCode = "th" }, park.ErrInvalidCountryCode},
		"3-letter country":   {func(p *park.ParkInput) { p.CountryCode = "THA" }, park.ErrInvalidCountryCode},
		"invalid timezone":   {func(p *park.ParkInput) { p.Timezone = "Asia/Nowhere" }, park.ErrInvalidTimezone},
		"empty timezone":     {func(p *park.ParkInput) { p.Timezone = "" }, park.ErrInvalidTimezone},
		"Local timezone":     {func(p *park.ParkInput) { p.Timezone = "Local" }, park.ErrInvalidTimezone},
		"unknown status":     {func(p *park.ParkInput) { p.Status = "OPEN" }, park.ErrInvalidStatus},
		"lowercase status":   {func(p *park.ParkInput) { p.Status = "active" }, park.ErrInvalidStatus},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			in := validPark()
			tc.mutate(&in)
			if _, err := park.NewPark(in, now); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNewZoneValidation(t *testing.T) {
	p, err := park.NewPark(validPark(), now)
	if err != nil {
		t.Fatal(err)
	}
	z, err := park.NewZone(park.ZoneInput{ParkID: p.ID, Slug: "Lake-Zone", Name: "Lake Zone", Status: park.StatusActive}, now)
	if err != nil || z.Slug != "lake-zone" || z.ParkID != p.ID {
		t.Fatalf("valid zone rejected: %+v %v", z, err)
	}
	if _, err = park.NewZone(park.ZoneInput{ParkID: "not-a-uuid", Slug: "x", Name: "X", Status: park.StatusActive}, now); !errors.Is(err, park.ErrInvalidParkID) {
		t.Fatalf("zone without a park must be rejected, got %v", err)
	}
	if _, err = park.NewZone(park.ZoneInput{ParkID: p.ID, Slug: "x", Name: "X", Status: "DELETED"}, now); !errors.Is(err, park.ErrInvalidStatus) {
		t.Fatalf("invalid zone status must be rejected, got %v", err)
	}
}
