package flowable

import (
	stdcontext "context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/orm"
	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/types"
	redisv9 "github.com/redis/go-redis/v9"
)

const (
	defaultTodoProjectionTTL      = 604800
	defaultTodoTaskPrefix         = "workflow:todo-task:"
	defaultTodoUserPrefix         = "workflow:user-todo:"
	defaultTodoGroupPrefix        = "workflow:group-todo:"
	defaultProcessRuntimeTaskPref = "workflow:process-runtime-tasks:"
	configTodoProjectionTTL       = "workflow.flowable.todo_projection_ttl_seconds"
	configTodoTaskPrefix          = "workflow.flowable.todo_projection_task_prefix"
	configTodoUserPrefix          = "workflow.flowable.todo_projection_user_prefix"
	configTodoGroupPrefix         = "workflow.flowable.todo_projection_group_prefix"
	configProcessRuntimeTaskPref  = "workflow.flowable.process_runtime_task_prefix"
)

type runtimeTaskProjection struct {
	TaskID                string   `json:"taskId"`
	TaskName              string   `json:"taskName,omitempty"`
	ActivityID            string   `json:"activityId,omitempty"`
	ActivityName          string   `json:"activityName,omitempty"`
	ProcessInstanceID     string   `json:"processInstanceId,omitempty"`
	RootProcessInstanceID string   `json:"rootProcessInstanceId,omitempty"`
	ProcessDefinitionID   string   `json:"processDefinitionId,omitempty"`
	ProcessDefinitionKey  string   `json:"processDefinitionKey,omitempty"`
	BizID                 string   `json:"bizId,omitempty"`
	BizType               string   `json:"bizType,omitempty"`
	Title                 string   `json:"title,omitempty"`
	PayloadRef            string   `json:"payloadRef,omitempty"`
	CreatedAt             string   `json:"createdAt,omitempty"`
	Assignee              string   `json:"assignee,omitempty"`
	Owner                 string   `json:"owner,omitempty"`
	DelegationState       string   `json:"delegationState,omitempty"`
	TenantID              string   `json:"tenantId,omitempty"`
	SystemCode            string   `json:"systemCode,omitempty"`
	FormKey               string   `json:"formKey,omitempty"`
	CandidateUsers        []string `json:"candidateUsers,omitempty"`
	CandidateGroups       []string `json:"candidateGroups,omitempty"`
}

func (p runtimeTaskProjection) toTaskInfo() types.TaskInfo {
	return types.TaskInfo{
		TaskID:                p.TaskID,
		TaskName:              p.TaskName,
		ActivityID:            p.ActivityID,
		ActivityName:          p.ActivityName,
		ProcessInstanceID:     p.ProcessInstanceID,
		RootProcessInstanceID: p.RootProcessInstanceID,
		ProcessDefinitionID:   p.ProcessDefinitionID,
		BizID:                 p.BizID,
		BizType:               p.BizType,
		Title:                 p.Title,
		PayloadRef:            p.PayloadRef,
		CreatedAt:             p.CreatedAt,
		Assignee:              p.Assignee,
		Owner:                 p.Owner,
		DelegationState:       p.DelegationState,
		TenantID:              p.TenantID,
		FormKey:               p.FormKey,
	}
}

func (c *RESTClient) initTodoProjectionConfigFromConfig() {
	todoProjectionTTLSeconds := config.GetConfigInt(configTodoProjectionTTL)
	if todoProjectionTTLSeconds <= 0 {
		todoProjectionTTLSeconds = defaultTodoProjectionTTL
	}
	c.todoProjectionTTL = time.Duration(todoProjectionTTLSeconds) * time.Second
	c.todoTaskPrefix = firstNonBlank(config.GetConfigString(configTodoTaskPrefix), defaultTodoTaskPrefix)
	c.todoUserPrefix = firstNonBlank(config.GetConfigString(configTodoUserPrefix), defaultTodoUserPrefix)
	c.todoGroupPrefix = firstNonBlank(config.GetConfigString(configTodoGroupPrefix), defaultTodoGroupPrefix)
	c.processRuntimeTaskPrefix = firstNonBlank(config.GetConfigString(configProcessRuntimeTaskPref), defaultProcessRuntimeTaskPref)
}

