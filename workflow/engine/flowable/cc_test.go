package flowable

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/goodbye-jack/go-common/orm"
	"github.com/goodbye-jack/go-common/utils"
	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/types"
	"github.com/spf13/viper"
)

func TestCCValuesFromVariables(t *testing.T) {
	originalUsersKey := viper.Get(configCCVariableUsers)
	originalGroupsKey := viper.Get(configCCVariableGroups)
	t.Cleanup(func() {
		viper.Set(configCCVariableUsers, originalUsersKey)
		viper.Set(configCCVariableGroups, originalGroupsKey)
	})

	viper.Set(configCCVariableUsers, "")
	viper.Set(configCCVariableGroups, "")

	users, groups := ccValuesFromVariables(map[string]interface{}{
		defaultCCVariableUsers:  []interface{}{"pqtest", " admin ", "pqtest"},
		defaultCCVariableGroups: "wfgrp:general_tenant:city-manager， wfgrp:general_tenant:area-owner",
	})
	if want := []string{"pqtest", "admin"}; !reflect.DeepEqual(users, want) {
		t.Fatalf("users=%v, want %v", users, want)
	}
	if want := []string{"wfgrp:general_tenant:city-manager", "wfgrp:general_tenant:area-owner"}; !reflect.DeepEqual(groups, want) {
		t.Fatalf("groups=%v, want %v", groups, want)
	}
}

func TestCCValuesFromVariablesWithCustomKeys(t *testing.T) {
	originalUsersKey := viper.Get(configCCVariableUsers)
	originalGroupsKey := viper.Get(configCCVariableGroups)
	t.Cleanup(func() {
		viper.Set(configCCVariableUsers, originalUsersKey)
		viper.Set(configCCVariableGroups, originalGroupsKey)
	})

	viper.Set(configCCVariableUsers, "customCcUsers")
	viper.Set(configCCVariableGroups, "customCcGroups")

	users, groups := ccValuesFromVariables(map[string]interface{}{
		"customCcUsers":  "u1;u2；u1",
		"customCcGroups": []string{"g1", "g2"},
	})
	if want := []string{"u1", "u2"}; !reflect.DeepEqual(users, want) {
		t.Fatalf("users=%v, want %v", users, want)
	}
	if want := []string{"g1", "g2"}; !reflect.DeepEqual(groups, want) {
		t.Fatalf("groups=%v, want %v", groups, want)
	}
}

