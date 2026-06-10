package flowable

import (
	stdcontext "context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/orm"
	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/types"
)

const (
	defaultActionTimelineCacheTTLSeconds = 60
	defaultActionTimelineCachePrefix     = "workflow:process-action-timeline:"
	configActionTimelineCacheTTLSeconds  = "workflow.flowable.action_timeline_cache_ttl_seconds"
	configActionTimelineCachePrefix      = "workflow.flowable.action_timeline_cache_prefix"
	workflowActionCommentPrefix          = "[WF_ACTION]"
	legacyTransferCommentPrefix          = "[TRANSFER]"
	workflowActionCommentVersion         = 1
)

type workflowActionComment struct {
	V                 int    `json:"v"`
	Action            string `json:"action,omitempty"`
	OperatorUserID    string `json:"operatorUserId,omitempty"`
	OperatorUserName  string `json:"operatorUserName,omitempty"`
	ProcessInstanceID string `json:"processInstanceId,omitempty"`
	TaskID            string `json:"taskId,omitempty"`
	TaskName          string `json:"taskName,omitempty"`
	ActivityID        string `json:"activityId,omitempty"`
	ActivityName      string `json:"activityName,omitempty"`
	FromAssignee      string `json:"fromAssignee,omitempty"`
	ToAssignee        string `json:"toAssignee,omitempty"`
	FromOwner         string `json:"fromOwner,omitempty"`
	ToOwner           string `json:"toOwner,omitempty"`
	Reason            string `json:"reason,omitempty"`
}

type historicCommentRecord struct {
	ID                string
	ProcessInstanceID string
	TaskID            string
	UserID            string
	Type              string
	Action            string
	Message           string
	FullMessage       string
	Time              time.Time
	TimeRaw           string
}

type taskActionMetadata struct {
	ProcessInstanceID string
	TaskID            string
	TaskName          string
	ActivityID        string
	ActivityName      string
}

type parsedActionComment struct {
	comment historicCommentRecord
	payload workflowActionComment
	source  string
}

const (
	actionCommentSourceWorkflow       = "workflow"
	actionCommentSourceLegacyTransfer = "legacy_transfer"
)

func (c *RESTClient) GetActionTimeline(ctx stdcontext.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessActionTimelineResponse, error) {
	summaryProcessInstanceID, err := c.resolveSummaryProcessInstanceID(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	if cached, ok := c.loadActionTimelineCache(ctx, summaryProcessInstanceID); ok {
		return cached, nil
	}
	summary, err := c.getProcessSummary(ctx, summaryProcessInstanceID, user)
	if err != nil {
		return nil, err
	}
	scope, err := c.collectProcessScope(ctx, summaryProcessInstanceID)
	if err != nil {
		return nil, err
	}
	taskMeta, _ := c.loadTaskActionMetadata(ctx, scope)
	items := c.buildActionTimelineItems(ctx, summaryProcessInstanceID, taskMeta)
	response := &types.ProcessActionTimelineResponse{
		Summary: summary,
		Items:   items,
	}
	normalizeActionTimelineResponse(response)
	c.storeActionTimelineCache(ctx, response)
	return response, nil
}

func (c *RESTClient) GetActionTimelineByBizID(ctx stdcontext.Context, bizID string, user *workflowcontext.UserContext) (*types.ProcessActionTimelineResponse, error) {
	processInstanceID, err := c.ResolveProcessInstanceIDByBizID(ctx, bizID, user)
	if err != nil {
		return nil, err
	}
	return c.GetActionTimeline(ctx, processInstanceID, user)
}

func (c *RESTClient) buildActionTimelineItems(ctx stdcontext.Context, rootProcessInstanceID string, taskMeta map[string]taskActionMetadata) []types.ProcessActionTimelineItem {
	items := mergeActionTimelineItems(
		c.buildActionTimelineCommentItems(ctx, rootProcessInstanceID, taskMeta),
		c.buildActionTimelineTaskRecordItems(ctx, rootProcessInstanceID),
	)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Time == items[j].Time {
			return items[i].CommentID < items[j].CommentID
		}
		return items[i].Time < items[j].Time
	})
	for index := range items {
		items[index].Sequence = index + 1
	}
	return items
}