func (c *RESTClient) listTodoFromProjection(ctx stdcontext.Context, user *workflowcontext.UserContext, query *types.TaskQuery) (*types.TaskPage, map[string]struct{}) {
	if orm.Redis == nil || c == nil || user == nil || strings.TrimSpace(user.UserID) == "" || query == nil || query.Start > 0 {
		return nil, nil
	}
	keys := c.todoProjectionLookupKeys(user)
	if len(keys) == 0 {
		return nil, nil
	}
	client := orm.Redis.GetClient()
	window := int64(max(1, query.Size) * 3)
	taskIDs := make([]string, 0, len(keys)*int(window))
	seenIDs := make(map[string]struct{})
	for _, key := range keys {
		items, err := client.ZRange(ctx, key, 0, window-1).Result()
		if err != nil {
			continue
		}
		for _, taskID := range items {
			taskID = strings.TrimSpace(taskID)
			if taskID == "" {
				continue
			}
			if _, ok := seenIDs[taskID]; ok {
				continue
			}
			seenIDs[taskID] = struct{}{}
			taskIDs = append(taskIDs, taskID)
		}
	}
	if len(taskIDs) == 0 {
		return nil, seenIDs
	}
	items := make([]types.TaskInfo, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		projection, ok := c.loadRuntimeTaskProjection(ctx, taskID)
		if !ok {
			continue
		}
		if !matchesRuntimeProjectionScope(projection, user) || !matchesRuntimeProjectionQuery(projection, query, user, c.groupPrefix, c.rolePrefix) {
			continue
		}
		items = append(items, projection.toTaskInfo())
	}
	sortTaskInfos(items, false)
	if len(items) == 0 {
		return nil, seenIDs
	}
	if len(items) > query.Size {
		items = items[:query.Size]
	}
	return &types.TaskPage{
		Items: items,
		Total: int64(len(items)),
		Start: query.Start,
		Size:  query.Size,
	}, seenIDs
}

func (c *RESTClient) syncRuntimeTaskProjection(ctx stdcontext.Context, taskID, processInstanceID string) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}
	task, err := c.getRuntimeTask(ctx, taskID)
	if err != nil {
		return
	}
	variables := task.ProcessVariables
	if len(variables) == 0 && strings.TrimSpace(processInstanceID) != "" {
		variables, _ = c.getProcessVariables(ctx, processInstanceID)
	}
	projection := c.buildRuntimeTaskProjection(task, variables)
	c.storeRuntimeTaskProjection(ctx, projection)
}

func (c *RESTClient) buildRuntimeTaskProjection(task runtimeTaskRecord, variables map[string]interface{}) runtimeTaskProjection {
	merged := make(map[string]interface{}, len(variables)+8)
	for key, value := range task.ProcessVariables {
		merged[key] = value
	}
	for key, value := range variables {
		merged[key] = value
	}
	rootProcessInstanceID := strings.TrimSpace(task.ProcessInstanceID)
	return runtimeTaskProjection{
		TaskID:                task.ID,
		TaskName:              task.Name,
		ActivityID:            task.TaskDefinitionKey,
		ActivityName:          firstNonBlank(task.Name, task.TaskDefinitionKey),
		ProcessInstanceID:     task.ProcessInstanceID,
		RootProcessInstanceID: rootProcessInstanceID,
		ProcessDefinitionID:   task.ProcessDefinitionID,
		ProcessDefinitionKey:  processDefinitionKey(task.ProcessDefinitionID),
		BizID:                 firstNonBlank(stringValue(merged["bizId"]), task.BusinessKey),
		BizType:               stringValue(merged["bizType"]),
		Title:                 stringValue(merged["title"]),
		PayloadRef:            firstNonBlank(stringValue(merged["payloadRef"]), stringValue(merged["bizId"]), task.BusinessKey),
		CreatedAt:             task.CreateTimeRaw,
		Assignee:              task.Assignee,
		Owner:                 task.Owner,
		DelegationState:       task.DelegationState,
		TenantID:              firstNonBlank(stringValue(merged["tenantId"]), task.TenantID),
		SystemCode:            stringValue(merged["systemCode"]),
		FormKey:               task.FormKey,
		CandidateUsers:        append([]string(nil), task.CandidateUsers...),
		CandidateGroups:       append([]string(nil), task.CandidateGroups...),
	}
}

