package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/formref"
	"github.com/goodbye-jack/go-common/workflow/types"
	"github.com/spf13/viper"
)

type formContextClient struct {
	stubFlowableClient
	err error
}

func (client formContextClient) GetTaskContext(context.Context, string, *workflowcontext.UserContext) (*types.TaskContextResponse, error) {
	return &types.TaskContextResponse{Task: types.TaskInfo{TaskID: "task-1"}}, client.err
}

type failingFormService struct {
	calls int
}

func (service *failingFormService) Resolve(context.Context, *formref.TaskFormLocator) (*types.TaskFormReference, error) {
	service.calls++
	return nil, errors.New("任务表单查询失败")
}

func TestFormHandlersPropagateRESTOnlyErrors(t *testing.T) {
	original := viper.Get("workflow.formref.db_fallback_enabled")
	t.Cleanup(func() { viper.Set("workflow.formref.db_fallback_enabled", original) })
	for _, endpoint := range []string{"context", "form-ref"} {
		for _, test := range []struct {
			name     string
			fallback bool
			denied   bool
			status   int
		}{
			{name: "REST-only错误", status: 500},
			{name: "旧模式兼容", fallback: true, status: 200},
			{name: "先检查任务权限", denied: true, status: 403},
		} {
			t.Run(endpoint+"/"+test.name, func(t *testing.T) {
				viper.Set("workflow.formref.db_fallback_enabled", test.fallback)
				service := &failingFormService{}
				client := formContextClient{}
				if test.denied {
					client.err = errors.New("forbidden")
				}
				module := &DefaultModule{
					resolver: stubResolver{user: &workflowcontext.UserContext{UserID: "reader"}},
					flowable: client, formref: service,
				}
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodGet, "/tasks/task-1/"+endpoint, nil)
				ctx.Params = gin.Params{{Key: "id", Value: "task-1"}}
				if endpoint == "context" {
					module.handleTaskContext(ctx)
				} else {
					module.handleTaskFormRef(ctx)
				}
				if recorder.Code != test.status {
					t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
				}
				if test.status != 200 && strings.Contains(recorder.Body.String(), `"message":"success"`) {
					t.Fatal("错误被转换为成功")
				}
				if test.denied && service.calls != 0 {
					t.Fatal("未授权用户触发表单查询")
				}
			})
		}
	}
}