func (c *RESTClient) buildActionTimelineCommentItems(ctx stdcontext.Context, rootProcessInstanceID string, taskMeta map[string]taskActionMetadata) []types.ProcessActionTimelineItem {
	comments := c.queryTaskActionComments(ctx, taskMeta)
	parsedComments := make([]parsedActionComment, 0, len(comments))
	workflowTransferKeys := make(map[string]struct{}, len(comments))
	for _, comment := range comments {
		payload, source, ok := parseActionCommentPayload(comment)
		if !ok {
			continue
		}
		parsedComments = append(parsedComments, parsedActionComment{
			comment: comment,
			payload: payload,
			source:  source,
		})
		if source == actionCommentSourceWorkflow && strings.EqualFold(strings.TrimSpace(payload.Action), types.TaskActionTypeTransfer) {
			workflowTransferKeys[buildTransferActionDedupKey(firstNonBlank(payload.TaskID, comment.TaskID), payload)] = struct{}{}
		}
	}
	items := make([]types.ProcessActionTimelineItem, 0, len(parsedComments))
	for _, parsed := range parsedComments {
		if parsed.source == actionCommentSourceLegacyTransfer {
			key := buildTransferActionDedupKey(firstNonBlank(parsed.payload.TaskID, parsed.comment.TaskID), parsed.payload)
			if _, exists := workflowTransferKeys[key]; exists {
				continue
			}
		}
		item := buildActionTimelineItem(rootProcessInstanceID, parsed.comment, parsed.payload, taskMeta)
		if strings.TrimSpace(item.ActionType) == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}

func (c *RESTClient) buildActionTimelineTaskRecordItems(ctx stdcontext.Context, rootProcessInstanceID string) []types.ProcessActionTimelineItem {
	records, err := c.loadTaskRecordItemsByRootProcessInstanceID(ctx, rootProcessInstanceID)
	if err != nil {
		return []types.ProcessActionTimelineItem{}
	}
	items := make([]types.ProcessActionTimelineItem, 0, len(records))
	for _, record := range records {
		item, ok := taskRecordToActionTimelineItem(record)
		if !ok {
			continue
		}
		items = append(items, item)
	}
	return items
}

func parseActionCommentPayload(comment historicCommentRecord) (workflowActionComment, string, bool) {
	payload, ok := parseWorkflowActionComment(comment.FullMessage)
	if ok {
		return payload, actionCommentSourceWorkflow, true
	}
	payload, ok = parseWorkflowActionComment(comment.Message)
	if ok {
		return payload, actionCommentSourceWorkflow, true
	}
	payload, ok = parseLegacyTransferComment(comment.FullMessage)
	if ok {
		return payload, actionCommentSourceLegacyTransfer, true
	}
	payload, ok = parseLegacyTransferComment(comment.Message)
	if ok {
		return payload, actionCommentSourceLegacyTransfer, true
	}
	return workflowActionComment{}, "", false
}

func buildActionTimelineItem(rootProcessInstanceID string, comment historicCommentRecord, payload workflowActionComment, taskMeta map[string]taskActionMetadata) types.ProcessActionTimelineItem {
	meta := taskMeta[strings.TrimSpace(firstNonBlank(payload.TaskID, comment.TaskID))]
	processInstanceID := firstNonBlank(comment.ProcessInstanceID, payload.ProcessInstanceID, meta.ProcessInstanceID)
	taskID := firstNonBlank(payload.TaskID, comment.TaskID, meta.TaskID)
	return types.ProcessActionTimelineItem{
		ItemType:              types.ProcessActionTimelineItemTypeTaskAction,
		ActionType:            strings.ToUpper(strings.TrimSpace(payload.Action)),
		Time:                  firstNonBlank(comment.TimeRaw),
		ProcessInstanceID:     processInstanceID,
		RootProcessInstanceID: strings.TrimSpace(rootProcessInstanceID),
		TaskID:                taskID,
		TaskName:              firstNonBlank(payload.TaskName, meta.TaskName),
		ActivityID:            firstNonBlank(payload.ActivityID, meta.ActivityID),
		ActivityName:          firstNonBlank(payload.ActivityName, meta.ActivityName),
		OperatorUserID:        firstNonBlank(payload.OperatorUserID, comment.UserID),
		OperatorUserName:      strings.TrimSpace(payload.OperatorUserName),
		FromAssignee:          strings.TrimSpace(payload.FromAssignee),
		ToAssignee:            strings.TrimSpace(payload.ToAssignee),
		FromOwner:             strings.TrimSpace(payload.FromOwner),
		ToOwner:               strings.TrimSpace(payload.ToOwner),
		Reason:                strings.TrimSpace(payload.Reason),
		CommentID:             strings.TrimSpace(comment.ID),
	}
}

func taskRecordToActionTimelineItem(record types.WorkflowTaskRecordItem) (types.ProcessActionTimelineItem, bool) {
	actionType := strings.ToUpper(strings.TrimSpace(record.ActionType))
	if actionType == "" || actionType == types.TaskActionTypeStartProcess {
		return types.ProcessActionTimelineItem{}, false
	}
	return types.ProcessActionTimelineItem{
		ItemType:              types.ProcessActionTimelineItemTypeTaskAction,
		ActionType:            actionType,
		Time:                  strings.TrimSpace(record.Time),
		ProcessInstanceID:     strings.TrimSpace(record.ProcessInstanceID),
		RootProcessInstanceID: strings.TrimSpace(record.RootProcessInstanceID),
		TaskID:                strings.TrimSpace(record.TaskID),
		TaskName:              strings.TrimSpace(record.TaskName),
		ActivityID:            strings.TrimSpace(record.ActivityID),
		ActivityName:          strings.TrimSpace(record.ActivityName),
		OperatorUserID:        strings.TrimSpace(record.OperatorUserID),
		OperatorUserName:      strings.TrimSpace(record.OperatorUserName),
		FromAssignee:          strings.TrimSpace(record.FromAssignee),
		ToAssignee:            strings.TrimSpace(record.ToAssignee),
		FromOwner:             strings.TrimSpace(record.FromOwner),
		ToOwner:               strings.TrimSpace(record.ToOwner),
		Reason:                strings.TrimSpace(record.Reason),
		CommentID:             fmt.Sprintf("record:%d", record.RecordID),
	}, true
}

func mergeActionTimelineItems(commentItems, recordItems []types.ProcessActionTimelineItem) []types.ProcessActionTimelineItem {
	items := make([]types.ProcessActionTimelineItem, 0, len(commentItems)+len(recordItems))
	items = append(items, commentItems...)
	for _, recordItem := range recordItems {
		if containsEquivalentActionTimelineItem(items, recordItem) {
			continue
		}
		items = append(items, recordItem)
	}
	return items
}

func containsEquivalentActionTimelineItem(items []types.ProcessActionTimelineItem, target types.ProcessActionTimelineItem) bool {
	for _, item := range items {
		if actionTimelineItemsEquivalent(item, target) {
			return true
		}
	}
	return false
}

func actionTimelineItemsEquivalent(left, right types.ProcessActionTimelineItem) bool {
	if strings.ToUpper(strings.TrimSpace(left.ActionType)) != strings.ToUpper(strings.TrimSpace(right.ActionType)) {
		return false
	}
	if strings.TrimSpace(left.TaskID) != strings.TrimSpace(right.TaskID) {
		return false
	}
	if strings.TrimSpace(left.OperatorUserID) != strings.TrimSpace(right.OperatorUserID) {
		return false
	}
	if strings.TrimSpace(left.FromAssignee) != strings.TrimSpace(right.FromAssignee) {
		return false
	}
	if strings.TrimSpace(left.ToAssignee) != strings.TrimSpace(right.ToAssignee) {
		return false
	}
	if strings.TrimSpace(left.FromOwner) != strings.TrimSpace(right.FromOwner) {
		return false
	}
	if strings.TrimSpace(left.ToOwner) != strings.TrimSpace(right.ToOwner) {
		return false
	}
	if strings.TrimSpace(left.Reason) != strings.TrimSpace(right.Reason) {
		return false
	}
	leftTime := parseTime(strings.TrimSpace(left.Time))
	rightTime := parseTime(strings.TrimSpace(right.Time))
	if leftTime.IsZero() || rightTime.IsZero() {
		return true
	}
	delta := leftTime.Sub(rightTime)
	if delta < 0 {
		delta = -delta
	}
	return delta <= 5*time.Second
}

func buildTransferActionDedupKey(taskID string, payload workflowActionComment) string {
	return strings.Join([]string{
		strings.TrimSpace(taskID),
		strings.ToUpper(strings.TrimSpace(payload.Action)),
		strings.TrimSpace(payload.FromAssignee),
		strings.TrimSpace(payload.ToAssignee),
		strings.TrimSpace(payload.Reason),
	}, "|")
}

func (c *RESTClient) queryTaskActionComments(ctx stdcontext.Context, taskMeta map[string]taskActionMetadata) []historicCommentRecord {
	if len(taskMeta) == 0 {
		return []historicCommentRecord{}
	}
	taskIDs := make([]string, 0, len(taskMeta))
	for taskID := range taskMeta {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			continue
		}
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	comments := make([]historicCommentRecord, 0, len(taskIDs)*2)
	seen := make(map[string]struct{}, len(taskIDs)*2)
	for _, taskID := range taskIDs {
		rows, err := c.queryTaskComments(ctx, taskID)
		if err != nil {
			continue
		}
		for _, row := range rows {
			commentID := strings.TrimSpace(row.ID)
			if commentID != "" {
				if _, exists := seen[commentID]; exists {
					continue
				}
				seen[commentID] = struct{}{}
			}
			comments = append(comments, row)
		}
	}
	return comments
}

func (c *RESTClient) queryTaskComments(ctx stdcontext.Context, taskID string) ([]historicCommentRecord, error) {
	body, err := c.doJSON(ctx, "GET", "/runtime/tasks/"+strings.TrimSpace(taskID)+"/comments", nil, nil)
	if err != nil {
		return nil, err
	}
	return parseHistoricComments(body)
}

func (c *RESTClient) collectProcessScope(ctx stdcontext.Context, processInstanceID string) ([]processInstanceRecord, error) {
	rootProcess, err := c.getProcessInstance(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	rootProcess, err = c.resolveRootProcessInstance(ctx, rootProcess)
	if err != nil {
		return nil, err
	}
	queue := []processInstanceRecord{rootProcess}
	visited := map[string]processInstanceRecord{
		strings.TrimSpace(rootProcess.ID): rootProcess,
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		children, err := c.listChildProcessInstances(ctx, current.ID, current.TenantID)
		if err != nil {
			continue
		}
		for _, child := range children {
			childID := strings.TrimSpace(child.ID)
			if childID == "" {
				continue
			}
			if _, exists := visited[childID]; exists {
				continue
			}
			visited[childID] = child
			queue = append(queue, child)
		}
	}
	result := make([]processInstanceRecord, 0, len(visited))
	for _, process := range visited {
		result = append(result, process)
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i].StartTime
		right := result[j].StartTime
		if left.Equal(right) {
			return result[i].ID < result[j].ID
		}
		if left.IsZero() {
			return false
		}
		if right.IsZero() {
			return true
		}
		return left.Before(right)
	})
	return result, nil
}

