// Package location models precise encounter coordinates. A Private location
// is personal data: it is persisted separately from the encounter and is
// never part of a public response or a log line.
package location

import (
	"errors"
	"math"
)

// Sources describe how the coordinates were obtained.
const (
	SourceGPS     = "GPS"
	SourceManual  = "MANUAL"
	SourceImport  = "IMPORT"
	SourceUnknown = "UNKNOWN"
)

var (
	ErrInvalidLatitude  = errors.New("latitude must be between -90 and 90")
	ErrInvalidLongitude = errors.New("longitude must be between -180 and 180")
	ErrInvalidAccuracy  = errors.New("accuracyMeters must be a finite number >= 0")
	ErrInvalidSource    = errors.New("source must be GPS, MANUAL, IMPORT, or UNKNOWN")
)

// Private is a WGS 84 (SRID 4326) point with optional accuracy.
type Private struct {
	Latitude       float64
	Longitude      float64
	AccuracyMeters *float64
	Source         string
}

// String deliberately hides the coordinates so an accidental %v/%s in a log
// or error message cannot leak them.
func (Private) String() string { return "location.Private{redacted}" }

// GoString hides the coordinates from %#v as well.
func (p Private) GoString() string { return p.String() }

func (p Private) Validate() error {
	if math.IsNaN(p.Latitude) || p.Latitude < -90 || p.Latitude > 90 {
		return ErrInvalidLatitude
	}
	if math.IsNaN(p.Longitude) || p.Longitude < -180 || p.Longitude > 180 {
		return ErrInvalidLongitude
	}
	if p.AccuracyMeters != nil {
		a := *p.AccuracyMeters
		if math.IsNaN(a) || math.IsInf(a, 0) || a < 0 {
			return ErrInvalidAccuracy
		}
	}
	switch p.Source {
	case SourceGPS, SourceManual, SourceImport, SourceUnknown:
		return nil
	}
	return ErrInvalidSource
}
