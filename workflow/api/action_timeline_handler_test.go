package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/types"
)

type stubResolver struct {
	user *workflowcontext.UserContext
	err  error
}

func (s stubResolver) Resolve(c *gin.Context) (*workflowcontext.UserContext, error) {
	return s.user, s.err
}

type stubFlowableClient struct{}

func (stubFlowableClient) StartProcess(context.Context, *types.StartProcessRequest) (*types.StartProcessResponse, error) {
	return nil, nil
}
func (stubFlowableClient) ProcessCallback(context.Context, *types.FlowableCallbackPayload) error {
	return nil
}
func (stubFlowableClient) ListTodo(context.Context, *workflowcontext.UserContext, *types.TaskQuery) (*types.TaskPage, error) {
	return nil, nil
}
func (stubFlowableClient) ListDone(context.Context, *workflowcontext.UserContext, *types.TaskQuery) (*types.TaskPage, error) {
	return nil, nil
}
func (stubFlowableClient) ListTaskRecords(context.Context, *workflowcontext.UserContext, *types.TaskQuery) (*types.WorkflowTaskRecordPage, error) {
	return nil, nil
}
func (stubFlowableClient) GetTaskContext(context.Context, string, *workflowcontext.UserContext) (*types.TaskContextResponse, error) {
	return nil, nil
}
func (stubFlowableClient) ClaimTask(context.Context, string, *workflowcontext.UserContext) (*types.TaskActionResponse, error) {
	return nil, nil
}
func (stubFlowableClient) UnclaimTask(context.Context, string, *workflowcontext.UserContext) (*types.TaskActionResponse, error) {
	return nil, nil
}
func (stubFlowableClient) CompleteTask(context.Context, string, *types.CompleteTaskRequest, *workflowcontext.UserContext) error {
	return nil
}
func (stubFlowableClient) DelegateTask(context.Context, string, *types.TaskDelegateRequest, *workflowcontext.UserContext) (*types.TaskActionResponse, error) {
	return nil, nil
}
func (stubFlowableClient) ResolveTask(context.Context, string, *types.TaskResolveRequest, *workflowcontext.UserContext) (*types.TaskActionResponse, error) {
	return nil, nil
}
func (stubFlowableClient) TransferTask(context.Context, string, *types.TaskTransferRequest, *workflowcontext.UserContext) (*types.TaskActionResponse, error) {
	return nil, nil
}
func (stubFlowableClient) GetProgressView(context.Context, string, *workflowcontext.UserContext) (*types.ProcessProgressViewResponse, error) {
	return nil, nil
}
func (stubFlowableClient) GetProgressTimeline(context.Context, string, *workflowcontext.UserContext) (*types.ProcessProgressTimelineResponse, error) {
	return nil, nil
}
func (stubFlowableClient) GetActionTimeline(context.Context, string, *workflowcontext.UserContext) (*types.ProcessActionTimelineResponse, error) {
	return &types.ProcessActionTimelineResponse{
		Summary: types.ProcessActionTimelineSummary{
			ProcessInstanceID: "process-001",
			Status:            "RUNNING",
		},
		Items: []types.ProcessActionTimelineItem{},
	}, nil
}
func (stubFlowableClient) GetTaskRecords(context.Context, string, *workflowcontext.UserContext) (*types.ProcessTaskRecordResponse, error) {
	return nil, nil
}
func (stubFlowableClient) GetProgressViewByBizID(context.Context, string, *workflowcontext.UserContext) (*types.ProcessProgressViewResponse, error) {
	return nil, nil
}
func (stubFlowableClient) GetProgressTimelineByBizID(context.Context, string, *workflowcontext.UserContext) (*types.ProcessProgressTimelineResponse, error) {
	return nil, nil
}
func (stubFlowableClient) GetActionTimelineByBizID(context.Context, string, *workflowcontext.UserContext) (*types.ProcessActionTimelineResponse, error) {
	return nil, nil
}
func (stubFlowableClient) GetTaskRecordsByBizID(context.Context, string, *workflowcontext.UserContext) (*types.ProcessTaskRecordResponse, error) {
	return nil, nil
}
func (stubFlowableClient) GetDefinitionXML(context.Context, string, *workflowcontext.UserContext) ([]byte, error) {
	return nil, nil
}
func (stubFlowableClient) GetDiagramView(context.Context, string, *workflowcontext.UserContext) (*types.ProcessCompositeDiagramResponse, error) {
	return nil, nil
}
func (stubFlowableClient) GetCompositeDiagram(context.Context, string, *workflowcontext.UserContext) (*types.ProcessCompositeDiagramResponse, error) {
	return nil, nil
}

func TestHandleActionTimelineReturnsEmptyItemsArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/workflow/process-instances/process-001/action-timeline", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "process-001"}}

	module := &DefaultModule{
		resolver: stubResolver{
			user: &workflowcontext.UserContext{UserID: "test1"},
		},
		flowable: stubFlowableClient{},
	}

	module.handleActionTimeline(ctx)

	if recorder.Code != 200 {
		t.Fatalf("status=%d, want 200", recorder.Code)
	}

	var payload struct {
		Data struct {
			Summary map[string]interface{}   `json:"summary"`
			Items   []map[string]interface{} `json:"items"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if payload.Message != "success" {
		t.Fatalf("message=%q, want success", payload.Message)
	}
	if payload.Data.Items == nil {
		t.Fatalf("expected data.items to be present as empty array, got nil")
	}
	if len(payload.Data.Items) != 0 {
		t.Fatalf("expected empty items array, got len=%d", len(payload.Data.Items))
	}
}