func (c *RESTClient) listChildProcessInstances(ctx stdcontext.Context, parentProcessInstanceID, tenantID string) ([]processInstanceRecord, error) {
	query := map[string]string{
		"superProcessInstanceId": strings.TrimSpace(parentProcessInstanceID),
		"size":                   "200",
		"sort":                   "startTime",
		"order":                  "asc",
	}
	if strings.TrimSpace(tenantID) != "" {
		query["tenantId"] = strings.TrimSpace(tenantID)
	}
	result := make([]processInstanceRecord, 0, 8)
	seen := map[string]struct{}{}
	appendItems := func(items []processInstanceRecord) {
		for _, item := range items {
			processID := strings.TrimSpace(item.ID)
			if processID == "" {
				continue
			}
			if _, ok := seen[processID]; ok {
				continue
			}
			seen[processID] = struct{}{}
			result = append(result, item)
		}
	}
	runtimeItems, runtimeErr := c.queryProcessInstances(ctx, "/runtime/process-instances", query)
	if runtimeErr == nil {
		appendItems(runtimeItems)
	}
	historicItems, historicErr := c.queryProcessInstances(ctx, "/history/historic-process-instances", query)
	if historicErr == nil {
		appendItems(historicItems)
	}
	if runtimeErr != nil && historicErr != nil {
		return nil, runtimeErr
	}
	return result, nil
}

