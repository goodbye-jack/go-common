package flowable

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// TaskFormDataError 表示任务表单信息请求的 HTTP 错误，不暴露上游响应正文。
type TaskFormDataError struct {
	StatusCode int
}

func (err *TaskFormDataError) Error() string {
	return fmt.Sprintf("任务表单信息查询失败，HTTP %d", err.StatusCode)
}

// GetTaskFormModel 按任务解析 Form Engine 模型，避免按 formKey 查询最新版本而误用其他部署或租户的表单。
// 运行时接口返回 404 时才查询历史任务；仅官方明确的未配置表单响应返回空模型。
// 本方法不检查业务用户权限，调用方必须先完成任务读取鉴权。
func (client *RESTClient) GetTaskFormModel(ctx context.Context, taskID string) (map[string]interface{}, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || taskID == "." || taskID == ".." {
		return nil, fmt.Errorf("任务 ID 无效")
	}
	ctx, cancel := context.WithTimeout(ctx, client.httpClient.Timeout)
	defer cancel()
	httpClient := *client.httpClient
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	paths := []string{
		"/runtime/tasks/" + url.PathEscape(taskID) + "/form",
		"/history/historic-task-instances/" + url.PathEscape(taskID) + "/form",
	}
	for index, path := range paths {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+path, nil)
		if err != nil {
			return nil, fmt.Errorf("创建任务表单查询失败")
		}
		request.Header.Set("Accept", "application/json")
		if client.username != "" {
			request.SetBasicAuth(client.username, client.password)
		}
		response, err := httpClient.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("任务表单查询网络失败")
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
		response.Body.Close()
		if readErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("任务表单响应读取失败")
		}
		if len(body) > 4<<20 {
			return nil, fmt.Errorf("任务表单响应超过大小限制")
		}
		if response.StatusCode == http.StatusNotFound && index == 0 {
			continue
		}
		if response.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		if response.StatusCode == http.StatusBadRequest {
			var failure struct {
				Message   string `json:"message"`
				Exception string `json:"exception"`
			}
			if json.Unmarshal(body, &failure) == nil && failure.Message == "Bad request" && failure.Exception == "Task has no form defined" {
				return nil, nil
			}
		}
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("任务表单查询失败，HTTP %d", response.StatusCode)
		}
		var model map[string]interface{}
		if json.Unmarshal(body, &model) != nil || model == nil {
			return nil, fmt.Errorf("任务表单模型不是有效 JSON 对象")
		}
		key, validKey := model["key"].(string)
		_, validFields := model["fields"].([]interface{})
		if !validKey || strings.TrimSpace(key) == "" || !validFields {
			return nil, fmt.Errorf("任务表单模型缺少 key 或 fields 数组")
		}
		return model, nil
	}
	return nil, nil
}
