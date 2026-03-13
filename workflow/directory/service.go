package directory

import (
	"context"

	"github.com/goodbye-jack/go-common/workflow/types"
)

type Service interface {
	ValidateUser(ctx context.Context, userID, password string) (*types.DirectoryUserProfile, error)
	GetCurrentUser(ctx context.Context, userID string) (*types.DirectoryUserProfile, error)
	GetUser(ctx context.Context, userID string) (*types.DirectoryUserProfile, error)
	GetManager(ctx context.Context, userID string) (*types.DirectoryUserSummary, error)
	GetDepartment(ctx context.Context, userID string) (*types.DirectoryDepartment, error)
}
