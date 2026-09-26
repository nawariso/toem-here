// Package seed loads the development reference data for Requirement 002: one
// park and its zones. It creates no Hias; synthetic Hias exist only in tests.
package seed

import (
	"context"
	"errors"
	"time"

	"github.com/nawariso/toem-here/services/api/internal/domain/park"
)

// ErrForbiddenEnvironment is returned for any APP_ENV outside the allowlist.
var ErrForbiddenEnvironment = errors.New("development seed runs only when APP_ENV is development or test")

// Permitted is an allowlist: production and every unknown value are refused.
func Permitted(appEnv string) bool { return appEnv == "development" || appEnv == "test" }

// Store is the subset of the park repository the seed needs. Ensure* must
// insert only when the slug is absent and never modify an existing row.
type Store interface {
	EnsurePark(context.Context, park.Park) (park.Park, bool, error)
	EnsureZone(context.Context, park.Zone) (park.Zone, bool, error)
}

// Result reports what a run inserted; a repeat run inserts nothing.
type Result struct {
	ParkID        string
	ParkCreated   bool
	ZonesCreated  int
	ZonesExisting int
}

var lumpini = park.ParkInput{Slug: "lumpini-park", Name: "Lumpini Park", City: "Bangkok", CountryCode: "TH", Timezone: "Asia/Bangkok", Status: park.StatusActive}

var lumpiniZones = []struct{ slug, name string }{
	{"lake-zone", "Lake Zone"},
	{"north-path", "North Path"},
	{"south-pond", "South Pond"},
}

// Run inserts the development park and zones idempotently.
func Run(ctx context.Context, appEnv string, store Store, now time.Time) (Result, error) {
	if !Permitted(appEnv) {
		return Result{}, ErrForbiddenEnvironment
	}
	p, err := park.NewPark(lumpini, now)
	if err != nil {
		return Result{}, err
	}
	stored, created, err := store.EnsurePark(ctx, p)
	if err != nil {
		return Result{}, err
	}
	result := Result{ParkID: stored.ID, ParkCreated: created}
	for _, z := range lumpiniZones {
		zone, err := park.NewZone(park.ZoneInput{ParkID: stored.ID, Slug: z.slug, Name: z.name, Status: park.StatusActive}, now)
		if err != nil {
			return Result{}, err
		}
		_, created, err := store.EnsureZone(ctx, zone)
		if err != nil {
			return Result{}, err
		}
		if created {
			result.ZonesCreated++
		} else {
			result.ZonesExisting++
		}
	}
	return result, nil
}
