package directory

import (
	"context"

	"github.com/goodbye-jack/go-common/workflow/types"
)

type NoopService struct{}

func NewNoopService() *NoopService {
	return &NoopService{}
}

func (s *NoopService) ValidateUser(ctx context.Context, userID, password string) (*types.DirectoryUserProfile, error) {
	return nil, ErrNotConfigured
}

func (s *NoopService) GetCurrentUser(ctx context.Context, userID string) (*types.DirectoryUserProfile, error) {
	return nil, ErrNotConfigured
}

func (s *NoopService) GetUser(ctx context.Context, userID string) (*types.DirectoryUserProfile, error) {
	return nil, ErrNotConfigured
}

func (s *NoopService) GetManager(ctx context.Context, userID string) (*types.DirectoryUserSummary, error) {
	return nil, ErrNotConfigured
}

func (s *NoopService) GetDepartment(ctx context.Context, userID string) (*types.DirectoryDepartment, error) {
	return nil, ErrNotConfigured
}
