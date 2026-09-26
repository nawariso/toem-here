// Package hia models an individual monitor lizard. The internal ID is a UUID;
// the stable public identity is the server-generated PublicCode (HIA-000001).
package hia

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	StatusProvisional = "PROVISIONAL"
	StatusConfirmed   = "CONFIRMED"
	StatusInactive    = "INACTIVE"
	StatusArchived    = "ARCHIVED"
	StatusMerged      = "MERGED"
)

var (
	ErrInvalidStatus      = errors.New("status must be PROVISIONAL, CONFIRMED, INACTIVE, ARCHIVED, or MERGED")
	ErrInvalidNickname    = errors.New("nickname must be 1-60 characters")
	ErrMergeIntoSelf      = errors.New("a hia cannot be merged into itself")
	ErrMergeTargetMissing = errors.New("status MERGED requires mergedIntoHiaId, and only MERGED may set it")
	ErrInvalidPublicCode  = errors.New("publicCode must look like HIA-000001")
	ErrInvalidID          = errors.New("hia id must be a UUID")
	ErrNotFound           = errors.New("hia not found")

	publicCodePattern = regexp.MustCompile(`^HIA-[0-9]{6,}$`)
)

// Hia is an individual animal. PublicCode and PublicNumber are assigned by
// the database on insert and never change afterwards.
type Hia struct {
	ID              string
	PublicNumber    int64
	PublicCode      string
	Nickname        *string
	Status          string
	HomeParkID      *string
	FirstSeenAt     *time.Time
	ConfirmedAt     *time.Time
	MergedIntoHiaID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewProvisional returns an unsaved PROVISIONAL hia. Its public code is empty
// until persistence allocates one.
func NewProvisional(nickname, homeParkID *string, now time.Time) (Hia, error) {
	ts := now.UTC().Truncate(time.Microsecond)
	h := Hia{ID: uuid.NewString(), Nickname: trimmed(nickname), Status: StatusProvisional, HomeParkID: homeParkID, CreatedAt: ts, UpdatedAt: ts}
	return h, h.Validate()
}

// Validate enforces the invariants the database also enforces.
func (h Hia) Validate() error {
	if _, err := uuid.Parse(h.ID); err != nil {
		return ErrInvalidID
	}
	if err := ValidateStatus(h.Status); err != nil {
		return err
	}
	if h.Nickname != nil {
		if n := len([]rune(*h.Nickname)); n < 1 || n > 60 {
			return ErrInvalidNickname
		}
	}
	if h.MergedIntoHiaID != nil && strings.EqualFold(*h.MergedIntoHiaID, h.ID) {
		return ErrMergeIntoSelf
	}
	if (h.Status == StatusMerged) != (h.MergedIntoHiaID != nil) {
		return ErrMergeTargetMissing
	}
	return nil
}

func ValidateStatus(status string) error {
	switch status {
	case StatusProvisional, StatusConfirmed, StatusInactive, StatusArchived, StatusMerged:
		return nil
	}
	return ErrInvalidStatus
}

// FormatPublicCode renders a sequence number as HIA-000001. Numbers beyond
// 999999 keep all their digits (HIA-1000000) rather than being truncated.
func FormatPublicCode(n int64) string { return fmt.Sprintf("HIA-%06d", n) }

// ValidatePublicCode checks the shape of a client-supplied public code.
func ValidatePublicCode(code string) error {
	if !publicCodePattern.MatchString(code) {
		return ErrInvalidPublicCode
	}
	return nil
}

func trimmed(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	return &v
}
