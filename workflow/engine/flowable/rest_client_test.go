package flowable

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/types"
)

func TestMergeOptionalNeedExpert(t *testing.T) {
	tests := []struct {
		name     string
		req      *types.CompleteTaskRequest
		target   map[string]interface{}
		expected interface{}
		exists   bool
	}{
		{
			name:     "skip when request omitted",
			req:      &types.CompleteTaskRequest{},
			target:   map[string]interface{}{},
			expected: nil,
			exists:   false,
		},
		{
			name: "keep explicit false from variables",
			req: &types.CompleteTaskRequest{
				Variables: map[string]interface{}{
					"needExpert": false,
				},
			},
			target: map[string]interface{}{
				"needExpert": false,
			},
			expected: false,
			exists:   true,
		},
		{
			name: "variables win over top level true",
			req: &types.CompleteTaskRequest{
				NeedExpert: true,
				Variables: map[string]interface{}{
					"needExpert": false,
				},
			},
			target: map[string]interface{}{
				"needExpert": false,
			},
			expected: false,
			exists:   true,
		},
		{
			name: "top level true fills missing variable",
			req: &types.CompleteTaskRequest{
				NeedExpert: true,
			},
			target:   map[string]interface{}{},
			expected: true,
			exists:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeOptionalNeedExpert(tt.target, tt.req)
			value, ok := tt.target["needExpert"]
			if ok != tt.exists {
				t.Fatalf("needExpert exists=%v, want %v", ok, tt.exists)
			}
			if !tt.exists {
				return
			}
			if value != tt.expected {
				t.Fatalf("needExpert=%v, want %v", value, tt.expected)
			}
		})
	}
}

