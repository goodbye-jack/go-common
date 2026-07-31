package flowable

import (
	stdcontext "context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/log"
	commonmodel "github.com/goodbye-jack/go-common/model"
	"github.com/goodbye-jack/go-common/orm"
	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/types"
	"gorm.io/gorm"
)

const (
	configCCEnabled           = "workflow.cc.enabled"
	configCCIdentityLinkType  = "workflow.cc.identity_link_type"
	configCCVariableUsers     = "workflow.cc.variable_keys.users"
	configCCVariableGroups    = "workflow.cc.variable_keys.groups"
	configCCNotifyEnabled     = "workflow.cc.notify_enabled"
	configCCIndexEnabled      = "workflow.cc.index_enabled"
	configCCDBInstance        = "workflow.cc.db_instance"
	defaultCCIdentityLinkType = "participant"
	defaultCCVariableUsers    = "nextCcUsers"
	defaultCCVariableGroups   = "nextCcGroups"
)

var (
	ccMigrateMu sync.Mutex
	ccMigrated  = map[string]bool{}
)

type workflowCCRecordModel struct {
	commonmodel.ModelBase
	TenantID             string     `gorm:"size:128;index:idx_wcc_tenant_identity;index:idx_wcc_scope_identity,priority:1;index:idx_wcc_biz_filter,priority:1" json:"tenantId"`
	SystemCode           string     `gorm:"size:128;index;index:idx_wcc_scope_identity,priority:2;index:idx_wcc_biz_filter,priority:2" json:"systemCode"`
	TaskID               string     `gorm:"size:64;index;index:idx_wcc_task_identity,priority:1;index:idx_wcc_task_status,priority:1" json:"taskId"`
	ProcessInstanceID    string     `gorm:"size:64;index;index:idx_wcc_process_status,priority:1" json:"processInstanceId"`
	ProcessDefinitionID  string     `gorm:"size:128;index" json:"processDefinitionId"`
	ProcessDefinitionKey string     `gorm:"size:128;index;index:idx_wcc_biz_filter,priority:4" json:"processDefinitionKey"`
	ActivityID           string     `gorm:"size:128;index;index:idx_wcc_biz_filter,priority:5" json:"activityId"`
	ActivityName         string     `gorm:"size:255" json:"activityName"`
	BizID                string     `gorm:"size:128;index" json:"bizId"`
	BizType              string     `gorm:"size:128;index;index:idx_wcc_biz_filter,priority:3" json:"bizType"`
	Title                string     `gorm:"size:255" json:"title"`
	IdentityType         string     `gorm:"size:16;index:idx_wcc_tenant_identity;index:idx_wcc_scope_identity,priority:3;index:idx_wcc_task_identity,priority:2" json:"identityType"`
	IdentityID           string     `gorm:"size:255;index:idx_wcc_tenant_identity;index:idx_wcc_scope_identity,priority:4;index:idx_wcc_task_identity,priority:3" json:"identityId"`
	LinkType             string     `gorm:"size:64;index" json:"linkType"`
	RelationStatus       string     `gorm:"size:32;index:idx_wcc_tenant_identity;index:idx_wcc_scope_identity,priority:5;index:idx_wcc_task_identity,priority:4;index:idx_wcc_biz_filter,priority:6" json:"relationStatus"`
	TaskStatus           string     `gorm:"size:32;index;index:idx_wcc_scope_identity,priority:6;index:idx_wcc_task_status,priority:2;index:idx_wcc_process_status,priority:2" json:"taskStatus"`
	NotifyStatus         string     `gorm:"size:32;index" json:"notifyStatus"`
	Source               string     `gorm:"size:64;index" json:"source"`
	SourceEventKey       string     `gorm:"size:255;index" json:"sourceEventKey"`
	Reason               string     `gorm:"type:text" json:"reason"`
	CreatedBy            string     `gorm:"size:128;index" json:"createdBy"`
	CreatedByName        string     `gorm:"size:255" json:"createdByName"`
	RevokedBy            string     `gorm:"size:128" json:"revokedBy"`
	RevokedAt            *time.Time `gorm:"index" json:"revokedAt"`
}

func (workflowCCRecordModel) TableName() string { return "workflow_cc_records" }

type workflowCCReadStateModel struct {
	commonmodel.ModelBase
	TenantID      string    `gorm:"size:128;index:idx_wcc_read_user" json:"tenantId"`
	CCRecordID    uint      `gorm:"index;uniqueIndex:uniq_wcc_read_user;index:idx_wcc_read_join,priority:2" json:"ccRecordId"`
	UserID        string    `gorm:"size:128;index:idx_wcc_read_user;uniqueIndex:uniq_wcc_read_user;index:idx_wcc_read_join,priority:1" json:"userId"`
	ReadStatus    string    `gorm:"size:32;index;index:idx_wcc_read_join,priority:3" json:"readStatus"`
	ReadAt        time.Time `gorm:"index" json:"readAt"`
	LastTaskID    string    `gorm:"size:64;index" json:"lastTaskId"`
	LastIdentity  string    `gorm:"size:255" json:"lastIdentity"`
	LastReadScope string    `gorm:"size:32" json:"lastReadScope"`
}