func (c *RESTClient) storeRuntimeTaskProjection(ctx stdcontext.Context, projection runtimeTaskProjection) {
	if orm.Redis == nil || c == nil || c.todoProjectionTTL <= 0 || strings.TrimSpace(projection.TaskID) == "" {
		return
	}
	taskKey := c.runtimeTaskProjectionKey(projection.TaskID)
	if taskKey == "" {
		return
	}
	if _, ok := c.loadRuntimeTaskProjection(ctx, projection.TaskID); ok {
		c.removeRuntimeTaskProjection(ctx, projection.TaskID)
	}
	payload, err := json.Marshal(projection)
	if err != nil {
		return
	}
	score := float64(parseTime(projection.CreatedAt).UnixMilli())
	if score == 0 {
		score = float64(time.Now().UnixMilli())
	}
	client := orm.Redis.GetClient()
	_ = orm.Redis.Set(ctx, taskKey, string(payload), c.todoProjectionTTL)
	for _, key := range c.todoProjectionUserKeys(projection) {
		if key == "" {
			continue
		}
		_ = client.ZAdd(ctx, key, redisv9.Z{Score: score, Member: projection.TaskID}).Err()
		_ = client.Expire(ctx, key, c.todoProjectionTTL).Err()
	}
	for _, key := range c.todoProjectionGroupKeys(projection) {
		if key == "" {
			continue
		}
		_ = client.ZAdd(ctx, key, redisv9.Z{Score: score, Member: projection.TaskID}).Err()
		_ = client.Expire(ctx, key, c.todoProjectionTTL).Err()
	}
	processKey := c.processRuntimeTaskKey(projection.ProcessInstanceID)
	if processKey != "" {
		_ = client.SAdd(ctx, processKey, projection.TaskID).Err()
		_ = client.Expire(ctx, processKey, c.todoProjectionTTL).Err()
	}
	c.storeCandidateTaskIndex(ctx, projection.TaskID, projection.CandidateUsers, projection.CandidateGroups)
}

func (c *RESTClient) removeRuntimeTaskProjection(ctx stdcontext.Context, taskID string) {
	if orm.Redis == nil || c == nil {
		return
	}
	projection, ok := c.loadRuntimeTaskProjection(ctx, taskID)
	if !ok {
		return
	}
	client := orm.Redis.GetClient()
	for _, key := range c.todoProjectionUserKeys(projection) {
		if key != "" {
			_ = client.ZRem(ctx, key, projection.TaskID).Err()
		}
	}
	for _, key := range c.todoProjectionGroupKeys(projection) {
		if key != "" {
			_ = client.ZRem(ctx, key, projection.TaskID).Err()
		}
	}
	if processKey := c.processRuntimeTaskKey(projection.ProcessInstanceID); processKey != "" {
		_ = client.SRem(ctx, processKey, projection.TaskID).Err()
	}
	if taskKey := c.runtimeTaskProjectionKey(projection.TaskID); taskKey != "" {
		_, _ = orm.Redis.Del(ctx, taskKey)
	}
	c.invalidateCandidateTaskIndex(ctx, projection.TaskID)
}

func (c *RESTClient) clearProcessRuntimeTaskProjections(ctx stdcontext.Context, processInstanceID string) {
	if orm.Redis == nil || c == nil {
		return
	}
	key := c.processRuntimeTaskKey(processInstanceID)
	if key == "" {
		return
	}
	client := orm.Redis.GetClient()
	taskIDs, err := client.SMembers(ctx, key).Result()
	if err == nil {
		for _, taskID := range taskIDs {
			c.removeRuntimeTaskProjection(ctx, taskID)
		}
	}
	_ = client.Del(ctx, key).Err()
}

func (c *RESTClient) syncDoneProjectionByTaskID(ctx stdcontext.Context, taskID string) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}
	task, err := c.getHistoricTask(ctx, taskID)
	if err != nil {
		return
	}
	projection := doneTaskProjection{
		TaskID:               task.ID,
		TaskName:             task.Name,
		ActivityID:           task.TaskDefinitionKey,
		ActivityName:         firstNonBlank(task.Name, task.TaskDefinitionKey),
		ProcessInstanceID:    task.ProcessInstanceID,
		ProcessDefinitionID:  task.ProcessDefinitionID,
		ProcessDefinitionKey: processDefinitionKey(task.ProcessDefinitionID),
		BizID:                firstNonBlank(stringValue(task.ProcessVariables["bizId"]), task.BusinessKey),
		BizType:              stringValue(task.ProcessVariables["bizType"]),
		Title:                stringValue(task.ProcessVariables["title"]),
		PayloadRef:           firstNonBlank(stringValue(task.ProcessVariables["payloadRef"]), stringValue(task.ProcessVariables["bizId"]), task.BusinessKey),
		CreatedAt:            task.CreateTimeRaw,
		CompletedAt:          task.EndTimeRaw,
		Assignee:             task.Assignee,
		Owner:                task.Owner,
		TenantID:             firstNonBlank(stringValue(task.ProcessVariables["tenantId"]), task.TenantID),
		SystemCode:           stringValue(task.ProcessVariables["systemCode"]),
		FormKey:              task.FormKey,
	}
	c.storeDoneTaskProjection(ctx, projection)
}