func (c *RESTClient) loadTaskActionMetadata(ctx stdcontext.Context, scope []processInstanceRecord) (map[string]taskActionMetadata, error) {
	result := make(map[string]taskActionMetadata)
	for _, process := range scope {
		runtimeTasks, err := c.queryRuntimeTasks(ctx, map[string]string{
			"processInstanceId": strings.TrimSpace(process.ID),
			"size":              "200",
			"sort":              "createTime",
			"order":             "asc",
		})
		if err == nil {
			for _, task := range runtimeTasks {
				result[strings.TrimSpace(task.ID)] = taskActionMetadata{
					ProcessInstanceID: strings.TrimSpace(task.ProcessInstanceID),
					TaskID:            strings.TrimSpace(task.ID),
					TaskName:          strings.TrimSpace(task.Name),
					ActivityID:        strings.TrimSpace(task.TaskDefinitionKey),
					ActivityName:      firstNonBlank(strings.TrimSpace(task.Name), strings.TrimSpace(task.TaskDefinitionKey)),
				}
			}
		}
		historicTasks, err := c.queryHistoricTasks(ctx, map[string]string{
			"processInstanceId": strings.TrimSpace(process.ID),
			"size":              "200",
			"sort":              "startTime",
			"order":             "asc",
		})
		if err != nil {
			continue
		}
		for _, task := range historicTasks {
			taskID := strings.TrimSpace(task.ID)
			if taskID == "" {
				continue
			}
			if _, exists := result[taskID]; exists {
				continue
			}
			result[taskID] = taskActionMetadata{
				ProcessInstanceID: strings.TrimSpace(task.ProcessInstanceID),
				TaskID:            taskID,
				TaskName:          strings.TrimSpace(task.Name),
				ActivityID:        strings.TrimSpace(task.TaskDefinitionKey),
				ActivityName:      firstNonBlank(strings.TrimSpace(task.Name), strings.TrimSpace(task.TaskDefinitionKey)),
			}
		}
	}
	return result, nil
}