func (workflowCCReadStateModel) TableName() string { return "workflow_cc_read_states" }

func ccDB() *orm.Orm {
	instanceName := strings.TrimSpace(config.GetConfigString(configCCDBInstance))
	if instanceName == "" {
		instanceName = strings.TrimSpace(config.GetConfigString(configTaskRecordsDBInstance))
	}
	if instanceName != "" {
		return orm.GetDB(instanceName)
	}
	return orm.DB
}

func ensureCCTables() error {
	db := ccDB()
	if db == nil || db.GetDB() == nil {
		return nil
	}
	key := strings.TrimSpace(config.GetConfigString(configCCDBInstance))
	if key == "" {
		key = strings.TrimSpace(config.GetConfigString(configTaskRecordsDBInstance))
	}
	if key == "" {
		key = "default"
	}
	ccMigrateMu.Lock()
	defer ccMigrateMu.Unlock()
	if ccMigrated[key] {
		return nil
	}
	if err := db.GetDB().AutoMigrate(&workflowCCRecordModel{}, &workflowCCReadStateModel{}); err != nil {
		return err
	}
	ccMigrated[key] = true
	return nil
}

func ccEnabled() bool {
	value := strings.TrimSpace(config.GetConfigString(configCCEnabled))
	if value == "" {
		return true
	}
	return config.GetConfigBool(configCCEnabled)
}

func ccLinkType(value string) string {
	return firstNonBlank(strings.TrimSpace(value), strings.TrimSpace(config.GetConfigString(configCCIdentityLinkType)), defaultCCIdentityLinkType)
}

func ccNotifyStatus(notify bool) string {
	if notify || config.GetConfigBool(configCCNotifyEnabled) {
		return types.TaskCCNotifyStatusPending
	}
	return types.TaskCCNotifyStatusSkipped
}

func (c *RESTClient) AddTaskCC(ctx stdcontext.Context, taskID string, req *types.TaskCCRequest, user *workflowcontext.UserContext) (*types.TaskCCResponse, error) {
	if !ccEnabled() {
		return nil, errors.New("workflow cc is disabled")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errors.New("task id is required")
	}
	if req == nil {
		return nil, errors.New("cc request is required")
	}
	task, err := c.getRuntimeTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return c.addTaskCCForTask(ctx, task, req, user, types.TaskCCSourceManual, "manual:"+taskID)
}

func (c *RESTClient) ListTaskCC(ctx stdcontext.Context, taskID string, user *workflowcontext.UserContext) (*types.TaskCCResponse, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, errors.New("task id is required")
	}
	db := ccDB()
	if db == nil || db.GetDB() == nil {
		return &types.TaskCCResponse{TaskID: strings.TrimSpace(taskID), Type: ccLinkType("")}, nil
	}
	if err := ensureCCTables(); err != nil {
		return nil, err
	}
	rows := make([]workflowCCRecordModel, 0)
	query := db.GetDB().WithContext(ctx).Where("task_id = ? AND relation_status = ?", strings.TrimSpace(taskID), types.TaskCCRelationStatusActive)
	if user != nil && strings.TrimSpace(user.TenantID) != "" {
		query = query.Where("(tenant_id = ? OR tenant_id = '')", strings.TrimSpace(user.TenantID))
	}
	if err := query.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return buildTaskCCResponse(strings.TrimSpace(taskID), rows), nil
}

func (c *RESTClient) RevokeTaskCCUser(ctx stdcontext.Context, taskID, userID string, user *workflowcontext.UserContext) (*types.TaskCCResponse, error) {
	return c.revokeTaskCC(ctx, taskID, types.TaskCCIdentityTypeUser, userID, user)
}

func (c *RESTClient) RevokeTaskCCGroup(ctx stdcontext.Context, taskID, groupID string, user *workflowcontext.UserContext) (*types.TaskCCResponse, error) {
	return c.revokeTaskCC(ctx, taskID, types.TaskCCIdentityTypeGroup, groupID, user)
}

func (c *RESTClient) revokeTaskCC(ctx stdcontext.Context, taskID, identityType, identityID string, user *workflowcontext.UserContext) (*types.TaskCCResponse, error) {
	taskID = strings.TrimSpace(taskID)
	identityID = strings.TrimSpace(identityID)
	if taskID == "" || identityID == "" {
		return nil, errors.New("task id and identity id are required")
	}
	db := ccDB()
	if db == nil || db.GetDB() == nil {
		return nil, errors.New("workflow cc storage is unavailable")
	}
	if err := ensureCCTables(); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"relation_status": types.TaskCCRelationStatusRevoked,
		"revoked_at":      &now,
	}
	if user != nil {
		updates["revoked_by"] = strings.TrimSpace(user.UserID)
	}
	query := db.GetDB().WithContext(ctx).Model(&workflowCCRecordModel{}).
		Where("task_id = ? AND identity_type = ? AND identity_id = ? AND relation_status = ?", taskID, identityType, identityID, types.TaskCCRelationStatusActive)
	if err := query.Updates(updates).Error; err != nil {
		return nil, err
	}
	_ = c.deleteTaskIdentityLink(ctx, taskID, identityType, identityID, ccLinkType(""))
	return c.ListTaskCC(ctx, taskID, user)
}

