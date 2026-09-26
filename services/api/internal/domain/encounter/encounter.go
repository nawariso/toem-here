// Package encounter models one observation: a user saw a monitor lizard at a
// place and time. An Encounter deliberately has no hia reference; linking an
// encounter to a Hia belongs to the future Identification/Verification
// boundary, not to the observer.
package encounter

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Statuses. Requirement 002 only moves DRAFT -> SUBMITTED; the others are
// reserved for the future identification workflow and are never set here.
const (
	StatusDraft       = "DRAFT"
	StatusSubmitted   = "SUBMITTED"
	StatusProcessing  = "PROCESSING"
	StatusNeedsReview = "NEEDS_REVIEW"
	StatusConfirmed   = "CONFIRMED"
	StatusRejected    = "REJECTED"
)

// Behaviors. Free text belongs in Notes.
const (
	BehaviorBasking  = "BASKING"
	BehaviorSwimming = "SWIMMING"
	BehaviorWalking  = "WALKING"
	BehaviorResting  = "RESTING"
	BehaviorEating   = "EATING"
	BehaviorClimbing = "CLIMBING"
	BehaviorOther    = "OTHER"
)

// MaxNotesLength bounds free text (characters, not bytes).
const MaxNotesLength = 2000

var (
	ErrCapturedAtRequired = errors.New("capturedAt is required")
	ErrInvalidBehavior    = errors.New("behavior must be BASKING, SWIMMING, WALKING, RESTING, EATING, CLIMBING, or OTHER")
	ErrNotesTooLong       = errors.New("notes must be at most 2000 characters")
	ErrZoneRequiresPark   = errors.New("zoneId requires parkId")
	ErrZoneNotInPark      = errors.New("zone does not belong to the given park")
	ErrParkNotFound       = errors.New("parkId does not reference a park")
	ErrZoneNotFound       = errors.New("zoneId does not reference a zone")
	ErrInvalidReference   = errors.New("parkId and zoneId must be UUIDs")
	ErrObserverRequired   = errors.New("encounter observer is required")
	ErrNotFound           = errors.New("encounter not found")
	ErrNotEditable        = errors.New("only DRAFT encounters can be edited")
	ErrInvalidTransition  = errors.New("encounter cannot be submitted from its current status")
	ErrUnknownStatus      = errors.New("unknown encounter status")
)

type Encounter struct {
	ID             string
	ObserverUserID string
	CapturedAt     time.Time
	SubmittedAt    *time.Time
	ParkID         *string
	ZoneID         *string
	Status         string
	Behavior       *string
	Notes          *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Details are the observer-editable attributes. Status, observer, and
// timestamps other than CapturedAt are never client-controlled.
type Details struct {
	CapturedAt time.Time
	ParkID     *string
	ZoneID     *string
	Behavior   *string
	Notes      *string
}

// NewDraft creates a DRAFT encounter owned by observerUserID, which must come
// from the authenticated internal user, never from the request body.
func NewDraft(observerUserID string, d Details, now time.Time) (Encounter, error) {
	if _, err := uuid.Parse(observerUserID); err != nil {
		return Encounter{}, ErrObserverRequired
	}
	ts := now.UTC().Truncate(time.Microsecond)
	e := Encounter{ID: uuid.NewString(), ObserverUserID: observerUserID, Status: StatusDraft, CreatedAt: ts, UpdatedAt: ts}
	if err := e.apply(d); err != nil {
		return Encounter{}, err
	}
	return e, nil
}

// Edit replaces the editable details of a DRAFT encounter.
func (e *Encounter) Edit(d Details, now time.Time) error {
	if e.Status != StatusDraft {
		return ErrNotEditable
	}
	if err := e.apply(d); err != nil {
		return err
	}
	e.UpdatedAt = now.UTC().Truncate(time.Microsecond)
	return nil
}

// Submit moves DRAFT -> SUBMITTED. Submitting an already SUBMITTED encounter
// is a deterministic no-op: changed is false and submittedAt is unchanged.
func (e *Encounter) Submit(now time.Time) (changed bool, err error) {
	switch e.Status {
	case StatusDraft:
		ts := now.UTC().Truncate(time.Microsecond)
		e.Status, e.SubmittedAt, e.UpdatedAt = StatusSubmitted, &ts, ts
		return true, nil
	case StatusSubmitted:
		return false, nil
	case StatusProcessing, StatusNeedsReview, StatusConfirmed, StatusRejected:
		return false, ErrInvalidTransition
	}
	return false, ErrUnknownStatus
}

// OwnedBy reports whether userID is the encounter's observer.
func (e Encounter) OwnedBy(userID string) bool {
	return userID != "" && e.ObserverUserID == userID
}

func (e *Encounter) apply(d Details) error {
	if err := d.Validate(); err != nil {
		return err
	}
	e.CapturedAt = d.CapturedAt.UTC().Truncate(time.Microsecond)
	e.ParkID, e.ZoneID, e.Behavior = d.ParkID, d.ZoneID, d.Behavior
	e.Notes = normalizeNotes(d.Notes)
	return nil
}

// Validate checks the details that do not need persistence. Whether the
// referenced park and zone exist and match is checked by the application
// service and enforced again by a composite foreign key.
func (d Details) Validate() error {
	if d.CapturedAt.IsZero() {
		return ErrCapturedAtRequired
	}
	for _, ref := range []*string{d.ParkID, d.ZoneID} {
		if ref != nil {
			if _, err := uuid.Parse(*ref); err != nil {
				return ErrInvalidReference
			}
		}
	}
	if d.ZoneID != nil && d.ParkID == nil {
		return ErrZoneRequiresPark
	}
	if d.Behavior != nil && !ValidBehavior(*d.Behavior) {
		return ErrInvalidBehavior
	}
	if d.Notes != nil && len([]rune(*d.Notes)) > MaxNotesLength {
		return ErrNotesTooLong
	}
	return nil
}

func ValidBehavior(b string) bool {
	switch b {
	case BehaviorBasking, BehaviorSwimming, BehaviorWalking, BehaviorResting, BehaviorEating, BehaviorClimbing, BehaviorOther:
		return true
	}
	return false
}

// normalizeNotes trims notes and treats blank notes as absent.
func normalizeNotes(notes *string) *string {
	if notes == nil {
		return nil
	}
	v := strings.TrimSpace(*notes)
	if v == "" {
		return nil
	}
	return &v
}
