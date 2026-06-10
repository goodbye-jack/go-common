package flowable

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goodbye-jack/go-common/workflow/types"
)

func TestNormalizeActionTimelineResponseFromLegacyPayload(t *testing.T) {
	var response types.ProcessActionTimelineResponse
	if err := json.Unmarshal([]byte(`{"summary":{"processInstanceId":"process-001"}}`), &response); err != nil {
		t.Fatalf("unmarshal legacy payload failed: %v", err)
	}
	if response.Items != nil {
		t.Fatalf("expected legacy payload items to be nil before normalize")
	}

	normalizeActionTimelineResponse(&response)

	if response.Items == nil {
		t.Fatalf("expected items to be initialized after normalize")
	}
	if len(response.Items) != 0 {
		t.Fatalf("expected empty items after normalize, got %d", len(response.Items))
	}

	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal normalized response failed: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, `"items":[]`) {
		t.Fatalf("expected marshaled payload to contain empty items array, got %s", text)
	}
}

func TestNormalizeActionTimelineResponseHandlesNilResponse(t *testing.T) {
	normalizeActionTimelineResponse(nil)
}

func TestBuildActionTimelineItemsDeduplicatesLegacyTransferComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/runtime/tasks/task-1/comments":
			_, _ = w.Write([]byte(`[
				{"id":"legacy-transfer","author":"rest-admin","message":"[TRANSFER] from=test1 to=test3 reason=转办给 test3","time":"2026-06-10T05:00:49.604Z","taskId":"task-1"},
				{"id":"workflow-transfer","author":"rest-admin","message":"[WF_ACTION]{\"v\":1,\"action\":\"TRANSFER\",\"operatorUserId\":\"test1\",\"operatorUserName\":\"Test User 1\",\"processInstanceId\":\"proc-1\",\"taskId\":\"task-1\",\"taskName\":\"最终确认完成\",\"activityId\":\"task_action_finish\",\"activityName\":\"最终确认完成\",\"fromAssignee\":\"test1\",\"toAssignee\":\"test3\",\"reason\":\"转办给 test3\"}","time":"2026-06-10T05:00:49.861Z","taskId":"task-1"}
			]`))
		case "/runtime/tasks/task-2/comments":
			_, _ = w.Write([]byte(`[
				{"id":"claim-001","author":"rest-admin","message":"[WF_ACTION]{\"v\":1,\"action\":\"CLAIM\",\"operatorUserId\":\"test1\",\"operatorUserName\":\"Test User 1\",\"processInstanceId\":\"proc-1\",\"taskId\":\"task-2\",\"taskName\":\"候选任务分派测试\",\"activityId\":\"task_action_dispatch\",\"activityName\":\"候选任务分派测试\",\"toAssignee\":\"test1\"}","time":"2026-06-10T04:59:55.565Z","taskId":"task-2"}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewRESTClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewRESTClient() error=%v", err)
	}

	items := client.buildActionTimelineItems(context.Background(), "proc-1", map[string]taskActionMetadata{
		"task-1": {
			ProcessInstanceID: "proc-1",
			TaskID:            "task-1",
			TaskName:          "最终确认完成",
			ActivityID:        "task_action_finish",
			ActivityName:      "最终确认完成",
		},
		"task-2": {
			ProcessInstanceID: "proc-1",
			TaskID:            "task-2",
			TaskName:          "候选任务分派测试",
			ActivityID:        "task_action_dispatch",
			ActivityName:      "候选任务分派测试",
		},
	})

	if len(items) != 2 {
		t.Fatalf("buildActionTimelineItems() len=%d, want 2", len(items))
	}
	if items[0].ActionType != types.TaskActionTypeClaim {
		t.Fatalf("items[0].ActionType=%q, want %q", items[0].ActionType, types.TaskActionTypeClaim)
	}
	if items[1].ActionType != types.TaskActionTypeTransfer {
		t.Fatalf("items[1].ActionType=%q, want %q", items[1].ActionType, types.TaskActionTypeTransfer)
	}
	if items[1].CommentID != "workflow-transfer" {
		t.Fatalf("items[1].CommentID=%q, want workflow-transfer", items[1].CommentID)
	}
	if items[1].FromAssignee != "test1" || items[1].ToAssignee != "test3" {
		t.Fatalf("unexpected transfer transition: from=%q to=%q", items[1].FromAssignee, items[1].ToAssignee)
	}
}
