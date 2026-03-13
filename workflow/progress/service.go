package progress

import (
	"context"

	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/types"
)

type Service interface {
	EnrichTaskPage(ctx context.Context, user *workflowcontext.UserContext, page *types.TaskPage) error
	GetView(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessProgressViewResponse, error)
	GetTimeline(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessProgressTimelineResponse, error)
}