func (c *RESTClient) ListCC(ctx stdcontext.Context, user *workflowcontext.UserContext, query *types.TaskQuery) (*types.TaskCCPage, error) {
	query = normalizeTaskQuery(query)
	if user == nil || strings.TrimSpace(user.UserID) == "" {
		return nil, errors.New("workflow user context is required")
	}
	db := ccDB()
	if db == nil || db.GetDB() == nil {
		return &types.TaskCCPage{Items: []types.TaskCCItem{}, Total: 0, Start: query.Start, Size: query.Size}, nil
	}
	if err := ensureCCTables(); err != nil {
		return nil, err
	}
	base := c.ccUserScopeQuery(db.GetDB().WithContext(ctx), user)
	base = applyCCQueryFilters(base, query)
	base = applyCCReadStatusFilter(base, user, query)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, err
	}
	rows := make([]workflowCCRecordModel, 0, query.Size)
	if err := base.Order("workflow_cc_records.created_at DESC").Order("workflow_cc_records.id DESC").Offset(query.Start).Limit(query.Size).Find(&rows).Error; err != nil {
		return nil, err
	}
	readStates, err := loadCCReadStates(ctx, db.GetDB(), user.UserID, rows)
	if err != nil {
		return nil, err
	}
	items := make([]types.TaskCCItem, 0, len(rows))
	for _, row := range rows {
		item := row.toItem(readStates[row.ID])
		if query.IncludeProgress {
			if taskInfo, ok := c.buildCCTaskInfo(ctx, row); ok {
				item.Task = taskInfo
			}
		}
		items = append(items, item)
	}
	return &types.TaskCCPage{Items: items, Total: total, Start: query.Start, Size: query.Size}, nil
}

func (c *RESTClient) CountCC(ctx stdcontext.Context, user *workflowcontext.UserContext, query *types.TaskQuery) (*types.TaskCCCountResponse, error) {
	query = normalizeTaskQuery(query)
	if user == nil || strings.TrimSpace(user.UserID) == "" {
		return nil, errors.New("workflow user context is required")
	}
	db := ccDB()
	if db == nil || db.GetDB() == nil {
		return &types.TaskCCCountResponse{}, nil
	}
	if err := ensureCCTables(); err != nil {
		return nil, err
	}
	base := applyCCReadStatusFilter(applyCCQueryFilters(c.ccUserScopeQuery(db.GetDB().WithContext(ctx), user), query), user, query)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, err
	}
	var active int64
	if err := applyCCReadStatusFilter(applyCCQueryFilters(c.ccUserScopeQuery(db.GetDB().WithContext(ctx), user), query), user, query).
		Where("workflow_cc_records.relation_status = ?", types.TaskCCRelationStatusActive).Count(&active).Error; err != nil {
		return nil, err
	}
	var revoked int64
	if err := applyCCReadStatusFilter(applyCCQueryFilters(c.ccUserScopeQuery(db.GetDB().WithContext(ctx), user), query), user, query).
		Where("workflow_cc_records.relation_status = ?", types.TaskCCRelationStatusRevoked).Count(&revoked).Error; err != nil {
		return nil, err
	}
	var unread int64
	unreadQuery := applyCCUnreadFilter(applyCCQueryFilters(c.ccUserScopeQuery(db.GetDB().WithContext(ctx), user), query), user).
		Where("workflow_cc_records.relation_status = ?", types.TaskCCRelationStatusActive).
		Where("workflow_cc_read_states.id IS NULL OR workflow_cc_read_states.read_status <> ?", types.TaskCCReadStatusRead)
	if err := unreadQuery.Count(&unread).Error; err != nil {
		return nil, err
	}
	return &types.TaskCCCountResponse{Total: total, Unread: unread, Active: active, Revoked: revoked}, nil
}

func (c *RESTClient) MarkCCRead(ctx stdcontext.Context, user *workflowcontext.UserContext, req *types.TaskCCReadRequest) (*types.TaskCCReadResponse, error) {
	if user == nil || strings.TrimSpace(user.UserID) == "" {
		return nil, errors.New("workflow user context is required")
	}
	if req == nil || (len(req.CCIDs) == 0 && len(req.TaskIDs) == 0) {
		return &types.TaskCCReadResponse{Updated: 0}, nil
	}
	db := ccDB()
	if db == nil || db.GetDB() == nil {
		return &types.TaskCCReadResponse{Updated: 0}, nil
	}
	if err := ensureCCTables(); err != nil {
		return nil, err
	}
	base := c.ccUserScopeQuery(db.GetDB().WithContext(ctx), user)
	if len(req.CCIDs) > 0 {
		base = base.Where("workflow_cc_records.id IN ?", req.CCIDs)
	}
	if len(req.TaskIDs) > 0 {
		base = base.Where("task_id IN ?", trimmedStrings(req.TaskIDs))
	}
	rows := make([]workflowCCRecordModel, 0)
	if err := base.Find(&rows).Error; err != nil {
		return nil, err
	}
	return markCCRowsRead(ctx, db.GetDB(), user, rows)
}

