package application

import (
	"context"
	"github.com/nawariso/toem-hia/services/api/internal/domain"
)

type UserService struct{ repo UserRepository }

func NewUserService(repo UserRepository) *UserService { return &UserService{repo: repo} }
func (s *UserService) Bootstrap(ctx context.Context, identity domain.ExternalIdentity) (domain.User, error) {
	if err := domain.ValidateIdentity(identity); err != nil {
		return domain.User{}, err
	}
	return s.repo.Bootstrap(ctx, identity)
}
func (s *UserService) Current(ctx context.Context, identity domain.ExternalIdentity) (domain.User, error) {
	if err := domain.ValidateIdentity(identity); err != nil {
		return domain.User{}, err
	}
	return s.repo.FindByIdentity(ctx, identity)
}
func (s *UserService) UpdateProfile(ctx context.Context, identity domain.ExternalIdentity, patch domain.ProfilePatch) (domain.User, error) {
	if err := domain.ValidateIdentity(identity); err != nil {
		return domain.User{}, err
	}
	if err := domain.ValidatePatch(patch); err != nil {
		return domain.User{}, err
	}
	return s.repo.UpdateProfile(ctx, identity, patch)
}
