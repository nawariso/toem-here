package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const RoleUser = "USER"

var (
	ErrInvalidIdentity    = errors.New("invalid external identity")
	ErrInvalidUsername    = errors.New("username must be 3-30 characters using letters, numbers, or underscore")
	ErrInvalidDisplayName = errors.New("display name must be 1-80 characters")
	ErrInvalidLocale      = errors.New("locale must be a supported language tag")
	ErrUsernameTaken      = errors.New("username is already taken")
	usernamePattern       = regexp.MustCompile(`^[A-Za-z0-9_]{3,30}$`)
	localePattern         = regexp.MustCompile(`^[a-z]{2}(?:-[A-Z]{2})?$`)
)

type ExternalIdentity struct {
	Provider string
	Subject  string
	Email    *string
}
type User struct {
	ID          string     `json:"id"`
	Username    *string    `json:"username"`
	DisplayName *string    `json:"displayName"`
	AvatarURL   *string    `json:"avatarUrl"`
	Locale      string     `json:"locale"`
	Status      string     `json:"status,omitempty"`
	Roles       []string   `json:"roles"`
	CreatedAt   time.Time  `json:"createdAt,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt,omitempty"`
	DeletedAt   *time.Time `json:"deletedAt,omitempty"`
}

func NewUser(locale string) User {
	now := time.Now().UTC()
	return User{ID: uuid.NewString(), Locale: locale, Status: "ACTIVE", CreatedAt: now, UpdatedAt: now, Roles: []string{}}
}
func (u User) ProfileComplete() bool {
	return u.Username != nil && strings.TrimSpace(*u.Username) != "" && u.DisplayName != nil && strings.TrimSpace(*u.DisplayName) != ""
}

type ProfilePatch struct {
	Username    *string
	DisplayName *string
	Locale      *string
}

func ValidateIdentity(i ExternalIdentity) error {
	if strings.TrimSpace(i.Provider) == "" || strings.TrimSpace(i.Subject) == "" {
		return ErrInvalidIdentity
	}
	return nil
}
func ValidatePatch(p ProfilePatch) error {
	if p.Username != nil && !usernamePattern.MatchString(*p.Username) {
		return ErrInvalidUsername
	}
	if p.DisplayName != nil {
		n := len([]rune(strings.TrimSpace(*p.DisplayName)))
		if n < 1 || n > 80 {
			return ErrInvalidDisplayName
		}
	}
	if p.Locale != nil && !localePattern.MatchString(*p.Locale) {
		return ErrInvalidLocale
	}
	return nil
}