func (c *RESTClient) MarkAllCCRead(ctx stdcontext.Context, user *workflowcontext.UserContext, query *types.TaskQuery) (*types.TaskCCReadResponse, error) {
	query = normalizeTaskQuery(query)
	if user == nil || strings.TrimSpace(user.UserID) == "" {
		return nil, errors.New("workflow user context is required")
	}
	db := ccDB()
	if db == nil || db.GetDB() == nil {
		return &types.TaskCCReadResponse{Updated: 0}, nil
	}
	if err := ensureCCTables(); err != nil {
		return nil, err
	}
	rows := make([]workflowCCRecordModel, 0)
	if err := applyCCQueryFilters(c.ccUserScopeQuery(db.GetDB().WithContext(ctx), user), query).
		Where("relation_status = ?", types.TaskCCRelationStatusActive).Find(&rows).Error; err != nil {
		return nil, err
	}
	return markCCRowsRead(ctx, db.GetDB(), user, rows)
}

func (c *RESTClient) addTaskCCForTask(ctx stdcontext.Context, task runtimeTaskRecord, req *types.TaskCCRequest, user *workflowcontext.UserContext, source, eventKey string) (*types.TaskCCResponse, error) {
	users := trimmedStrings(req.Users)
	groups := trimmedStrings(req.Groups)
	if len(users) == 0 && len(groups) == 0 {
		return nil, errors.New("cc users or groups are required")
	}
	db := ccDB()
	if db == nil || db.GetDB() == nil {
		return nil, errors.New("workflow cc storage is unavailable")
	}
	if err := ensureCCTables(); err != nil {
		return nil, err
	}
	linkType := ccLinkType(req.Type)
	variables := task.ProcessVariables
	nowStatus := types.TaskCCTaskStatusRunning
	createdBy := ""
	createdByName := ""
	if user != nil {
		createdBy = strings.TrimSpace(user.UserID)
		createdByName = firstNonBlank(strings.TrimSpace(user.UserName), createdBy)
	}
	created := make([]workflowCCRecordModel, 0, len(users)+len(groups))
	for _, ccUser := range users {
		if err := c.addTaskIdentityLink(ctx, task.ID, types.TaskCCIdentityTypeUser, ccUser, linkType); err != nil {
			return nil, err
		}
		created = append(created, c.upsertCCRecord(ctx, db.GetDB(), task, variables, types.TaskCCIdentityTypeUser, ccUser, linkType, nowStatus, source, eventKey, req, createdBy, createdByName))
	}
	for _, ccGroup := range groups {
		if err := c.addTaskIdentityLink(ctx, task.ID, types.TaskCCIdentityTypeGroup, ccGroup, linkType); err != nil {
			return nil, err
		}
		created = append(created, c.upsertCCRecord(ctx, db.GetDB(), task, variables, types.TaskCCIdentityTypeGroup, ccGroup, linkType, nowStatus, source, eventKey, req, createdBy, createdByName))
	}
	return buildTaskCCResponse(task.ID, created), nil
}

func (c *RESTClient) applyCCFromCallback(ctx stdcontext.Context, payload *types.FlowableCallbackPayload) {
	if payload == nil || strings.TrimSpace(payload.TaskID) == "" {
		return
	}
	task, err := c.getRuntimeTask(ctx, payload.TaskID)
	if err != nil {
		log.Warnf("【workflow】自动抄送获取任务失败: task_id=%s err=%v", payload.TaskID, err)
		return
	}
	variables := map[string]interface{}{}
	for key, value := range task.ProcessVariables {
		variables[key] = value
	}
	if len(payload.Variables) > 0 {
		for key, value := range payload.Variables {
			variables[key] = value
		}
	}
	if len(variables) == 0 && strings.TrimSpace(task.ProcessInstanceID) != "" {
		if loaded, loadErr := c.getProcessVariables(ctx, task.ProcessInstanceID); loadErr == nil {
			variables = loaded
		}
	}
	c.applyCCVariablesToTask(ctx, task, variables, nil, types.TaskCCSourceCallback, "callback:"+firstNonBlank(payload.EventID, payload.TaskID))
}

func (c *RESTClient) applyCCVariablesToCurrentTasks(ctx stdcontext.Context, processInstanceID string, variables map[string]interface{}, user *workflowcontext.UserContext, source, eventKey string) {
	if !ccEnabled() || strings.TrimSpace(processInstanceID) == "" {
		return
	}
	users, groups := ccValuesFromVariables(variables)
	if len(users) == 0 && len(groups) == 0 {
		return
	}
	tasks, err := c.queryRuntimeTasks(ctx, map[string]string{
		"processInstanceId": strings.TrimSpace(processInstanceID),
		"sort":              "createTime",
		"order":             "asc",
		"size":              "100",
	})
	if err != nil {
		log.Warnf("【workflow】自动抄送查询当前任务失败: process_instance_id=%s err=%v", processInstanceID, err)
		return
	}
	for _, task := range tasks {
		c.applyCCVariablesToTask(ctx, task, variables, user, source, eventKey+":"+task.ID)
	}
}

