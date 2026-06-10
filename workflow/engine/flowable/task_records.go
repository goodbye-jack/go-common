package flowable

import (
	stdcontext "context"
	"errors"
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
	configTaskRecordsDBInstance = "workflow.records.db_instance"
	taskRecordSourceWorkflowAPI = "workflow_api"
	taskRecordStartActivityID   = "process_start"
	taskRecordStartActivityName = "流程发起"
)

var (
	taskRecordMigrateMu sync.Mutex
	taskRecordMigrated  = map[string]bool{}
)

type workflowTaskRecordModel struct {
	commonmodel.ModelBase
	RootProcessInstanceID string    `gorm:"size:64;index:idx_wtr_root_time" json:"rootProcessInstanceId"`
	ProcessInstanceID     string    `gorm:"size:64;index" json:"processInstanceId"`
	ProcessDefinitionID   string    `gorm:"size:128;index" json:"processDefinitionId"`
	ProcessDefinitionKey  string    `gorm:"size:128;index" json:"processDefinitionKey"`
	TaskID                string    `gorm:"size:64;index" json:"taskId"`
	TaskName              string    `gorm:"size:255" json:"taskName"`
	ActivityID            string    `gorm:"size:128;index" json:"activityId"`
	ActivityName          string    `gorm:"size:255" json:"activityName"`
	BizID                 string    `gorm:"size:128;index" json:"bizId"`
	BizType               string    `gorm:"size:128;index" json:"bizType"`
	Title                 string    `gorm:"size:255" json:"title"`
	TenantID              string    `gorm:"size:128;index" json:"tenantId"`
	SystemCode            string    `gorm:"size:128;index" json:"systemCode"`
	ActionType            string    `gorm:"size:64;index:idx_wtr_action_time" json:"actionType"`
	OperatorUserID        string    `gorm:"size:128;index:idx_wtr_operator_time" json:"operatorUserId"`
	OperatorUserName      string    `gorm:"size:255" json:"operatorUserName"`
	FromAssignee          string    `gorm:"size:128" json:"fromAssignee"`
	ToAssignee            string    `gorm:"size:128" json:"toAssignee"`
	FromOwner             string    `gorm:"size:128" json:"fromOwner"`
	ToOwner               string    `gorm:"size:128" json:"toOwner"`
	Comment               string    `gorm:"type:text" json:"comment"`
	Reason                string    `gorm:"type:text" json:"reason"`
	Source                string    `gorm:"size:64;uniqueIndex:uniq_wtr_source_event" json:"source"`
	SourceEventKey        string    `gorm:"size:255;uniqueIndex:uniq_wtr_source_event" json:"sourceEventKey"`
	ActionTime            time.Time `gorm:"index:idx_wtr_action_time;index:idx_wtr_root_time;index:idx_wtr_operator_time" json:"actionTime"`
}

func (workflowTaskRecordModel) TableName() string {
	return "workflow_task_records"
}

func taskRecordDB() *orm.Orm {
	instanceName := strings.TrimSpace(config.GetConfigString(configTaskRecordsDBInstance))
	if instanceName != "" {
		return orm.GetDB(instanceName)
	}
	return orm.DB
}

func ensureTaskRecordTable() error {
	db := taskRecordDB()
	if db == nil || db.GetDB() == nil {
		return nil
	}
	key := strings.TrimSpace(config.GetConfigString(configTaskRecordsDBInstance))
	if key == "" {
		key = "default"
	}
	taskRecordMigrateMu.Lock()
	defer taskRecordMigrateMu.Unlock()
	if taskRecordMigrated[key] {
		return nil
	}
	if err := db.GetDB().AutoMigrate(&workflowTaskRecordModel{}); err != nil {
		return err
	}
	taskRecordMigrated[key] = true
	return nil
}

