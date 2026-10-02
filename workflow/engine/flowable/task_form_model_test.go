package flowable

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestGetTaskFormModel(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		body      string
		history   bool
		wantError bool
		wantModel bool
	}{
		{name: "运行中表单", status: 200, body: `{"key":"approval","fields":[]}`, wantModel: true},
		{name: "历史表单", status: 404, history: true, wantModel: true},
		{name: "未配置模型", status: 400, body: `{"message":"Bad request","exception":"Task has no form defined"}`},
		{name: "其他400", status: 400, body: `{"message":"Bad request","exception":"Invalid request"}`, wantError: true},
		{name: "空400", status: 400, wantError: true},
		{name: "禁止访问", status: 403, body: "secret", wantError: true},
		{name: "服务异常", status: 500, body: "secret", wantError: true},
		{name: "错误JSON", status: 200, body: `{"key":"approval"}`, wantError: true},
		{name: "空模型", status: 200, body: `null`, wantError: true},
		{name: "重定向", status: 302, wantError: true},
		{name: "响应过大", status: 200, body: strings.Repeat(" ", (4<<20)+1), wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			paths := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				paths = append(paths, request.URL.EscapedPath())
				username, password, ok := request.BasicAuth()
				if !ok || username != "reader" || password != "test-only" || request.Method != http.MethodGet || request.URL.RawQuery != "" {
					t.Error("请求方法、凭据或编码错误")
				}
				if test.history && len(paths) == 2 {
					writer.Write([]byte(`{"key":"approval","fields":[]}`))
					return
				}
				writer.Header().Set("Location", "/should-not-follow")
				writer.WriteHeader(test.status)
				writer.Write([]byte(test.body))
			}))
			defer server.Close()
			client, err := NewRESTClient(Config{BaseURL: server.URL, Username: "reader", Password: "test-only"})
			if err != nil {
				t.Fatal(err)
			}
			model, err := client.GetTaskFormModel(context.Background(), "task/one?key=other")
			if (err != nil) != test.wantError || (model != nil) != test.wantModel {
				t.Fatalf("model=%v err=%v", model, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("错误泄露响应正文")
			}
			expected := []string{"/runtime/tasks/task%2Fone%3Fkey=other/form"}
			if test.history {
				expected = append(expected, "/history/historic-task-instances/task%2Fone%3Fkey=other/form")
			}
			if !reflect.DeepEqual(paths, expected) {
				t.Fatalf("请求路径=%v", paths)
			}
		})
	}
}

func TestGetTaskFormModelCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Error("已取消请求不应访问服务端")
	}))
	defer server.Close()
	client, err := NewRESTClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.GetTaskFormModel(ctx, "task-1"); err != context.Canceled {
		t.Fatalf("err=%v", err)
	}
}