func TestTaskCCLifecycleWithFlowableMockAndSQLite(t *testing.T) {
	const dbInstance = "workflow_cc_lifecycle_test"
	originalDB := orm.DB
	originalInstance := orm.RelationalMap[dbInstance]
	originalCCDB := viper.Get(configCCDBInstance)
	originalEnabled := viper.Get(configCCEnabled)
	originalNotify := viper.Get(configCCNotifyEnabled)
	t.Cleanup(func() {
		orm.DB = originalDB
		if originalInstance != nil {
			orm.RelationalMap[dbInstance] = originalInstance
		} else {
			delete(orm.RelationalMap, dbInstance)
		}
		viper.Set(configCCDBInstance, originalCCDB)
		viper.Set(configCCEnabled, originalEnabled)
		viper.Set(configCCNotifyEnabled, originalNotify)
		ccMigrateMu.Lock()
		delete(ccMigrated, dbInstance)
		ccMigrateMu.Unlock()
	})

	testDB := orm.NewOrm(filepath.Join(t.TempDir(), "workflow_cc_lifecycle_test.sqlite"), utils.DBTypeSQLite, 1)
	if testDB == nil || testDB.GetDB() == nil {
		t.Fatalf("NewOrm() returned nil")
	}
	orm.RelationalMap[dbInstance] = testDB
	viper.Set(configCCDBInstance, dbInstance)
	viper.Set(configCCEnabled, true)
	viper.Set(configCCNotifyEnabled, false)

	links := make([]map[string]string, 0)
	createLinkCount := 0
	deleteLinkCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/runtime/tasks/task-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":                  "task-1",
				"name":                "信息审核",
				"taskDefinitionKey":   "ownerReview",
				"processInstanceId":   "proc-1",
				"processDefinitionId": "training_score_approval_process:1:proc-def-1",
				"tenantId":            "general_tenant",
				"businessKey":         "training_score_review:general_tenant:8",
				"processVariables": []map[string]interface{}{
					{"name": "tenantId", "value": "general_tenant"},
					{"name": "systemCode", "value": "relics-protect-backend"},
					{"name": "bizId", "value": "training_score_review:general_tenant:8"},
					{"name": "bizType", "value": "training_score_review"},
					{"name": "title", "value": "测试数据 - 联调志愿者康巴什区01"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/runtime/tasks/task-1/identitylinks":
			data := make([]map[string]string, 0, len(links))
			data = append(data, links...)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
		case r.Method == http.MethodPost && r.URL.Path == "/runtime/tasks/task-1/identitylinks":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode identity link payload error=%v", err)
			}
			links = append(links, payload)
			createLinkCount++
			_ = json.NewEncoder(w).Encode(payload)
		case r.Method == http.MethodDelete && r.URL.Path == "/runtime/tasks/task-1/identitylinks/users/pqtest/participant":
			filtered := make([]map[string]string, 0, len(links))
			for _, link := range links {
				if link["user"] == "pqtest" && link["type"] == "participant" {
					continue
				}
				filtered = append(filtered, link)
			}
			links = filtered
			deleteLinkCount++
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewRESTClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewRESTClient() error=%v", err)
	}
	ctx := context.Background()
	operator := &workflowcontext.UserContext{UserID: "operator", UserName: "操作人", TenantID: "general_tenant", SystemCode: "relics-protect-backend"}
	ccUser := &workflowcontext.UserContext{
		UserID:     "pqtest",
		TenantID:   "general_tenant",
		SystemCode: "relics-protect-backend",
		Groups:     []string{"wfgrp:general_tenant:test-code-2"},
	}

	response, err := client.AddTaskCC(ctx, "task-1", &types.TaskCCRequest{
		Users:  []string{"pqtest", "pqtest"},
		Groups: []string{"wfgrp:general_tenant:test-code-2"},
		Notify: true,
		Reason: "请知悉",
	}, operator)
	if err != nil {
		t.Fatalf("AddTaskCC() error=%v", err)
	}
	if len(response.Users) != 1 || len(response.Groups) != 1 {
		t.Fatalf("AddTaskCC() response=%+v, want one user and one group", response)
	}
	if createLinkCount != 2 {
		t.Fatalf("identity link create count=%d, want 2", createLinkCount)
	}

	page, err := client.ListCC(ctx, ccUser, &types.TaskQuery{Start: 0, Size: 20})
	if err != nil {
		t.Fatalf("ListCC() error=%v", err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("ListCC() total=%d len=%d, want 2", page.Total, len(page.Items))
	}
	if page.Items[0].BizType != "training_score_review" || page.Items[0].Title == "" {
		t.Fatalf("ListCC() item missing business metadata: %+v", page.Items[0])
	}

	count, err := client.CountCC(ctx, ccUser, &types.TaskQuery{})
	if err != nil {
		t.Fatalf("CountCC() error=%v", err)
	}
	if count.Total != 2 || count.Unread != 2 || count.Active != 2 {
		t.Fatalf("CountCC()=%+v, want total=2 unread=2 active=2", count)
	}

	readResp, err := client.MarkCCRead(ctx, ccUser, &types.TaskCCReadRequest{CCIDs: []uint{page.Items[0].CCID}})
	if err != nil {
		t.Fatalf("MarkCCRead() error=%v", err)
	}
	if readResp.Updated != 1 {
		t.Fatalf("MarkCCRead().Updated=%d, want 1", readResp.Updated)
	}
	count, err = client.CountCC(ctx, ccUser, &types.TaskQuery{})
	if err != nil {
		t.Fatalf("CountCC() after read error=%v", err)
	}
	if count.Unread != 1 {
		t.Fatalf("CountCC().Unread=%d, want 1", count.Unread)
	}

	readAllResp, err := client.MarkAllCCRead(ctx, ccUser, &types.TaskQuery{})
	if err != nil {
		t.Fatalf("MarkAllCCRead() error=%v", err)
	}
	if readAllResp.Updated != 1 {
		t.Fatalf("MarkAllCCRead().Updated=%d, want 1", readAllResp.Updated)
	}
	count, err = client.CountCC(ctx, ccUser, &types.TaskQuery{})
	if err != nil {
		t.Fatalf("CountCC() after read-all error=%v", err)
	}
	if count.Unread != 0 {
		t.Fatalf("CountCC().Unread=%d, want 0", count.Unread)
	}

	if _, err := client.RevokeTaskCCUser(ctx, "task-1", "pqtest", operator); err != nil {
		t.Fatalf("RevokeTaskCCUser() error=%v", err)
	}
	if deleteLinkCount != 1 {
		t.Fatalf("identity link delete count=%d, want 1", deleteLinkCount)
	}
	count, err = client.CountCC(ctx, ccUser, &types.TaskQuery{})
	if err != nil {
		t.Fatalf("CountCC() after revoke error=%v", err)
	}
	if count.Active != 1 || count.Revoked != 1 {
		t.Fatalf("CountCC() after revoke=%+v, want active=1 revoked=1", count)
	}
}

func TestUpdateCCRecordsByTaskIDMarksCompleted(t *testing.T) {
	const dbInstance = "workflow_cc_status_test"
	originalInstance := orm.RelationalMap[dbInstance]
	originalCCDB := viper.Get(configCCDBInstance)
	t.Cleanup(func() {
		if originalInstance != nil {
			orm.RelationalMap[dbInstance] = originalInstance
		} else {
			delete(orm.RelationalMap, dbInstance)
		}
		viper.Set(configCCDBInstance, originalCCDB)
		ccMigrateMu.Lock()
		delete(ccMigrated, dbInstance)
		ccMigrateMu.Unlock()
	})

	testDB := orm.NewOrm(filepath.Join(t.TempDir(), "workflow_cc_status_test.sqlite"), utils.DBTypeSQLite, 1)
	if testDB == nil || testDB.GetDB() == nil {
		t.Fatalf("NewOrm() returned nil")
	}
	orm.RelationalMap[dbInstance] = testDB
	viper.Set(configCCDBInstance, dbInstance)
	if err := ensureCCTables(); err != nil {
		t.Fatalf("ensureCCTables() error=%v", err)
	}
	records := []workflowCCRecordModel{
		{TaskID: "task-complete", ProcessInstanceID: "proc-1", RelationStatus: types.TaskCCRelationStatusActive, TaskStatus: types.TaskCCTaskStatusRunning},
		{TaskID: "task-other", ProcessInstanceID: "proc-1", RelationStatus: types.TaskCCRelationStatusActive, TaskStatus: types.TaskCCTaskStatusRunning},
	}
	if err := testDB.GetDB().Create(&records).Error; err != nil {
		t.Fatalf("create cc records error=%v", err)
	}

	(&RESTClient{}).updateCCRecordsByTaskID(context.Background(), "task-complete", types.TaskCCTaskStatusCompleted)

	var updated, untouched workflowCCRecordModel
	if err := testDB.GetDB().Where("task_id = ?", "task-complete").First(&updated).Error; err != nil {
		t.Fatalf("load updated record error=%v", err)
	}
	if err := testDB.GetDB().Where("task_id = ?", "task-other").First(&untouched).Error; err != nil {
		t.Fatalf("load untouched record error=%v", err)
	}
	if updated.TaskStatus != types.TaskCCTaskStatusCompleted {
		t.Fatalf("updated task status=%q, want %q", updated.TaskStatus, types.TaskCCTaskStatusCompleted)
	}
	if untouched.TaskStatus != types.TaskCCTaskStatusRunning {
		t.Fatalf("untouched task status=%q, want %q", untouched.TaskStatus, types.TaskCCTaskStatusRunning)
	}
}

func BenchmarkListCCSQLite(b *testing.B) {
	const dbInstance = "workflow_cc_benchmark_test"
	originalDB := orm.DB
	originalInstance := orm.RelationalMap[dbInstance]
	originalCCDB := viper.Get(configCCDBInstance)
	originalEnabled := viper.Get(configCCEnabled)
	b.Cleanup(func() {
		orm.DB = originalDB
		if originalInstance != nil {
			orm.RelationalMap[dbInstance] = originalInstance
		} else {
			delete(orm.RelationalMap, dbInstance)
		}
		viper.Set(configCCDBInstance, originalCCDB)
		viper.Set(configCCEnabled, originalEnabled)
		ccMigrateMu.Lock()
		delete(ccMigrated, dbInstance)
		ccMigrateMu.Unlock()
	})

	testDB := orm.NewOrm(filepath.Join(b.TempDir(), "workflow_cc_benchmark_test.sqlite"), utils.DBTypeSQLite, 1)
	if testDB == nil || testDB.GetDB() == nil {
		b.Fatalf("NewOrm() returned nil")
	}
	orm.RelationalMap[dbInstance] = testDB
	viper.Set(configCCDBInstance, dbInstance)
	viper.Set(configCCEnabled, true)
	if err := ensureCCTables(); err != nil {
		b.Fatalf("ensureCCTables() error=%v", err)
	}

	rows := make([]workflowCCRecordModel, 0, 1000)
	for i := 0; i < 1000; i++ {
		rows = append(rows, workflowCCRecordModel{
			TenantID:             "general_tenant",
			SystemCode:           "relics-protect-backend",
			TaskID:               fmt.Sprintf("task-%04d", i),
			ProcessInstanceID:    fmt.Sprintf("proc-%04d", i),
			ProcessDefinitionKey: "training_score_approval_process",
			ActivityID:           "ownerReview",
			BizID:                fmt.Sprintf("training_score_review:general_tenant:%d", i),
			BizType:              "training_score_review",
			Title:                fmt.Sprintf("测试数据-%04d", i),
			IdentityType:         types.TaskCCIdentityTypeUser,
			IdentityID:           "pqtest",
			LinkType:             defaultCCIdentityLinkType,
			RelationStatus:       types.TaskCCRelationStatusActive,
			TaskStatus:           types.TaskCCTaskStatusRunning,
			NotifyStatus:         types.TaskCCNotifyStatusSkipped,
			Source:               types.TaskCCSourceManual,
		})
	}
	if err := testDB.GetDB().Create(&rows).Error; err != nil {
		b.Fatalf("seed workflow_cc_records error=%v", err)
	}

	client := &RESTClient{}
	user := &workflowcontext.UserContext{UserID: "pqtest", TenantID: "general_tenant", SystemCode: "relics-protect-backend"}
	query := &types.TaskQuery{Start: 0, Size: 100, BizType: "training_score_review"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page, err := client.ListCC(context.Background(), user, query)
		if err != nil {
			b.Fatalf("ListCC() error=%v", err)
		}
		if page.Total != 1000 || len(page.Items) != 100 {
			b.Fatalf("ListCC() total=%d len=%d, want total=1000 len=100", page.Total, len(page.Items))
		}
	}
}