func (c *RESTClient) recordProcessStart(ctx stdcontext.Context, req *types.StartProcessRequest, resp *types.StartProcessResponse) {
	if resp == nil || strings.TrimSpace(resp.ProcessInstanceID) == "" {
		return
	}
	variables := map[string]interface{}{}
	if req != nil && req.Variables != nil {
		for key, value := range req.Variables {
			variables[key] = value
		}
	}
	definitionKey := ""
	if req != nil {
		definitionKey = strings.TrimSpace(req.ProcessDefinitionKey)
	}
	if definitionKey == "" {
		definitionKey = processDefinitionKey(resp.ProcessDefinitionID)
	}
	record := workflowTaskRecordModel{
		RootProcessInstanceID: strings.TrimSpace(resp.ProcessInstanceID),
		ProcessInstanceID:     strings.TrimSpace(resp.ProcessInstanceID),
		ProcessDefinitionID:   strings.TrimSpace(resp.ProcessDefinitionID),
		ProcessDefinitionKey:  definitionKey,
		ActivityID:            taskRecordStartActivityID,
		ActivityName:          taskRecordStartActivityName,
		BizID:                 firstNonBlank(stringValue(variables["bizId"]), stringValue(variables["businessKey"]), stringValue(reqFieldBizID(req))),
		BizType:               stringValue(variables["bizType"]),
		Title:                 firstNonBlank(stringValue(variables["title"]), stringValue(reqFieldTitle(req))),
		TenantID:              firstNonBlank(stringValue(variables["tenantId"]), strings.TrimSpace(resp.TenantID)),
		SystemCode:            stringValue(variables["systemCode"]),
		ActionType:            types.TaskActionTypeStartProcess,
		OperatorUserID:        firstNonBlank(stringValue(variables["starterId"]), stringValue(variables["startUserId"])),
		OperatorUserName:      firstNonBlank(stringValue(variables["starterName"]), stringValue(variables["startUserName"])),
		Source:                taskRecordSourceWorkflowAPI,
		SourceEventKey:        "start:" + strings.TrimSpace(resp.ProcessInstanceID),
		ActionTime:            time.Now().UTC(),
	}
	c.insertTaskRecord(ctx, record)
}

func (c *RESTClient) recordTaskActionRecord(ctx stdcontext.Context, actionType string, beforeTask, afterTask runtimeTaskRecord, user *workflowcontext.UserContext, comment, reason string) {
	task := afterTask
	if strings.TrimSpace(task.ID) == "" {
		task = beforeTask
	}
	if strings.TrimSpace(task.ProcessInstanceID) == "" {
		return
	}
	rootProcessInstanceID := strings.TrimSpace(task.ProcessInstanceID)
	if process, err := c.getProcessInstance(ctx, task.ProcessInstanceID); err == nil {
		if root, resolveErr := c.resolveRootProcessInstance(ctx, process); resolveErr == nil && strings.TrimSpace(root.ID) != "" {
			rootProcessInstanceID = strings.TrimSpace(root.ID)
		}
	}
	variables := map[string]interface{}{}
	for key, value := range beforeTask.ProcessVariables {
		variables[key] = value
	}
	for key, value := range afterTask.ProcessVariables {
		variables[key] = value
	}
	operatorUserID := ""
	operatorUserName := ""
	if user != nil {
		operatorUserID = strings.TrimSpace(user.UserID)
		operatorUserName = firstNonBlank(strings.TrimSpace(user.UserName), strings.TrimSpace(user.UserID))
	}
	record := workflowTaskRecordModel{
		RootProcessInstanceID: rootProcessInstanceID,
		ProcessInstanceID:     strings.TrimSpace(task.ProcessInstanceID),
		ProcessDefinitionID:   strings.TrimSpace(task.ProcessDefinitionID),
		ProcessDefinitionKey:  processDefinitionKey(task.ProcessDefinitionID),
		TaskID:                strings.TrimSpace(task.ID),
		TaskName:              firstNonBlank(strings.TrimSpace(task.Name), strings.TrimSpace(beforeTask.Name)),
		ActivityID:            firstNonBlank(strings.TrimSpace(task.TaskDefinitionKey), strings.TrimSpace(beforeTask.TaskDefinitionKey)),
		ActivityName:          firstNonBlank(strings.TrimSpace(task.Name), strings.TrimSpace(beforeTask.Name), strings.TrimSpace(task.TaskDefinitionKey), strings.TrimSpace(beforeTask.TaskDefinitionKey)),
		BizID:                 firstNonBlank(stringValue(variables["bizId"]), strings.TrimSpace(task.BusinessKey), strings.TrimSpace(beforeTask.BusinessKey)),
		BizType:               stringValue(variables["bizType"]),
		Title:                 stringValue(variables["title"]),
		TenantID:              firstNonBlank(stringValue(variables["tenantId"]), strings.TrimSpace(task.TenantID), strings.TrimSpace(beforeTask.TenantID)),
		SystemCode:            stringValue(variables["systemCode"]),
		ActionType:            strings.ToUpper(strings.TrimSpace(actionType)),
		OperatorUserID:        operatorUserID,
		OperatorUserName:      operatorUserName,
		FromAssignee:          strings.TrimSpace(beforeTask.Assignee),
		ToAssignee:            strings.TrimSpace(afterTask.Assignee),
		FromOwner:             strings.TrimSpace(beforeTask.Owner),
		ToOwner:               strings.TrimSpace(afterTask.Owner),
		Comment:               strings.TrimSpace(comment),
		Reason:                strings.TrimSpace(reason),
		Source:                taskRecordSourceWorkflowAPI,
		SourceEventKey:        buildTaskRecordEventKey(actionType, beforeTask, afterTask),
		ActionTime:            time.Now().UTC(),
	}
	enrichTaskRecordScope(&record, user)
	c.insertTaskRecord(ctx, record)
}