func (c *RESTClient) applyCCVariablesToTask(ctx stdcontext.Context, task runtimeTaskRecord, variables map[string]interface{}, user *workflowcontext.UserContext, source, eventKey string) {
	if !ccEnabled() || strings.TrimSpace(task.ID) == "" {
		return
	}
	users, groups := ccValuesFromVariables(variables)
	if len(users) == 0 && len(groups) == 0 {
		return
	}
	if task.ProcessVariables == nil {
		task.ProcessVariables = map[string]interface{}{}
	}
	for key, value := range variables {
		task.ProcessVariables[key] = value
	}
	_, err := c.addTaskCCForTask(ctx, task, &types.TaskCCRequest{
		Users:  users,
		Groups: groups,
		Type:   ccLinkType(""),
		Notify: config.GetConfigBool(configCCNotifyEnabled),
		Reason: "流程自动抄送",
	}, user, source, eventKey)
	if err != nil {
		log.Warnf("【workflow】自动抄送失败: task_id=%s err=%v", task.ID, err)
	}
}

func (c *RESTClient) upsertCCRecord(ctx stdcontext.Context, db *gorm.DB, task runtimeTaskRecord, variables map[string]interface{}, identityType, identityID, linkType, taskStatus, source, eventKey string, req *types.TaskCCRequest, createdBy, createdByName string) workflowCCRecordModel {
	row := workflowCCRecordModel{}
	query := db.WithContext(ctx).Where("task_id = ? AND identity_type = ? AND identity_id = ? AND relation_status = ?", strings.TrimSpace(task.ID), identityType, identityID, types.TaskCCRelationStatusActive)
	if err := query.First(&row).Error; err == nil {
		return row
	}
	row = workflowCCRecordModel{
		TenantID:             firstNonBlank(stringValue(variables["tenantId"]), strings.TrimSpace(task.TenantID)),
		SystemCode:           stringValue(variables["systemCode"]),
		TaskID:               strings.TrimSpace(task.ID),
		ProcessInstanceID:    strings.TrimSpace(task.ProcessInstanceID),
		ProcessDefinitionID:  strings.TrimSpace(task.ProcessDefinitionID),
		ProcessDefinitionKey: processDefinitionKeyFrom(task.ProcessDefinitionID, variables),
		ActivityID:           strings.TrimSpace(task.TaskDefinitionKey),
		ActivityName:         firstNonBlank(strings.TrimSpace(task.Name), strings.TrimSpace(task.TaskDefinitionKey)),
		BizID:                firstNonBlank(stringValue(variables["bizId"]), strings.TrimSpace(task.BusinessKey)),
		BizType:              stringValue(variables["bizType"]),
		Title:                stringValue(variables["title"]),
		IdentityType:         identityType,
		IdentityID:           identityID,
		LinkType:             linkType,
		RelationStatus:       types.TaskCCRelationStatusActive,
		TaskStatus:           taskStatus,
		NotifyStatus:         ccNotifyStatus(req.Notify),
		Source:               source,
		SourceEventKey:       eventKey,
		Reason:               strings.TrimSpace(req.Reason),
		CreatedBy:            createdBy,
		CreatedByName:        createdByName,
	}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		log.Warnf("【workflow】workflow_cc_records 写入失败: %v", err)
	}
	return row
}

func (c *RESTClient) addTaskIdentityLink(ctx stdcontext.Context, taskID, identityType, identityID, linkType string) error {
	if c.taskIdentityLinkExists(ctx, taskID, identityType, identityID, linkType) {
		return nil
	}
	payload := map[string]interface{}{"type": ccLinkType(linkType)}
	switch identityType {
	case types.TaskCCIdentityTypeUser:
		payload["user"] = strings.TrimSpace(identityID)
	case types.TaskCCIdentityTypeGroup:
		payload["group"] = strings.TrimSpace(identityID)
	default:
		return fmt.Errorf("invalid cc identity type: %s", identityType)
	}
	_, err := c.doJSON(ctx, http.MethodPost, "/runtime/tasks/"+strings.TrimSpace(taskID)+"/identitylinks", nil, payload)
	return err
}

func (c *RESTClient) taskIdentityLinkExists(ctx stdcontext.Context, taskID, identityType, identityID, linkType string) bool {
	links, err := c.getTaskIdentityLinks(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return false
	}
	identityID = strings.TrimSpace(identityID)
	linkType = ccLinkType(linkType)
	for _, link := range links {
		if !strings.EqualFold(strings.TrimSpace(link.Type), linkType) {
			continue
		}
		switch identityType {
		case types.TaskCCIdentityTypeUser:
			if strings.TrimSpace(link.User) == identityID {
				return true
			}
		case types.TaskCCIdentityTypeGroup:
			if strings.TrimSpace(link.Group) == identityID {
				return true
			}
		}
	}
	return false
}

func (c *RESTClient) deleteTaskIdentityLink(ctx stdcontext.Context, taskID, identityType, identityID, linkType string) error {
	segment := "users"
	if identityType == types.TaskCCIdentityTypeGroup {
		segment = "groups"
	}
	path := "/runtime/tasks/" + strings.TrimSpace(taskID) + "/identitylinks/" + segment + "/" + strings.TrimSpace(identityID) + "/" + ccLinkType(linkType)
	_, err := c.doJSON(ctx, http.MethodDelete, path, nil, nil)
	return err
}