func (c *RESTClient) getHistoricTask(ctx stdcontext.Context, taskID string) (historicTaskRecord, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/history/historic-task-instances/"+strings.TrimSpace(taskID), nil, nil)
	if err != nil {
		return historicTaskRecord{}, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return historicTaskRecord{}, err
	}
	record := historicTaskRecord{
		ID:                  stringValue(raw["id"]),
		Name:                stringValue(raw["name"]),
		Description:         stringValue(raw["description"]),
		TaskDefinitionKey:   stringValue(raw["taskDefinitionKey"]),
		Assignee:            stringValue(raw["assignee"]),
		Owner:               stringValue(raw["owner"]),
		ProcessInstanceID:   stringValue(raw["processInstanceId"]),
		ProcessDefinitionID: stringValue(raw["processDefinitionId"]),
		TenantID:            stringValue(raw["tenantId"]),
		FormKey:             stringValue(raw["formKey"]),
		BusinessKey:         firstNonBlank(stringValue(raw["businessKey"]), stringValue(raw["processBusinessKey"])),
		ProcessVariables:    parseVariableMap(raw["processVariables"]),
		StartTimeRaw:        firstNonBlank(stringValue(raw["startTime"]), stringValue(raw["createTime"])),
		CreateTimeRaw:       stringValue(raw["createTime"]),
		EndTimeRaw:          stringValue(raw["endTime"]),
		DurationInMillis:    int64Value(raw["durationInMillis"]),
	}
	if record.EndTimeRaw != "" {
		endTime := parseTime(record.EndTimeRaw)
		record.EndTime = &endTime
	}
	if len(record.ProcessVariables) == 0 && strings.TrimSpace(record.ProcessInstanceID) != "" {
		record.ProcessVariables, _ = c.getProcessVariables(ctx, record.ProcessInstanceID)
	}
	return record, nil
}

func (c *RESTClient) refreshProcessSummaryProjection(ctx stdcontext.Context, payload *types.FlowableCallbackPayload) {
	if c == nil || payload == nil || strings.TrimSpace(payload.ProcessInstanceID) == "" {
		return
	}
	view, err := c.GetProgressView(ctx, payload.ProcessInstanceID, nil)
	if err == nil && view != nil {
		return
	}
	summary := types.ProcessProgressSummary{
		ProcessInstanceID:    strings.TrimSpace(payload.ProcessInstanceID),
		ProcessDefinitionID:  strings.TrimSpace(payload.ProcessDefinitionID),
		ProcessDefinitionKey: processDefinitionKey(strings.TrimSpace(payload.ProcessDefinitionID)),
		BizID:                firstNonBlank(strings.TrimSpace(payload.BizID), stringValue(payload.Variables["bizId"]), stringValue(payload.Variables["payloadRef"])),
		BizType:              stringValue(payload.Variables["bizType"]),
		Title:                stringValue(payload.Variables["title"]),
		TenantID:             stringValue(payload.Variables["tenantId"]),
		SystemCode:           stringValue(payload.Variables["systemCode"]),
		Status:               "RUNNING",
		StartTime:            strings.TrimSpace(payload.EventTime),
	}
	if strings.EqualFold(strings.TrimSpace(payload.EventType), "PROCESS_ENDED") {
		summary.Status = "COMPLETED"
		summary.EndTime = strings.TrimSpace(payload.EventTime)
		summary.ProgressPercent = 100
	}
	c.storeProcessSummaryCache(ctx, &summary)
}

func (c *RESTClient) loadRuntimeTaskProjection(ctx stdcontext.Context, taskID string) (runtimeTaskProjection, bool) {
	if orm.Redis == nil || c == nil {
		return runtimeTaskProjection{}, false
	}
	key := c.runtimeTaskProjectionKey(taskID)
	if key == "" {
		return runtimeTaskProjection{}, false
	}
	value, err := orm.Redis.Get(ctx, key)
	if err != nil || strings.TrimSpace(value) == "" {
		return runtimeTaskProjection{}, false
	}
	var projection runtimeTaskProjection
	if err := json.Unmarshal([]byte(value), &projection); err != nil {
		return runtimeTaskProjection{}, false
	}
	if strings.TrimSpace(projection.TaskID) == "" {
		return runtimeTaskProjection{}, false
	}
	return projection, true
}

func (c *RESTClient) runtimeTaskProjectionKey(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if c == nil || taskID == "" {
		return ""
	}
	return firstNonBlank(c.todoTaskPrefix, defaultTodoTaskPrefix) + taskID
}