func parseWorkflowActionComment(message string) (workflowActionComment, bool) {
	message = strings.TrimSpace(message)
	if !strings.HasPrefix(message, workflowActionCommentPrefix) {
		return workflowActionComment{}, false
	}
	var payload workflowActionComment
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(message, workflowActionCommentPrefix))), &payload); err != nil {
		return workflowActionComment{}, false
	}
	if strings.TrimSpace(payload.Action) == "" {
		return workflowActionComment{}, false
	}
	return payload, true
}

func parseLegacyTransferComment(message string) (workflowActionComment, bool) {
	message = strings.TrimSpace(message)
	if !strings.HasPrefix(message, legacyTransferCommentPrefix) {
		return workflowActionComment{}, false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(message, legacyTransferCommentPrefix))
	reason := ""
	if index := strings.Index(rest, " reason="); index >= 0 {
		reason = strings.TrimSpace(rest[index+len(" reason="):])
		rest = strings.TrimSpace(rest[:index])
	}
	parts := strings.Fields(rest)
	payload := workflowActionComment{
		V:      workflowActionCommentVersion,
		Action: types.TaskActionTypeTransfer,
		Reason: reason,
	}
	for _, part := range parts {
		switch {
		case strings.HasPrefix(part, "from="):
			payload.FromAssignee = strings.TrimSpace(strings.TrimPrefix(part, "from="))
			payload.OperatorUserID = payload.FromAssignee
		case strings.HasPrefix(part, "to="):
			payload.ToAssignee = strings.TrimSpace(strings.TrimPrefix(part, "to="))
		}
	}
	if payload.FromAssignee == "" || payload.ToAssignee == "" {
		return workflowActionComment{}, false
	}
	return payload, true
}

