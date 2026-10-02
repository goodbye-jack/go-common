package formref

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goodbye-jack/go-common/orm"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/goodbye-jack/go-common/workflow/engine/flowable"
	"github.com/spf13/viper"
	"gorm.io/gorm/logger"
)

type formSQLCounter struct {
	logger.Interface
	queries atomic.Int64
}

func (counter *formSQLCounter) Trace(ctx context.Context, begin time.Time, sql func() (string, int64), err error) {
	counter.queries.Add(1)
}

func TestResolveRESTOnlyNeverQueriesDB(t *testing.T) {
	const instanceName = "form_rest_only_test"
	database := orm.NewOrm("file:form-rest-only?mode=memory&cache=shared", utils.DBTypeSQLite, 1)
	if database == nil || database.GetDB() == nil {
		t.Fatal("创建测试数据库失败")
	}
	counter := &formSQLCounter{Interface: logger.Discard}
	originalLogger := database.GetDB().Logger
	database.GetDB().Logger = counter
	originalInstance := viper.Get(configFormRefDBInstance)
	originalEnabled := viper.Get(configFormRefDBFallbackEnabled)
	originalORM := orm.RelationalMap[instanceName]
	orm.RelationalMap[instanceName] = database
	viper.Set(configFormRefDBInstance, instanceName)
	viper.Set(configFormRefDBFallbackEnabled, false)
	t.Cleanup(func() {
		database.GetDB().Logger = originalLogger
		viper.Set(configFormRefDBInstance, originalInstance)
		viper.Set(configFormRefDBFallbackEnabled, originalEnabled)
		if originalORM == nil {
			delete(orm.RelationalMap, instanceName)
		} else {
			orm.RelationalMap[instanceName] = originalORM
		}
		connection, err := database.GetDB().DB()
		if err == nil {
			connection.Close()
		}
	})
	for _, test := range []struct {
		name      string
		formData  string
		dataCode  int
		modelCode int
		resolved  bool
		wantError bool
		history   bool
	}{
		{name: "独立FormEngine", dataCode: 200, formData: `{"formKey":"approval","deploymentId":"bpmn-dep"}`, modelCode: 200, resolved: true},
		{name: "历史任务", dataCode: 404, modelCode: 404, history: true, resolved: true},
		{name: "仅表单key", dataCode: 200, formData: `{"formKey":"approval"}`, modelCode: 400},
		{name: "权限拒绝", dataCode: 403, modelCode: 403, wantError: true},
		{name: "禁止认证失败后兜底", dataCode: 401, modelCode: 200, wantError: true},
		{name: "不能用正文伪造404", dataCode: 500, formData: "status=404", modelCode: 200, wantError: true},
		{name: "历史模型缺失保留表单key", dataCode: 404, modelCode: 404},
		{name: "服务故障", dataCode: 200, formData: `{"formKey":"approval"}`, modelCode: 500, wantError: true},
		{name: "formProperties兼容", dataCode: 200, formData: `{"formProperties":[{"id":"title","type":"string"}]}`, resolved: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			paths := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				paths = append(paths, request.URL.Path)
				switch request.URL.Path {
				case "/form/form-data":
					writer.WriteHeader(test.dataCode)
					writer.Write([]byte(test.formData))
				case "/repository/deployments/bpmn-dep/resources":
					writer.Write([]byte(`[]`))
				case "/runtime/tasks/task-1/form", "/history/historic-task-instances/task-1/form":
					status := test.modelCode
					if test.history && request.URL.Path == "/history/historic-task-instances/task-1/form" {
						status = 200
					}
					writer.WriteHeader(status)
					if status == 400 {
						_, _ = writer.Write([]byte(`{"message":"Bad request","exception":"Task has no form defined"}`))
						return
					}
					writer.Write([]byte(`{"id":"form-v1","key":"approval","name":"审批表单","fields":[{"id":"title","name":"标题","type":"text","required":true}],"outcomes":[{"id":"approve","name":"同意"}]}`))
				default:
					t.Errorf("意外请求: %s", request.URL.Path)
					writer.WriteHeader(500)
				}
			}))
			defer server.Close()
			client, err := flowable.NewRESTClient(flowable.Config{BaseURL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			ref, err := NewFlowableService(client).Resolve(context.Background(), &TaskFormLocator{TaskID: "task-1", FormKey: "approval"})
			if (err != nil) != test.wantError {
				t.Fatalf("ref=%+v err=%v", ref, err)
			}
			if !test.wantError && (ref == nil || ref.Resolved != test.resolved) {
				t.Fatalf("ref=%+v", ref)
			}
			if ref != nil && ref.Source == "flowable-form-model" {
				if len(ref.Fields) != 1 || !ref.Fields[0].Required || len(ref.Outcomes) != 1 || ref.Outcomes[0].ID != "approve" || ref.DeploymentID != "" {
					t.Fatalf("表单解析或部署ID错误: %+v", ref)
				}
			}
			if test.name == "formProperties兼容" && !reflect.DeepEqual(paths, []string{"/form/form-data"}) {
				t.Fatalf("旧表单不应增加请求: %v", paths)
			}
			if test.dataCode != 200 && test.dataCode != 404 && len(paths) != 1 {
				t.Fatalf("上游故障后不应继续兜底: %v", paths)
			}
			if queries := counter.queries.Load(); queries != 0 {
				t.Fatalf("REST-only 模式执行了 %d 条 SQL", queries)
			}
		})
	}
	_, err := (&FlowableService{}).Resolve(context.Background(), &TaskFormLocator{TaskID: "no-client", FormKey: "approval"})
	if err != nil || counter.queries.Load() != 0 {
		t.Fatalf("无客户端时不应访问数据库: %v", err)
	}
}