func (c *RESTClient) todoProjectionUserKey(userID string) string {
	userID = strings.TrimSpace(userID)
	if c == nil || userID == "" {
		return ""
	}
	return firstNonBlank(c.todoUserPrefix, defaultTodoUserPrefix) + userID
}

func (c *RESTClient) todoProjectionGroupKey(groupID string) string {
	groupID = strings.TrimSpace(groupID)
	if c == nil || groupID == "" {
		return ""
	}
	return firstNonBlank(c.todoGroupPrefix, defaultTodoGroupPrefix) + groupID
}

func (c *RESTClient) processRuntimeTaskKey(processInstanceID string) string {
	processInstanceID = strings.TrimSpace(processInstanceID)
	if c == nil || processInstanceID == "" {
		return ""
	}
	return firstNonBlank(c.processRuntimeTaskPrefix, defaultProcessRuntimeTaskPref) + processInstanceID
}

func (c *RESTClient) todoProjectionUserKeys(projection runtimeTaskProjection) []string {
	result := make([]string, 0, 1+len(projection.CandidateUsers))
	if key := c.todoProjectionUserKey(projection.Assignee); key != "" {
		result = append(result, key)
	}
	for _, userID := range projection.CandidateUsers {
		key := c.todoProjectionUserKey(userID)
		if key == "" {
			continue
		}
		seen := false
		for _, current := range result {
			if current == key {
				seen = true
				break
			}
		}
		if !seen {
			result = append(result, key)
		}
	}
	return result
}

func (c *RESTClient) todoProjectionGroupKeys(projection runtimeTaskProjection) []string {
	result := make([]string, 0, len(projection.CandidateGroups))
	for _, groupID := range projection.CandidateGroups {
		key := c.todoProjectionGroupKey(groupID)
		if key == "" {
			continue
		}
		seen := false
		for _, current := range result {
			if current == key {
				seen = true
				break
			}
		}
		if !seen {
			result = append(result, key)
		}
	}
	return result
}

func (c *RESTClient) todoProjectionLookupKeys(user *workflowcontext.UserContext) []string {
	if c == nil || user == nil {
		return nil
	}
	result := make([]string, 0, 1+len(user.Groups)+len(user.Roles))
	if key := c.todoProjectionUserKey(user.UserID); key != "" {
		result = append(result, key)
	}
	for _, groupID := range candidateGroups(user, c.groupPrefix, c.rolePrefix) {
		key := c.todoProjectionGroupKey(groupID)
		if key == "" {
			continue
		}
		seen := false
		for _, current := range result {
			if current == key {
				seen = true
				break
			}
		}
		if !seen {
			result = append(result, key)
		}
	}
	return result
}

func matchesRuntimeProjectionScope(projection runtimeTaskProjection, user *workflowcontext.UserContext) bool {
	if user == nil {
		return false
	}
	userTenantID := strings.TrimSpace(user.TenantID)
	projectionTenantID := strings.TrimSpace(projection.TenantID)
	if userTenantID != "" && projectionTenantID != "" && projectionTenantID != userTenantID {
		return false
	}
	if strings.TrimSpace(user.SystemCode) != "" && strings.TrimSpace(projection.SystemCode) != "" && strings.TrimSpace(projection.SystemCode) != strings.TrimSpace(user.SystemCode) {
		return false
	}
	return true
}

func matchesRuntimeProjectionQuery(projection runtimeTaskProjection, query *types.TaskQuery, user *workflowcontext.UserContext, groupPrefix, rolePrefix string) bool {
	if strings.TrimSpace(projection.Assignee) != "" && strings.TrimSpace(projection.Assignee) != strings.TrimSpace(user.UserID) {
		return false
	}
	if strings.TrimSpace(projection.Assignee) == "" {
		userID := strings.TrimSpace(user.UserID)
		groupSet := make(map[string]struct{})
		for _, group := range candidateGroups(user, groupPrefix, rolePrefix) {
			groupSet[strings.TrimSpace(group)] = struct{}{}
		}
		allowed := false
		for _, candidateUser := range projection.CandidateUsers {
			if strings.TrimSpace(candidateUser) == userID {
				allowed = true
				break
			}
		}
		if !allowed {
			for _, candidateGroup := range projection.CandidateGroups {
				if _, ok := groupSet[strings.TrimSpace(candidateGroup)]; ok {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			return false
		}
	}
	return matchesTaskQuery(projection.toTaskInfo(), query) &&
		(query.ProcessDefinitionKey == "" || strings.EqualFold(strings.TrimSpace(projection.ProcessDefinitionKey), query.ProcessDefinitionKey)) &&
		(query.ActivityID == "" || strings.EqualFold(strings.TrimSpace(projection.ActivityID), query.ActivityID))
}
