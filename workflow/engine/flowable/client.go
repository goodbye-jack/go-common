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
	ListTaskRecords(ctx context.Context, user *workflowcontext.UserContext, query *types.TaskQuery) (*types.WorkflowTaskRecordPage, error)
	GetTaskContext(ctx context.Context, taskID string, user *workflowcontext.UserContext) (*types.TaskContextResponse, error)
	ClaimTask(ctx context.Context, taskID string, user *workflowcontext.UserContext) (*types.TaskActionResponse, error)
	UnclaimTask(ctx context.Context, taskID string, user *workflowcontext.UserContext) (*types.TaskActionResponse, error)
	CompleteTask(ctx context.Context, taskID string, req *types.CompleteTaskRequest, user *workflowcontext.UserContext) error
	DelegateTask(ctx context.Context, taskID string, req *types.TaskDelegateRequest, user *workflowcontext.UserContext) (*types.TaskActionResponse, error)
	ResolveTask(ctx context.Context, taskID string, req *types.TaskResolveRequest, user *workflowcontext.UserContext) (*types.TaskActionResponse, error)
	TransferTask(ctx context.Context, taskID string, req *types.TaskTransferRequest, user *workflowcontext.UserContext) (*types.TaskActionResponse, error)
	GetProgressView(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessProgressViewResponse, error)
	GetProgressTimeline(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessProgressTimelineResponse, error)
	GetActionTimeline(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessActionTimelineResponse, error)
	GetTaskRecords(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessTaskRecordResponse, error)
	GetProgressViewByBizID(ctx context.Context, bizID string, user *workflowcontext.UserContext) (*types.ProcessProgressViewResponse, error)
	GetProgressTimelineByBizID(ctx context.Context, bizID string, user *workflowcontext.UserContext) (*types.ProcessProgressTimelineResponse, error)
	GetActionTimelineByBizID(ctx context.Context, bizID string, user *workflowcontext.UserContext) (*types.ProcessActionTimelineResponse, error)
	GetTaskRecordsByBizID(ctx context.Context, bizID string, user *workflowcontext.UserContext) (*types.ProcessTaskRecordResponse, error)
	GetDefinitionXML(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) ([]byte, error)
	GetDiagramView(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessCompositeDiagramResponse, error)
	GetCompositeDiagram(ctx context.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessCompositeDiagramResponse, error)
}