func (c *RESTClient) ccUserScopeQuery(db *gorm.DB, user *workflowcontext.UserContext) *gorm.DB {
	groups := candidateGroups(user, c.groupPrefix, c.rolePrefix)
	query := db.Model(&workflowCCRecordModel{}).Where("workflow_cc_records.relation_status <> ?", types.TaskCCRelationStatusDeleted)
	if strings.TrimSpace(user.TenantID) != "" {
		query = query.Where("(workflow_cc_records.tenant_id = ? OR workflow_cc_records.tenant_id = '')", strings.TrimSpace(user.TenantID))
	}
	if strings.TrimSpace(user.SystemCode) != "" {
		query = query.Where("(workflow_cc_records.system_code = ? OR workflow_cc_records.system_code = '')", strings.TrimSpace(user.SystemCode))
	}
	if len(groups) > 0 {
		return query.Where("(workflow_cc_records.identity_type = ? AND workflow_cc_records.identity_id = ?) OR (workflow_cc_records.identity_type = ? AND workflow_cc_records.identity_id IN ?)", types.TaskCCIdentityTypeUser, strings.TrimSpace(user.UserID), types.TaskCCIdentityTypeGroup, groups)
	}
	return query.Where("workflow_cc_records.identity_type = ? AND workflow_cc_records.identity_id = ?", types.TaskCCIdentityTypeUser, strings.TrimSpace(user.UserID))
}

func (c *RESTClient) buildTaskAccessInfo(ctx stdcontext.Context, task runtimeTaskRecord, user *workflowcontext.UserContext) *types.TaskAccessInfo {
	if user == nil || strings.TrimSpace(user.UserID) == "" {
		return &types.TaskAccessInfo{ReadOnly: true}
	}
	isAssignee := matchesAssignedRuntimeTask(task, user)
	isCandidate := false
	if allowed, err := c.matchesCandidateRuntimeTask(ctx, task, user); err == nil {
		isCandidate = allowed
	}
	isCC := c.isTaskCCVisibleToUser(ctx, task.ID, user)
	canClaim := !isAssignee && isCandidate && strings.TrimSpace(task.Assignee) == ""
	canComplete := isAssignee || isCandidate
	canTransfer := isAssignee
	readOnly := isCC && !canComplete && !canClaim && !canTransfer
	return &types.TaskAccessInfo{
		IsAssignee:  isAssignee,
		IsCandidate: isCandidate,
		IsCC:        isCC,
		ReadOnly:    readOnly,
		CanClaim:    canClaim,
		CanComplete: canComplete,
		CanTransfer: canTransfer,
	}
}

func (c *RESTClient) isTaskCCVisibleToUser(ctx stdcontext.Context, taskID string, user *workflowcontext.UserContext) bool {
	if user == nil || strings.TrimSpace(user.UserID) == "" || strings.TrimSpace(taskID) == "" {
		return false
	}
	db := ccDB()
	if db == nil || db.GetDB() == nil {
		return false
	}
	if err := ensureCCTables(); err != nil {
		return false
	}
	var count int64
	query := c.ccUserScopeQuery(db.GetDB().WithContext(ctx), user).
		Where("task_id = ? AND relation_status = ?", strings.TrimSpace(taskID), types.TaskCCRelationStatusActive)
	if err := query.Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func (c *RESTClient) updateCCRecordsByTaskID(ctx stdcontext.Context, taskID, taskStatus string) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}
	c.updateCCRecords(ctx, "task_id = ?", taskID, taskStatus)
}

func (c *RESTClient) updateCCRecordsByProcessInstanceID(ctx stdcontext.Context, processInstanceID, taskStatus string) {
	processInstanceID = strings.TrimSpace(processInstanceID)
	if processInstanceID == "" {
		return
	}
	c.updateCCRecords(ctx, "process_instance_id = ?", processInstanceID, taskStatus)
}

func (c *RESTClient) updateCCRecords(ctx stdcontext.Context, condition string, value interface{}, taskStatus string) {
	db := ccDB()
	if db == nil || db.GetDB() == nil {
		return
	}
	if err := ensureCCTables(); err != nil {
		log.Warnf("【workflow】抄送表初始化失败: err=%v", err)
		return
	}
	if err := db.GetDB().WithContext(ctx).Model(&workflowCCRecordModel{}).
		Where(condition, value).
		Where("relation_status = ?", types.TaskCCRelationStatusActive).
		Update("task_status", strings.TrimSpace(taskStatus)).Error; err != nil {
		log.Warnf("【workflow】抄送任务状态更新失败: err=%v", err)
	}
}

