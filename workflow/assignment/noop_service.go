package assignment

import (
	"context"

	"github.com/goodbye-jack/go-common/workflow/types"
)

type NoopService struct{}

func NewNoopService() *NoopService {
	return &NoopService{}
}

func (s *NoopService) ResolveStart(ctx context.Context, req *types.AssignmentResolveRequest) (*types.AssignmentResolveResponse, error) {
	return nil, nil
}

func (s *NoopService) ResolveComplete(ctx context.Context, req *types.AssignmentResolveRequest) (*types.AssignmentResolveResponse, error) {
	return nil, nil
}
