package approvalbridge

import (
	"context"

	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/types"
)

type Bridge interface {
	CreateApprovalProcess(ctx context.Context, user *workflowcontext.UserContext, req *types.StartProcessRequest) (*types.StartProcessResponse, error)
}