func applyCCQueryFilters(db *gorm.DB, query *types.TaskQuery) *gorm.DB {
	if query == nil {
		return db
	}
	if strings.TrimSpace(query.Title) != "" {
		db = db.Where("workflow_cc_records.title LIKE ?", "%"+strings.TrimSpace(query.Title)+"%")
	}
	if strings.TrimSpace(query.BizType) != "" {
		db = db.Where("workflow_cc_records.biz_type = ?", strings.TrimSpace(query.BizType))
	}
	if strings.TrimSpace(query.ProcessDefinitionKey) != "" {
		db = db.Where("workflow_cc_records.process_definition_key = ?", strings.TrimSpace(query.ProcessDefinitionKey))
	}
	if strings.TrimSpace(query.ActivityID) != "" {
		db = db.Where("workflow_cc_records.activity_id = ?", strings.TrimSpace(query.ActivityID))
	}
	if after := parseTime(query.CreatedAfter); !after.IsZero() {
		db = db.Where("workflow_cc_records.created_at >= ?", after)
	}
	if before := parseTime(query.CreatedBefore); !before.IsZero() {
		db = db.Where("workflow_cc_records.created_at <= ?", before)
	}
	if after := parseTime(query.CompletedAfter); !after.IsZero() {
		db = db.Where("workflow_cc_records.revoked_at >= ?", after)
	}
	if before := parseTime(query.CompletedBefore); !before.IsZero() {
		db = db.Where("workflow_cc_records.revoked_at <= ?", before)
	}
	status := strings.ToLower(strings.TrimSpace(query.Status))
	switch status {
	case types.TaskCCRelationStatusActive, types.TaskCCRelationStatusRevoked, types.TaskCCRelationStatusDeleted:
		db = db.Where("workflow_cc_records.relation_status = ?", status)
	case types.TaskCCTaskStatusRunning, types.TaskCCTaskStatusCompleted, types.TaskCCTaskStatusCanceled, types.TaskCCTaskStatusTerminated:
		db = db.Where("workflow_cc_records.task_status = ?", status)
	case types.TaskCCReadStatusUnread:
		// 未读是用户维度状态，由 applyCCReadStatusFilter 统一追加关联查询。
	case types.TaskCCReadStatusRead:
	}
	return db
}

func applyCCReadStatusFilter(db *gorm.DB, user *workflowcontext.UserContext, query *types.TaskQuery) *gorm.DB {
	if query == nil {
		return db
	}
	switch strings.ToLower(strings.TrimSpace(query.Status)) {
	case types.TaskCCReadStatusUnread:
		return applyCCUnreadFilter(db, user)
	case types.TaskCCReadStatusRead:
		if user == nil {
			return db.Where("1 = 0")
		}
		return db.Joins("INNER JOIN workflow_cc_read_states ON workflow_cc_read_states.cc_record_id = workflow_cc_records.id AND workflow_cc_read_states.user_id = ? AND workflow_cc_read_states.read_status = ?", strings.TrimSpace(user.UserID), types.TaskCCReadStatusRead)
	default:
		return db
	}
}

func applyCCUnreadFilter(db *gorm.DB, user *workflowcontext.UserContext) *gorm.DB {
	if user == nil {
		return db.Where("1 = 0")
	}
	return db.Joins("LEFT JOIN workflow_cc_read_states ON workflow_cc_read_states.cc_record_id = workflow_cc_records.id AND workflow_cc_read_states.user_id = ?", strings.TrimSpace(user.UserID))
}

func loadCCReadStates(ctx stdcontext.Context, db *gorm.DB, userID string, rows []workflowCCRecordModel) (map[uint]workflowCCReadStateModel, error) {
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	result := map[uint]workflowCCReadStateModel{}
	if len(ids) == 0 {
		return result, nil
	}
	states := make([]workflowCCReadStateModel, 0, len(ids))
	if err := db.WithContext(ctx).Where("cc_record_id IN ? AND user_id = ?", ids, strings.TrimSpace(userID)).Find(&states).Error; err != nil {
		return nil, err
	}
	for _, state := range states {
		result[state.CCRecordID] = state
	}
	return result, nil
}

