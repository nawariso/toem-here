package application

import (
	"context"
	"errors"

	"github.com/nawariso/toem-here/services/api/internal/domain"
)

var ErrNotFound = errors.New("user not found")

type IdentityVerifier interface {
	Verify(context.Context, string) (domain.ExternalIdentity, error)
}
type UserRepository interface {
	Bootstrap(context.Context, domain.ExternalIdentity) (domain.User, error)
	FindByIdentity(context.Context, domain.ExternalIdentity) (domain.User, error)
	UpdateProfile(context.Context, domain.ExternalIdentity, domain.ProfilePatch) (domain.User, error)
}
type Readiness interface{ Ping(context.Context) error }
type Users interface {
	Bootstrap(context.Context, domain.ExternalIdentity) (domain.User, error)
	Current(context.Context, domain.ExternalIdentity) (domain.User, error)
	UpdateProfile(context.Context, domain.ExternalIdentity, domain.ProfilePatch) (domain.User, error)
}