func enrichTaskRecordScope(record *workflowTaskRecordModel, user *workflowcontext.UserContext) {
	if record == nil || user == nil {
		return
	}
	record.TenantID = firstNonBlank(strings.TrimSpace(record.TenantID), strings.TrimSpace(user.TenantID))
	record.SystemCode = firstNonBlank(strings.TrimSpace(record.SystemCode), strings.TrimSpace(user.SystemCode))
}

func (c *RESTClient) insertTaskRecord(ctx stdcontext.Context, record workflowTaskRecordModel) {
	db := taskRecordDB()
	if db == nil || db.GetDB() == nil {
		return
	}
	if err := ensureTaskRecordTable(); err != nil {
		log.Warnf("【workflow】workflow_task_records 自动建表失败: %v", err)
		return
	}
	if record.ActionTime.IsZero() {
		record.ActionTime = time.Now().UTC()
	}
	if err := db.GetDB().WithContext(ctx).Create(&record).Error; err != nil {
		log.Warnf("【workflow】workflow_task_records 写入失败: %v", err)
	}
}

func buildTaskRecordEventKey(actionType string, beforeTask, afterTask runtimeTaskRecord) string {
	taskID := firstNonBlank(strings.TrimSpace(afterTask.ID), strings.TrimSpace(beforeTask.ID))
	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(actionType)),
		taskID,
		strings.TrimSpace(beforeTask.Assignee),
		strings.TrimSpace(afterTask.Assignee),
		strings.TrimSpace(beforeTask.Owner),
		strings.TrimSpace(afterTask.Owner),
	}, ":")
}

func reqFieldBizID(req *types.StartProcessRequest) string {
	if req == nil {
		return ""
	}
	return req.BizID
}

func reqFieldTitle(req *types.StartProcessRequest) string {
	if req == nil {
		return ""
	}
	return req.Title
}

func (c *RESTClient) ListTaskRecords(ctx stdcontext.Context, user *workflowcontext.UserContext, query *types.TaskQuery) (*types.WorkflowTaskRecordPage, error) {
	query = normalizeTaskQuery(query)
	if user == nil || strings.TrimSpace(user.UserID) == "" {
		return nil, errors.New("workflow user context is required")
	}
	db := taskRecordDB()
	if db == nil || db.GetDB() == nil {
		return &types.WorkflowTaskRecordPage{
			Items: []types.WorkflowTaskRecordItem{},
			Total: 0,
			Start: query.Start,
			Size:  query.Size,
		}, nil
	}
	if err := ensureTaskRecordTable(); err != nil {
		return nil, err
	}
	base := db.GetDB().WithContext(ctx).Model(&workflowTaskRecordModel{}).
		Where("operator_user_id = ?", strings.TrimSpace(user.UserID))
	if strings.TrimSpace(user.TenantID) != "" {
		base = base.Where("(tenant_id = ? OR tenant_id = '')", strings.TrimSpace(user.TenantID))
	}
	if strings.TrimSpace(user.SystemCode) != "" {
		base = base.Where("(system_code = ? OR system_code = '')", strings.TrimSpace(user.SystemCode))
	}
	base = applyTaskRecordQueryFilters(base, query)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, err
	}
	rows := make([]workflowTaskRecordModel, 0, query.Size)
	if err := base.Order("action_time DESC").Order("id DESC").Offset(query.Start).Limit(query.Size).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]types.WorkflowTaskRecordItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toItem())
	}
	return &types.WorkflowTaskRecordPage{
		Items: items,
		Total: total,
		Start: query.Start,
		Size:  query.Size,
	}, nil
}