func markCCRowsRead(ctx stdcontext.Context, db *gorm.DB, user *workflowcontext.UserContext, rows []workflowCCRecordModel) (*types.TaskCCReadResponse, error) {
	updated := int64(0)
	now := time.Now().UTC()
	for _, row := range rows {
		state := workflowCCReadStateModel{}
		err := db.WithContext(ctx).Where("cc_record_id = ? AND user_id = ?", row.ID, strings.TrimSpace(user.UserID)).First(&state).Error
		if err == nil {
			if state.ReadStatus == types.TaskCCReadStatusRead {
				continue
			}
			if err := db.WithContext(ctx).Model(&state).Updates(map[string]interface{}{"read_status": types.TaskCCReadStatusRead, "read_at": now}).Error; err != nil {
				return nil, err
			}
			updated++
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		state = workflowCCReadStateModel{
			TenantID:      row.TenantID,
			CCRecordID:    row.ID,
			UserID:        strings.TrimSpace(user.UserID),
			ReadStatus:    types.TaskCCReadStatusRead,
			ReadAt:        now,
			LastTaskID:    row.TaskID,
			LastIdentity:  row.IdentityID,
			LastReadScope: row.IdentityType,
		}
		if err := db.WithContext(ctx).Create(&state).Error; err != nil {
			return nil, err
		}
		updated++
	}
	return &types.TaskCCReadResponse{Updated: updated}, nil
}

func (row workflowCCRecordModel) toItem(readState workflowCCReadStateModel) types.TaskCCItem {
	readStatus := types.TaskCCReadStatusUnread
	readAt := ""
	if readState.ID != 0 && readState.ReadStatus == types.TaskCCReadStatusRead {
		readStatus = types.TaskCCReadStatusRead
		if !readState.ReadAt.IsZero() {
			readAt = readState.ReadAt.Format(time.RFC3339)
		}
	}
	createdAt := ""
	if !row.CreatedAt.IsZero() {
		createdAt = row.CreatedAt.Format(time.RFC3339)
	}
	revokedAt := ""
	if row.RevokedAt != nil && !row.RevokedAt.IsZero() {
		revokedAt = row.RevokedAt.Format(time.RFC3339)
	}
	return types.TaskCCItem{
		CCID:                 row.ID,
		TaskID:               row.TaskID,
		ProcessInstanceID:    row.ProcessInstanceID,
		ProcessDefinitionID:  row.ProcessDefinitionID,
		ProcessDefinitionKey: row.ProcessDefinitionKey,
		ActivityID:           row.ActivityID,
		ActivityName:         row.ActivityName,
		BizID:                row.BizID,
		BizType:              row.BizType,
		Title:                row.Title,
		TenantID:             row.TenantID,
		SystemCode:           row.SystemCode,
		IdentityType:         row.IdentityType,
		IdentityID:           row.IdentityID,
		LinkType:             firstNonBlank(row.LinkType, defaultCCIdentityLinkType),
		RelationStatus:       firstNonBlank(row.RelationStatus, types.TaskCCRelationStatusActive),
		ReadStatus:           readStatus,
		TaskStatus:           firstNonBlank(row.TaskStatus, types.TaskCCTaskStatusUnknown),
		NotifyStatus:         firstNonBlank(row.NotifyStatus, types.TaskCCNotifyStatusSkipped),
		Source:               row.Source,
		Reason:               row.Reason,
		CreatedBy:            row.CreatedBy,
		CreatedByName:        row.CreatedByName,
		CreatedAt:            createdAt,
		ReadAt:               readAt,
		RevokedAt:            revokedAt,
	}
}

func (c *RESTClient) buildCCTaskInfo(ctx stdcontext.Context, row workflowCCRecordModel) (types.TaskInfo, bool) {
	if task, err := c.getRuntimeTask(ctx, row.TaskID); err == nil {
		info := c.toTaskInfo(task, nil, false)
		return info, true
	}
	return types.TaskInfo{
		TaskID:              row.TaskID,
		TaskName:            row.ActivityName,
		ActivityID:          row.ActivityID,
		ActivityName:        row.ActivityName,
		ProcessInstanceID:   row.ProcessInstanceID,
		ProcessDefinitionID: row.ProcessDefinitionID,
		BizID:               row.BizID,
		BizType:             row.BizType,
		Title:               row.Title,
		TenantID:            row.TenantID,
	}, true
}

func buildTaskCCResponse(taskID string, rows []workflowCCRecordModel) *types.TaskCCResponse {
	resp := &types.TaskCCResponse{TaskID: strings.TrimSpace(taskID), Type: ccLinkType("")}
	for _, row := range rows {
		identity := types.TaskCCIdentity{IdentityType: row.IdentityType, IdentityID: row.IdentityID, LinkType: firstNonBlank(row.LinkType, defaultCCIdentityLinkType), RelationStatus: row.RelationStatus}
		resp.Items = append(resp.Items, identity)
		if row.IdentityType == types.TaskCCIdentityTypeUser {
			resp.Users = append(resp.Users, row.IdentityID)
		}
		if row.IdentityType == types.TaskCCIdentityTypeGroup {
			resp.Groups = append(resp.Groups, row.IdentityID)
		}
	}
	return resp
}

func trimmedStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		current := strings.TrimSpace(value)
		if current == "" {
			continue
		}
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		result = append(result, current)
	}
	return result
}

func ccValuesFromVariables(variables map[string]interface{}) ([]string, []string) {
	if len(variables) == 0 {
		return nil, nil
	}
	userKey := firstNonBlank(strings.TrimSpace(config.GetConfigString(configCCVariableUsers)), defaultCCVariableUsers)
	groupKey := firstNonBlank(strings.TrimSpace(config.GetConfigString(configCCVariableGroups)), defaultCCVariableGroups)
	return stringsFromVariableValue(variables[userKey]), stringsFromVariableValue(variables[groupKey])
}

func stringsFromVariableValue(value interface{}) []string {
	switch current := value.(type) {
	case nil:
		return nil
	case []string:
		return trimmedStrings(current)
	case []interface{}:
		result := make([]string, 0, len(current))
		for _, item := range current {
			result = append(result, fmt.Sprint(item))
		}
		return trimmedStrings(result)
	case string:
		if strings.TrimSpace(current) == "" {
			return nil
		}
		parts := strings.FieldsFunc(current, func(r rune) bool {
			return r == ',' || r == ';' || r == '，' || r == '；'
		})
		return trimmedStrings(parts)
	default:
		text := strings.TrimSpace(fmt.Sprint(current))
		if text == "" {
			return nil
		}
		return []string{text}
	}
}