func TestShouldRetryFlowableDeadlock(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "deadlock message",
			err:  assertError("POST /runtime/tasks/1 failed: status=500 body={\"exception\":\"Deadlock found when trying to get lock; try restarting transaction\"}"),
			want: true,
		},
		{
			name: "other error",
			err:  assertError("POST /runtime/tasks/1 failed: status=500 body={\"exception\":\"Unknown property used in expression\"}"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetryFlowableDeadlock(tt.err); got != tt.want {
				t.Fatalf("shouldRetryFlowableDeadlock()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildTransferComment(t *testing.T) {
	tests := []struct {
		name   string
		from   string
		to     string
		reason string
		want   string
	}{
		{
			name:   "with reason",
			from:   "test1",
			to:     "test2",
			reason: "当前事项改由 test2 继续处理",
			want:   "[TRANSFER] from=test1 to=test2 reason=当前事项改由 test2 继续处理",
		},
		{
			name: "without reason",
			from: "test1",
			to:   "test2",
			want: "[TRANSFER] from=test1 to=test2",
		},
		{
			name: "missing target",
			from: "test1",
			to:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTransferComment(tt.from, tt.to, tt.reason)
			if got != tt.want {
				t.Fatalf("buildTransferComment()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildWorkflowActionCommentAndParse(t *testing.T) {
	comment := buildWorkflowActionComment(
		types.TaskActionTypeDelegate,
		runtimeTaskRecord{
			ID:                "task-1",
			Name:              "市局管理员预警查看",
			TaskDefinitionKey: "task_bureau_view",
			Assignee:          "test1",
			ProcessInstanceID: "proc-1",
		},
		runtimeTaskRecord{
			ID:                "task-1",
			Name:              "市局管理员预警查看",
			TaskDefinitionKey: "task_bureau_view",
			Assignee:          "test2",
			Owner:             "test1",
			ProcessInstanceID: "proc-1",
		},
		nil,
		"临时协助处理",
	)
	if comment == "" {
		t.Fatalf("buildWorkflowActionComment() returned empty comment")
	}
	payload, ok := parseWorkflowActionComment(comment)
	if !ok {
		t.Fatalf("parseWorkflowActionComment() ok=false")
	}
	if payload.Action != types.TaskActionTypeDelegate {
		t.Fatalf("payload.Action=%q, want %q", payload.Action, types.TaskActionTypeDelegate)
	}
	if payload.FromAssignee != "test1" || payload.ToAssignee != "test2" {
		t.Fatalf("unexpected assignee transition: from=%q to=%q", payload.FromAssignee, payload.ToAssignee)
	}
	if payload.ToOwner != "test1" {
		t.Fatalf("payload.ToOwner=%q, want test1", payload.ToOwner)
	}
	if payload.Reason != "临时协助处理" {
		t.Fatalf("payload.Reason=%q, want 临时协助处理", payload.Reason)
	}
}

func TestParseLegacyTransferComment(t *testing.T) {
	payload, ok := parseLegacyTransferComment("[TRANSFER] from=test1 to=test2 reason=当前事项改由 test2 继续处理")
	if !ok {
		t.Fatalf("parseLegacyTransferComment() ok=false")
	}
	if payload.Action != types.TaskActionTypeTransfer {
		t.Fatalf("payload.Action=%q, want %q", payload.Action, types.TaskActionTypeTransfer)
	}
	if payload.FromAssignee != "test1" || payload.ToAssignee != "test2" {
		t.Fatalf("unexpected transfer payload: from=%q to=%q", payload.FromAssignee, payload.ToAssignee)
	}
	if payload.Reason != "当前事项改由 test2 继续处理" {
		t.Fatalf("payload.Reason=%q, want 当前事项改由 test2 继续处理", payload.Reason)
	}
}

func TestParseHistoricComments(t *testing.T) {
	body := []byte(`{"data":[{"id":"c1","processInstanceId":"proc-1","taskId":"task-1","userId":"test1","time":"2026-06-09T10:00:00.000Z","message":"[WF_ACTION]{\"v\":1,\"action\":\"CLAIM\",\"taskId\":\"task-1\"}"}]}`)
	comments, err := parseHistoricComments(body)
	if err != nil {
		t.Fatalf("parseHistoricComments() error=%v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("parseHistoricComments() len=%d, want 1", len(comments))
	}
	if comments[0].ID != "c1" || comments[0].TaskID != "task-1" {
		t.Fatalf("unexpected comment record: %+v", comments[0])
	}
}

func TestParseHistoricCommentsSupportsBareArrayAndAuthor(t *testing.T) {
	body := []byte(`[{"id":"c2","taskId":"task-2","author":"rest-admin","time":"2026-06-10T05:00:49.861Z","message":"[WF_ACTION]{\"v\":1,\"action\":\"TRANSFER\",\"taskId\":\"task-2\"}"}]`)
	comments, err := parseHistoricComments(body)
	if err != nil {
		t.Fatalf("parseHistoricComments() error=%v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("parseHistoricComments() len=%d, want 1", len(comments))
	}
	if comments[0].UserID != "rest-admin" {
		t.Fatalf("comments[0].UserID=%q, want rest-admin", comments[0].UserID)
	}
	if comments[0].TaskID != "task-2" {
		t.Fatalf("comments[0].TaskID=%q, want task-2", comments[0].TaskID)
	}
}

func TestUnclaimTaskClearsAssigneeViaPut(t *testing.T) {
	var updatePayload map[string]interface{}
	getCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/runtime/tasks/task-1":
			getCount++
			w.Header().Set("Content-Type", "application/json")
			if getCount == 1 {
				_, _ = w.Write([]byte(`{"id":"task-1","processInstanceId":"proc-1","name":"任务一","taskDefinitionKey":"task_action_finish","assignee":"test1","delegationState":"","processDefinitionId":"proc-def-1"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"task-1","processInstanceId":"proc-1","name":"任务一","taskDefinitionKey":"task_action_finish","assignee":"","delegationState":"","processDefinitionId":"proc-def-1"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/runtime/tasks/task-1":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll() error=%v", err)
			}
			if err := json.Unmarshal(body, &updatePayload); err != nil {
				t.Fatalf("Unmarshal update payload error=%v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"task-1","processInstanceId":"proc-1","name":"任务一","taskDefinitionKey":"task_action_finish","assignee":"","delegationState":"","processDefinitionId":"proc-def-1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/runtime/tasks/task-1/comments":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"comment-1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewRESTClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewRESTClient() error=%v", err)
	}
	response, err := client.UnclaimTask(context.Background(), "task-1", &workflowcontext.UserContext{UserID: "test1"})
	if err != nil {
		t.Fatalf("UnclaimTask() error=%v", err)
	}
	if response == nil {
		t.Fatalf("UnclaimTask() response=nil")
	}
	if response.Status != "unclaimed" {
		t.Fatalf("response.Status=%q, want unclaimed", response.Status)
	}
	if response.Assignee != "" {
		t.Fatalf("response.Assignee=%q, want empty", response.Assignee)
	}
	if value, exists := updatePayload["assignee"]; !exists || value != nil {
		t.Fatalf("update assignee=%v, want null", updatePayload["assignee"])
	}
}

func TestGetProgressViewResolvesProcessDefinitionKeyFromMetadata(t *testing.T) {
	const definitionID = "4eb66431-8442-11f1-b2ee-82acb04b355b"
	const definitionKey = "training_score_approval_process"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/runtime/process-instances/proc-1":
			_, _ = w.Write([]byte(`{"id":"proc-1","processDefinitionId":"` + definitionID + `","businessKey":"training_score_review:general_tenant:4","tenantId":"","startTime":"2026-07-21T09:20:49.377Z"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repository/process-definitions/"+definitionID:
			_, _ = w.Write([]byte(`{"id":"` + definitionID + `","key":"` + definitionKey + `","version":4,"tenantId":""}`))
		case r.Method == http.MethodGet && r.URL.Path == "/runtime/process-instances/proc-1/variables":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/history/historic-variable-instances":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/runtime/tasks":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/history/historic-task-instances":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/history/historic-activity-instances":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repository/process-definitions/"+definitionID+"/resourcedata":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn" targetNamespace="http://www.flowable.org/processdef">
  <process id="training_score_approval_process" name="培训考核分数审批流程" isExecutable="true">
    <startEvent id="startEvent1" name="开始"/>
    <sequenceFlow id="flow1" sourceRef="startEvent1" targetRef="ownerReview"/>
    <userTask id="ownerReview" name="信息审核" flowable:candidateGroups="${nextCandidateGroups}"/>
    <sequenceFlow id="flow2" sourceRef="ownerReview" targetRef="endEvent1"/>
    <endEvent id="endEvent1" name="结束"/>
  </process>
</definitions>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewRESTClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewRESTClient() error=%v", err)
	}
	view, err := client.GetProgressView(context.Background(), "proc-1", nil)
	if err != nil {
		t.Fatalf("GetProgressView() error=%v", err)
	}
	if view == nil {
		t.Fatalf("GetProgressView() returned nil")
	}
	if view.Summary.ProcessDefinitionKey != definitionKey {
		t.Fatalf("ProcessDefinitionKey=%q, want %q", view.Summary.ProcessDefinitionKey, definitionKey)
	}
	if view.Summary.ProcessDefinitionID != definitionID {
		t.Fatalf("ProcessDefinitionID=%q, want %q", view.Summary.ProcessDefinitionID, definitionID)
	}
}

func TestNormalizeTaskRecordDefinitionKeys(t *testing.T) {
	records := []types.WorkflowTaskRecordItem{
		{
			ProcessDefinitionID:  "definition-1",
			ProcessDefinitionKey: "definition-1",
		},
		{
			ProcessDefinitionID:  "definition-2",
			ProcessDefinitionKey: "definition-2",
		},
	}

	normalized := normalizeTaskRecordDefinitionKeys(records, "definition-1", "training_score_approval_process")
	if normalized[0].ProcessDefinitionKey != "training_score_approval_process" {
		t.Fatalf("normalized[0].ProcessDefinitionKey=%q", normalized[0].ProcessDefinitionKey)
	}
	if normalized[1].ProcessDefinitionKey != "definition-2" {
		t.Fatalf("normalized[1].ProcessDefinitionKey=%q", normalized[1].ProcessDefinitionKey)
	}
	if records[0].ProcessDefinitionKey != "definition-1" {
		t.Fatalf("original record was mutated: %q", records[0].ProcessDefinitionKey)
	}
}

type assertError string

func (e assertError) Error() string {
	return string(e)
}
