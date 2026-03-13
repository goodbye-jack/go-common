package flowable

import (
	"context"

	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/types"
)

type Config struct {
	BaseURL        string
	Username       string
	Password       string
	TimeoutSeconds int
}

type DeploymentResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Client interface {
	StartProcess(ctx context.Context, req *types.StartProcessRequest) (*types.StartProcessResponse, error)
	ProcessCallback(ctx context.Context, payload *types.FlowableCallbackPayload) error
	ListTodo(ctx context.Context, user *workflowcontext.UserContext, query *types.TaskQuery) (*types.TaskPage, error)
	ListDone(ctx context.Context, user *workflowcontext.UserContext, query *types.TaskQuery) (*types.TaskPage, error)
	GetTaskContext(ctx context.Context, taskID string, user *workflowcontext.UserContext) (*types.TaskContextResponse, error)
	CompleteTask(ctx context.Context, taskID string, req *types.CompleteTaskRequest, user *workflowcontext.UserContext) error
	GetProgressView(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessProgressViewResponse, error)
	GetProgressTimeline(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessProgressTimelineResponse, error)
	GetProgressViewByBizID(ctx context.Context, bizID string, user *workflowcontext.UserContext) (*types.ProcessProgressViewResponse, error)
	GetProgressTimelineByBizID(ctx context.Context, bizID string, user *workflowcontext.UserContext) (*types.ProcessProgressTimelineResponse, error)
	GetDefinitionXML(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) ([]byte, error)
	GetDiagramView(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessCompositeDiagramResponse, error)
	GetCompositeDiagram(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessCompositeDiagramResponse, error)
}