func buildWorkflowActionComment(actionType string, beforeTask, afterTask runtimeTaskRecord, user *workflowcontext.UserContext, reason string) string {
	payload := workflowActionComment{
		V:                 workflowActionCommentVersion,
		Action:            strings.ToUpper(strings.TrimSpace(actionType)),
		OperatorUserID:    "",
		OperatorUserName:  "",
		ProcessInstanceID: firstNonBlank(afterTask.ProcessInstanceID, beforeTask.ProcessInstanceID),
		TaskID:            firstNonBlank(afterTask.ID, beforeTask.ID),
		TaskName:          firstNonBlank(afterTask.Name, beforeTask.Name),
		ActivityID:        firstNonBlank(afterTask.TaskDefinitionKey, beforeTask.TaskDefinitionKey),
		ActivityName:      firstNonBlank(afterTask.Name, beforeTask.Name, afterTask.TaskDefinitionKey, beforeTask.TaskDefinitionKey),
		FromAssignee:      strings.TrimSpace(beforeTask.Assignee),
		ToAssignee:        strings.TrimSpace(afterTask.Assignee),
		FromOwner:         strings.TrimSpace(beforeTask.Owner),
		ToOwner:           strings.TrimSpace(afterTask.Owner),
		Reason:            strings.TrimSpace(reason),
	}
	if user != nil {
		payload.OperatorUserID = strings.TrimSpace(user.UserID)
		payload.OperatorUserName = strings.TrimSpace(user.UserName)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return workflowActionCommentPrefix + string(data)
}

func (c *RESTClient) writeWorkflowActionComment(ctx stdcontext.Context, actionType string, beforeTask, afterTask runtimeTaskRecord, user *workflowcontext.UserContext, reason string) {
	message := buildWorkflowActionComment(actionType, beforeTask, afterTask, user, reason)
	if strings.TrimSpace(message) == "" {
		return
	}
	if err := c.addTaskCommentWithRetry(ctx, firstNonBlank(afterTask.ID, beforeTask.ID), message); err != nil {
		log.Warnf("workflow action comment write failed | action=%s | task=%s | err=%v", actionType, firstNonBlank(afterTask.ID, beforeTask.ID), err)
	}
}

func (c *RESTClient) addTaskCommentWithRetry(ctx stdcontext.Context, taskID, message string) error {
	err := c.addTaskComment(ctx, taskID, message)
	if err == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(120 * time.Millisecond):
	}
	return c.addTaskComment(ctx, taskID, message)
}