func (c *RESTClient) GetTaskRecords(ctx stdcontext.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessTaskRecordResponse, error) {
	summaryProcessInstanceID, err := c.resolveSummaryProcessInstanceID(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	summary, err := c.getProcessSummary(ctx, summaryProcessInstanceID, user)
	if err != nil {
		return nil, err
	}
	items, err := c.loadTaskRecordItemsByRootProcessInstanceID(ctx, summaryProcessInstanceID)
	if err != nil {
		return nil, err
	}
	return &types.ProcessTaskRecordResponse{
		Summary: summary,
		Items:   items,
	}, nil
}

func (c *RESTClient) GetTaskRecordsByBizID(ctx stdcontext.Context, bizID string, user *workflowcontext.UserContext) (*types.ProcessTaskRecordResponse, error) {
	processInstanceID, err := c.ResolveProcessInstanceIDByBizID(ctx, bizID, user)
	if err != nil {
		return nil, err
	}
	return c.GetTaskRecords(ctx, processInstanceID, user)
}

func (c *RESTClient) loadTaskRecordItemsByRootProcessInstanceID(ctx stdcontext.Context, rootProcessInstanceID string) ([]types.WorkflowTaskRecordItem, error) {
	db := taskRecordDB()
	if db == nil || db.GetDB() == nil || strings.TrimSpace(rootProcessInstanceID) == "" {
		return []types.WorkflowTaskRecordItem{}, nil
	}
	if err := ensureTaskRecordTable(); err != nil {
		return nil, err
	}
	rows := make([]workflowTaskRecordModel, 0)
	if err := db.GetDB().WithContext(ctx).
		Where("root_process_instance_id = ?", strings.TrimSpace(rootProcessInstanceID)).
		Order("action_time ASC").Order("id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]types.WorkflowTaskRecordItem, 0, len(rows))
	for index, row := range rows {
		item := row.toItem()
		item.Sequence = index + 1
		items = append(items, item)
	}
	return items, nil
}

func applyTaskRecordQueryFilters(db *gorm.DB, query *types.TaskQuery) *gorm.DB {
	if query == nil {
		return db
	}
	if strings.TrimSpace(query.Title) != "" {
		db = db.Where("title LIKE ?", "%"+strings.TrimSpace(query.Title)+"%")
	}
	if strings.TrimSpace(query.BizType) != "" {
		db = db.Where("biz_type = ?", strings.TrimSpace(query.BizType))
	}
	if strings.TrimSpace(query.ProcessDefinitionKey) != "" {
		db = db.Where("process_definition_key = ?", strings.TrimSpace(query.ProcessDefinitionKey))
	}
	if strings.TrimSpace(query.ActivityID) != "" {
		db = db.Where("activity_id = ?", strings.TrimSpace(query.ActivityID))
	}
	if strings.TrimSpace(query.ActionType) != "" {
		db = db.Where("action_type = ?", strings.ToUpper(strings.TrimSpace(query.ActionType)))
	}
	return db
}

func (m workflowTaskRecordModel) toItem() types.WorkflowTaskRecordItem {
	return types.WorkflowTaskRecordItem{
		RecordID:              m.ID,
		ActionType:            strings.ToUpper(strings.TrimSpace(m.ActionType)),
		Time:                  formatTime(m.ActionTime),
		ProcessInstanceID:     strings.TrimSpace(m.ProcessInstanceID),
		RootProcessInstanceID: strings.TrimSpace(m.RootProcessInstanceID),
		ProcessDefinitionID:   strings.TrimSpace(m.ProcessDefinitionID),
		ProcessDefinitionKey:  strings.TrimSpace(m.ProcessDefinitionKey),
		TaskID:                strings.TrimSpace(m.TaskID),
		TaskName:              strings.TrimSpace(m.TaskName),
		ActivityID:            strings.TrimSpace(m.ActivityID),
		ActivityName:          strings.TrimSpace(m.ActivityName),
		BizID:                 strings.TrimSpace(m.BizID),
		BizType:               strings.TrimSpace(m.BizType),
		Title:                 strings.TrimSpace(m.Title),
		TenantID:              strings.TrimSpace(m.TenantID),
		SystemCode:            strings.TrimSpace(m.SystemCode),
		OperatorUserID:        strings.TrimSpace(m.OperatorUserID),
		OperatorUserName:      strings.TrimSpace(m.OperatorUserName),
		FromAssignee:          strings.TrimSpace(m.FromAssignee),
		ToAssignee:            strings.TrimSpace(m.ToAssignee),
		FromOwner:             strings.TrimSpace(m.FromOwner),
		ToOwner:               strings.TrimSpace(m.ToOwner),
		Comment:               strings.TrimSpace(m.Comment),
		Reason:                strings.TrimSpace(m.Reason),
		Source:                strings.TrimSpace(m.Source),
	}
}
