package assignment

import (
	"context"

	"github.com/goodbye-jack/go-common/workflow/types"
)

type Service interface {
	ResolveStart(ctx context.Context, req *types.AssignmentResolveRequest) (*types.AssignmentResolveResponse, error)
	ResolveComplete(ctx context.Context, req *types.AssignmentResolveRequest) (*types.AssignmentResolveResponse, error)
}