func parseHistoricComments(body []byte) ([]historicCommentRecord, error) {
	data, err := extractDataArray(body)
	if err != nil {
		return nil, err
	}
	result := make([]historicCommentRecord, 0, len(data))
	for _, row := range data {
		record := historicCommentRecord{
			ID:                stringValue(row["id"]),
			ProcessInstanceID: stringValue(row["processInstanceId"]),
			TaskID:            stringValue(row["taskId"]),
			UserID:            firstNonBlank(stringValue(row["userId"]), stringValue(row["author"])),
			Type:              stringValue(row["type"]),
			Action:            stringValue(row["action"]),
			Message:           stringValue(row["message"]),
			FullMessage:       firstNonBlank(stringValue(row["fullMessage"]), stringValue(row["message"])),
			TimeRaw:           stringValue(row["time"]),
		}
		record.Time = parseTime(record.TimeRaw)
		result = append(result, record)
	}
	return result, nil
}

func (c *RESTClient) loadActionTimelineCache(ctx stdcontext.Context, processInstanceID string) (*types.ProcessActionTimelineResponse, bool) {
	if orm.Redis == nil || c == nil || c.actionTimelineCacheTTL <= 0 {
		return nil, false
	}
	key := c.actionTimelineCacheKey(processInstanceID)
	if key == "" {
		return nil, false
	}
	value, err := orm.Redis.Get(ctx, key)
	if err != nil || strings.TrimSpace(value) == "" {
		return nil, false
	}
	var response types.ProcessActionTimelineResponse
	if err := json.Unmarshal([]byte(value), &response); err != nil {
		return nil, false
	}
	if strings.TrimSpace(response.Summary.ProcessInstanceID) == "" {
		return nil, false
	}
	normalizeActionTimelineResponse(&response)
	return &response, true
}

func (c *RESTClient) storeActionTimelineCache(ctx stdcontext.Context, response *types.ProcessActionTimelineResponse) {
	if orm.Redis == nil || c == nil || response == nil || c.actionTimelineCacheTTL <= 0 {
		return
	}
	normalizeActionTimelineResponse(response)
	key := c.actionTimelineCacheKey(response.Summary.ProcessInstanceID)
	if key == "" {
		return
	}
	payload, err := json.Marshal(response)
	if err != nil {
		return
	}
	_ = orm.Redis.Set(ctx, key, string(payload), c.actionTimelineCacheTTL)
}

func normalizeActionTimelineResponse(response *types.ProcessActionTimelineResponse) {
	if response == nil {
		return
	}
	if response.Items == nil {
		response.Items = make([]types.ProcessActionTimelineItem, 0)
	}
}

func (c *RESTClient) invalidateActionTimelineCache(ctx stdcontext.Context, processInstanceID string) {
	if orm.Redis == nil || c == nil {
		return
	}
	summaryProcessInstanceID, err := c.resolveSummaryProcessInstanceID(ctx, processInstanceID)
	if err != nil {
		summaryProcessInstanceID = strings.TrimSpace(processInstanceID)
	}
	key := c.actionTimelineCacheKey(summaryProcessInstanceID)
	if key == "" {
		return
	}
	_, _ = orm.Redis.Del(ctx, key)
}

func (c *RESTClient) actionTimelineCacheKey(processInstanceID string) string {
	processInstanceID = strings.TrimSpace(processInstanceID)
	if c == nil || processInstanceID == "" {
		return ""
	}
	return firstNonBlank(c.actionTimelineCachePrefix, defaultActionTimelineCachePrefix) + processInstanceID
}

func (c *RESTClient) recordWorkflowAction(ctx stdcontext.Context, actionType string, beforeTask, afterTask runtimeTaskRecord, user *workflowcontext.UserContext, reason string) {
	c.writeWorkflowActionComment(ctx, actionType, beforeTask, afterTask, user, reason)
	c.invalidateActionTimelineCache(ctx, firstNonBlank(afterTask.ProcessInstanceID, beforeTask.ProcessInstanceID))
}

func formatActionCommentError(actionType, taskID string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("workflow action comment write failed | action=%s | task=%s | err=%w", actionType, taskID, err)
}
