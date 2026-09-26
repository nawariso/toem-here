package location_test

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/nawariso/toem-here/services/api/internal/domain/location"
)

func acc(v float64) *float64 { return &v }

func TestValidateBoundaries(t *testing.T) {
	valid := []location.Private{
		{Latitude: 13.7307, Longitude: 100.5418, AccuracyMeters: acc(5), Source: location.SourceGPS},
		{Latitude: -90, Longitude: -180, Source: location.SourceManual},
		{Latitude: 90, Longitude: 180, AccuracyMeters: acc(0), Source: location.SourceImport},
		{Latitude: 0, Longitude: 0, Source: location.SourceUnknown},
	}
	for _, p := range valid {
		if err := p.Validate(); err != nil {
			t.Fatalf("valid location rejected: %v", err)
		}
	}
	invalid := []struct {
		p    location.Private
		want error
	}{
		{location.Private{Latitude: 90.0001, Longitude: 0, Source: "GPS"}, location.ErrInvalidLatitude},
		{location.Private{Latitude: -90.0001, Longitude: 0, Source: "GPS"}, location.ErrInvalidLatitude},
		{location.Private{Latitude: math.NaN(), Longitude: 0, Source: "GPS"}, location.ErrInvalidLatitude},
		{location.Private{Latitude: 0, Longitude: 180.0001, Source: "GPS"}, location.ErrInvalidLongitude},
		{location.Private{Latitude: 0, Longitude: -180.0001, Source: "GPS"}, location.ErrInvalidLongitude},
		{location.Private{Latitude: 0, Longitude: math.Inf(1), Source: "GPS"}, location.ErrInvalidLongitude},
		{location.Private{Latitude: 0, Longitude: 0, AccuracyMeters: acc(-0.1), Source: "GPS"}, location.ErrInvalidAccuracy},
		{location.Private{Latitude: 0, Longitude: 0, AccuracyMeters: acc(math.Inf(1)), Source: "GPS"}, location.ErrInvalidAccuracy},
		{location.Private{Latitude: 0, Longitude: 0, Source: "gps"}, location.ErrInvalidSource},
		{location.Private{Latitude: 0, Longitude: 0, Source: ""}, location.ErrInvalidSource},
	}
	for _, tc := range invalid {
		if err := tc.p.Validate(); !errors.Is(err, tc.want) {
			t.Fatalf("got %v want %v", err, tc.want)
		}
	}
}

// Formatting a location must never print coordinates (Requirement 002 §48).
func TestFormattingRedactsCoordinates(t *testing.T) {
	p := location.Private{Latitude: 13.7307123, Longitude: 100.5418456, AccuracyMeters: acc(4.5), Source: "GPS"}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		out := fmt.Sprintf(verb, p)
		if strings.Contains(out, "13.73") || strings.Contains(out, "100.54") {
			t.Fatalf("%s leaked coordinates: %s", verb, out)
		}
	}
}
