package flowable

import (
	stdcontext "context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/orm"
	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/identity"
	"github.com/goodbye-jack/go-common/workflow/types"
	redisv9 "github.com/redis/go-redis/v9"
)

const (
	defaultTimeoutSeconds         = 15
	defaultTaskPageSize           = 20
	maxTaskPageSize               = 100
	defaultSummaryCacheTTLSeconds = 120
	defaultSummaryCachePrefix     = "workflow:process-summary:"
	defaultCandidateIndexTTL      = 300
	defaultCandidateIndexPrefix   = "workflow:task-candidate:"
	defaultDoneProjectionTTL      = 2592000
	defaultDoneTaskPrefix         = "workflow:done-task:"
	defaultDoneUserPrefix         = "workflow:user-done:"
	configBaseURL                 = "workflow.flowable.base_url"
	configUsername                = "workflow.flowable.username"
	configPassword                = "workflow.flowable.password"
	configTimeoutSeconds          = "workflow.flowable.timeout_seconds"
	configGroupPrefix             = "workflow.flowable.group_prefix"
	configRolePrefix              = "workflow.flowable.role_prefix"
	configSummaryCacheTTLSeconds  = "workflow.flowable.summary_cache_ttl_seconds"
	configSummaryCachePrefix      = "workflow.flowable.summary_cache_prefix"
	configCandidateIndexTTL       = "workflow.flowable.candidate_index_ttl_seconds"
	configCandidateIndexPrefix    = "workflow.flowable.candidate_index_prefix"
	configDoneProjectionTTL       = "workflow.flowable.done_projection_ttl_seconds"
	configDoneTaskPrefix          = "workflow.flowable.done_projection_task_prefix"
	configDoneUserPrefix          = "workflow.flowable.done_projection_user_prefix"
)

type RESTClient struct {
	baseURL                   string
	username                  string
	password                  string
	groupPrefix               string
	rolePrefix                string
	summaryCacheTTL           time.Duration
	summaryCachePrefix        string
	actionTimelineCacheTTL    time.Duration
	actionTimelineCachePrefix string
	candidateIndexTTL         time.Duration
	candidateIndexKey         string
	doneProjectionTTL         time.Duration
	doneTaskPrefix            string
	doneUserPrefix            string
	todoProjectionTTL         time.Duration
	todoTaskPrefix            string
	todoUserPrefix            string
	todoGroupPrefix           string
	processRuntimeTaskPrefix  string
	httpClient                *http.Client
}

func NewRESTClient(cfg Config) (*RESTClient, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil, errors.New("flowable base url is required")
	}
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = defaultTimeoutSeconds
	}
	return &RESTClient{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: strings.TrimSpace(cfg.Username),
		password: cfg.Password,
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}, nil
}

func NewRESTClientFromConfig() (*RESTClient, error) {
	cfg := Config{
		BaseURL:        config.GetConfigString(configBaseURL),
		Username:       config.GetConfigString(configUsername),
		Password:       config.GetConfigString(configPassword),
		TimeoutSeconds: config.GetConfigInt(configTimeoutSeconds),
	}
	client, err := NewRESTClient(cfg)
	if err != nil {
		return nil, err
	}
	client.groupPrefix = config.GetConfigString(configGroupPrefix)
	client.rolePrefix = config.GetConfigString(configRolePrefix)
	cacheTTLSeconds := config.GetConfigInt(configSummaryCacheTTLSeconds)
	if cacheTTLSeconds <= 0 {
		cacheTTLSeconds = defaultSummaryCacheTTLSeconds
	}
	client.summaryCacheTTL = time.Duration(cacheTTLSeconds) * time.Second
	client.summaryCachePrefix = firstNonBlank(config.GetConfigString(configSummaryCachePrefix), defaultSummaryCachePrefix)
	actionTimelineTTLSeconds := config.GetConfigInt(configActionTimelineCacheTTLSeconds)
	if actionTimelineTTLSeconds <= 0 {
		actionTimelineTTLSeconds = defaultActionTimelineCacheTTLSeconds
	}
	client.actionTimelineCacheTTL = time.Duration(actionTimelineTTLSeconds) * time.Second
	client.actionTimelineCachePrefix = firstNonBlank(config.GetConfigString(configActionTimelineCachePrefix), defaultActionTimelineCachePrefix)
	candidateTTLSeconds := config.GetConfigInt(configCandidateIndexTTL)
	if candidateTTLSeconds <= 0 {
		candidateTTLSeconds = defaultCandidateIndexTTL
	}
	client.candidateIndexTTL = time.Duration(candidateTTLSeconds) * time.Second
	client.candidateIndexKey = firstNonBlank(config.GetConfigString(configCandidateIndexPrefix), defaultCandidateIndexPrefix)
	doneProjectionTTLSeconds := config.GetConfigInt(configDoneProjectionTTL)
	if doneProjectionTTLSeconds <= 0 {
		doneProjectionTTLSeconds = defaultDoneProjectionTTL
	}
	client.doneProjectionTTL = time.Duration(doneProjectionTTLSeconds) * time.Second
	client.doneTaskPrefix = firstNonBlank(config.GetConfigString(configDoneTaskPrefix), defaultDoneTaskPrefix)
	client.doneUserPrefix = firstNonBlank(config.GetConfigString(configDoneUserPrefix), defaultDoneUserPrefix)
	client.initTodoProjectionConfigFromConfig()
	return client, nil
}

func (c *RESTClient) StartProcess(ctx stdcontext.Context, req *types.StartProcessRequest) (*types.StartProcessResponse, error) {
	payload := map[string]interface{}{}
	if req != nil {
		variables := map[string]interface{}{}
		for key, value := range req.Variables {
			variables[key] = value
		}
		if req.BizID != "" && variables["bizId"] == nil {
			variables["bizId"] = req.BizID
		}
		if req.BizType != "" && variables["bizType"] == nil {
			variables["bizType"] = req.BizType
		}
		if req.Title != "" && variables["title"] == nil {
			variables["title"] = req.Title
		}
		if req.ProcessDefinitionID != "" {
			payload["processDefinitionId"] = req.ProcessDefinitionID
		} else if req.ProcessDefinitionKey != "" {
			tenantID := ""
			if len(variables) > 0 {
				tenantID = stringValue(variables["tenantId"])
			}
			definitionID, err := c.resolveProcessDefinitionID(ctx, req.ProcessDefinitionKey, tenantID)
			if err == nil && definitionID != "" {
				payload["processDefinitionId"] = definitionID
			} else {
				payload["processDefinitionKey"] = req.ProcessDefinitionKey
			}
		}
		if req.BusinessKey != "" {
			payload["businessKey"] = req.BusinessKey
		} else if req.BizID != "" {
			payload["businessKey"] = req.BizID
		}
		if req.Name != "" {
			payload["name"] = req.Name
		} else if req.Title != "" {
			payload["name"] = req.Title
		}
		if len(variables) > 0 {
			payload["variables"] = toFlowableVariables(variables)
		}
	}
	body, err := c.doJSON(ctx, http.MethodPost, "/runtime/process-instances", nil, payload)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	response := &types.StartProcessResponse{
		ProcessInstanceID:   stringValue(result["id"]),
		ProcessDefinitionID: stringValue(result["processDefinitionId"]),
		BusinessKey:         stringValue(result["businessKey"]),
		TenantID:            stringValue(result["tenantId"]),
	}
	c.invalidateProcessSummaryCache(ctx, response.ProcessInstanceID)
	c.recordProcessStart(ctx, req, response)
	return response, nil
}

func (c *RESTClient) resolveProcessDefinitionID(ctx stdcontext.Context, processDefinitionKey, tenantID string) (string, error) {
	key := strings.TrimSpace(processDefinitionKey)
	if key == "" {
		return "", errors.New("process definition key is required")
	}
	if strings.TrimSpace(tenantID) != "" {
		definitions, err := c.queryProcessDefinitions(ctx, map[string]string{
			"key":      key,
			"latest":   "true",
			"tenantId": strings.TrimSpace(tenantID),
			"size":     "1",
		})
		if err == nil && len(definitions) > 0 && definitions[0].ID != "" {
			return definitions[0].ID, nil
		}
	}
	definitions, err := c.queryProcessDefinitions(ctx, map[string]string{
		"key":    key,
		"latest": "true",
		"size":   "20",
	})
	if err != nil {
		return "", err
	}
	if len(definitions) == 0 {
		return "", fmt.Errorf("process definition not found by key: %s", key)
	}
	for _, definition := range definitions {
		if strings.TrimSpace(definition.TenantID) == "" {
			return definition.ID, nil
		}
	}
	return definitions[0].ID, nil
}

func (c *RESTClient) ListTodo(ctx stdcontext.Context, user *workflowcontext.UserContext, query *types.TaskQuery) (*types.TaskPage, error) {
	if user == nil {
		return nil, errors.New("workflow user context is required")
	}
	query = normalizeTaskQuery(query)
	projectedPage, projectedIDs := c.listTodoFromProjection(ctx, user, query)
	scopeCache := map[string]*types.ProcessProgressSummary{}
	if projectedPage != nil && query.Start == 0 && len(projectedPage.Items) >= query.Size {
		_ = c.enrichTaskPage(ctx, user, projectedPage, scopeCache, query.IncludeProgress)
		return projectedPage, nil
	}
	if !matchesTodoStatus(query.Status) {
		return &types.TaskPage{
			Items: []types.TaskInfo{},
			Total: 0,
			Start: query.Start,
			Size:  query.Size,
		}, nil
	}

	taskMap := map[string]types.TaskInfo{}
	for _, item := range projectedPageItems(projectedPage) {
		taskMap[strings.TrimSpace(item.TaskID)] = item
	}
	assigned, err := c.queryRuntimeTasks(ctx, c.buildRuntimeTaskQuery(query, map[string]string{
		"assignee": user.UserID,
		"sort":     "createTime",
		"order":    "asc",
	}))
	if err != nil {
		return nil, err
	}
	for _, task := range assigned {
		if !matchesAssignedRuntimeTask(task, user) {
			continue
		}
		allowed, filterErr := c.matchesProcessScope(ctx, task.ProcessInstanceID, user, scopeCache)
		if filterErr != nil || !allowed {
			continue
		}
		info := c.toTaskInfo(task, nil, false)
		c.applyTaskSummary(&info, lookupProcessSummary(scopeCache, task.ProcessInstanceID), false)
		if !matchesTaskQuery(info, query) {
			continue
		}
		if _, exists := projectedIDs[strings.TrimSpace(task.ID)]; exists {
			current := taskMap[task.ID]
			c.applyTaskSummary(&current, lookupProcessSummary(scopeCache, task.ProcessInstanceID), false)
			taskMap[task.ID] = current
			continue
		}
		taskMap[task.ID] = info
	}

	candidateUserTasks, err := c.queryRuntimeTasks(ctx, c.buildRuntimeTaskQuery(query, map[string]string{
		"candidateUser": user.UserID,
		"sort":          "createTime",
		"order":         "asc",
	}))
	if err == nil {
		for _, task := range candidateUserTasks {
			allowed, filterErr := c.matchesCandidateRuntimeTask(ctx, task, user)
			if filterErr != nil || !allowed {
				continue
			}
			allowed, filterErr = c.matchesProcessScope(ctx, task.ProcessInstanceID, user, scopeCache)
			if filterErr != nil || !allowed {
				continue
			}
			info := c.toTaskInfo(task, nil, false)
			c.applyTaskSummary(&info, lookupProcessSummary(scopeCache, task.ProcessInstanceID), false)
			if !matchesTaskQuery(info, query) {
				continue
			}
			if _, exists := projectedIDs[strings.TrimSpace(task.ID)]; exists {
				current := taskMap[task.ID]
				c.applyTaskSummary(&current, lookupProcessSummary(scopeCache, task.ProcessInstanceID), false)
				taskMap[task.ID] = current
				continue
			}
			taskMap[task.ID] = info
		}
	}

	groups := candidateGroups(user, c.groupPrefix, c.rolePrefix)
	if len(groups) > 0 {
		candidateGroupTasks, err := c.queryRuntimeTasks(ctx, c.buildRuntimeTaskQuery(query, map[string]string{
			"candidateGroupIn": strings.Join(groups, ","),
			"sort":             "createTime",
			"order":            "asc",
		}))
		if err == nil {
			for _, task := range candidateGroupTasks {
				allowed, filterErr := c.matchesCandidateRuntimeTask(ctx, task, user)
				if filterErr != nil || !allowed {
					continue
				}
				allowed, filterErr = c.matchesProcessScope(ctx, task.ProcessInstanceID, user, scopeCache)
				if filterErr != nil || !allowed {
					continue
				}
				info := c.toTaskInfo(task, nil, false)
				c.applyTaskSummary(&info, lookupProcessSummary(scopeCache, task.ProcessInstanceID), false)
				if !matchesTaskQuery(info, query) {
					continue
				}
				if _, exists := projectedIDs[strings.TrimSpace(task.ID)]; exists {
					current := taskMap[task.ID]
					c.applyTaskSummary(&current, lookupProcessSummary(scopeCache, task.ProcessInstanceID), false)
					taskMap[task.ID] = current
					continue
				}
				taskMap[task.ID] = info
			}
		}
	}

	items := make([]types.TaskInfo, 0, len(taskMap))
	for _, item := range taskMap {
		items = append(items, item)
	}
	sortTaskInfos(items, false)
	page := &types.TaskPage{
		Items: items,
		Total: int64(len(items)),
		Start: query.Start,
		Size:  query.Size,
	}
	_ = c.enrichTaskPage(ctx, user, page, scopeCache, query.IncludeProgress)
	return page, nil
}

func matchesAssignedRuntimeTask(task runtimeTaskRecord, user *workflowcontext.UserContext) bool {
	if user == nil || strings.TrimSpace(user.UserID) == "" {
		return false
	}
	return strings.TrimSpace(task.Assignee) == strings.TrimSpace(user.UserID)
}

func (c *RESTClient) matchesCandidateRuntimeTask(ctx stdcontext.Context, task runtimeTaskRecord, user *workflowcontext.UserContext) (bool, error) {
	if user == nil || strings.TrimSpace(user.UserID) == "" {
		return false, nil
	}
	if strings.TrimSpace(task.Assignee) != "" && strings.TrimSpace(task.Assignee) != strings.TrimSpace(user.UserID) {
		return false, nil
	}
	resolvedGroups := candidateGroups(user, c.groupPrefix, c.rolePrefix)
	if index, ok := c.loadCandidateTaskIndex(ctx, task.ID); ok {
		return index.matches(strings.TrimSpace(user.UserID), resolvedGroups), nil
	}
	links, err := c.getTaskIdentityLinks(ctx, task.ID)
	if err != nil {
		return false, err
	}
	candidateUsers, candidateRuntimeGroups := splitCandidateIdentityLinks(links)
	c.storeCandidateTaskIndex(ctx, task.ID, candidateUsers, candidateRuntimeGroups)
	userID := strings.TrimSpace(user.UserID)
	for _, candidateUser := range candidateUsers {
		if strings.TrimSpace(candidateUser) == userID {
			return true, nil
		}
	}
	groupSet := make(map[string]struct{}, len(resolvedGroups))
	for _, group := range resolvedGroups {
		groupSet[strings.TrimSpace(group)] = struct{}{}
	}
	for _, candidateGroup := range candidateRuntimeGroups {
		if _, ok := groupSet[strings.TrimSpace(candidateGroup)]; ok {
			return true, nil
		}
	}
	return false, nil
}

func (c *RESTClient) ListDone(ctx stdcontext.Context, user *workflowcontext.UserContext, query *types.TaskQuery) (*types.TaskPage, error) {
	if user == nil {
		return nil, errors.New("workflow user context is required")
	}
	query = normalizeTaskQuery(query)
	projectedPage, projectedIDs := c.listDoneFromProjection(ctx, user, query)
	scopeCache := map[string]*types.ProcessProgressSummary{}
	if projectedPage != nil && query.Start == 0 && len(projectedPage.Items) >= query.Size {
		_ = c.enrichTaskPage(ctx, user, projectedPage, scopeCache, query.IncludeProgress)
		return projectedPage, nil
	}
	historic, err := c.queryHistoricTasks(ctx, c.buildHistoricTaskQuery(query, map[string]string{
		"taskAssignee": user.UserID,
		"finished":     "true",
		"sort":         "endTime",
		"order":        "desc",
	}))
	if err != nil {
		return nil, err
	}
	items := make([]types.TaskInfo, 0, len(historic))
	for _, task := range historic {
		if !matchesHistoricTaskUser(task, user) {
			continue
		}
		allowed, filterErr := c.matchesProcessScope(ctx, task.ProcessInstanceID, user, scopeCache)
		if filterErr != nil || !allowed {
			continue
		}
		info := c.toTaskInfo(runtimeTaskRecord{}, &task, true)
		c.applyTaskSummary(&info, lookupProcessSummary(scopeCache, task.ProcessInstanceID), false)
		if !matchesTaskQuery(info, query) {
			continue
		}
		items = append(items, info)
	}
	sortTaskInfos(items, true)
	if projectedPage != nil && query.Start == 0 {
		items = mergeDoneTaskInfos(projectedPage.Items, items, projectedIDs)
		sortTaskInfos(items, true)
		if len(items) > query.Size {
			items = items[:query.Size]
		}
	}
	page := &types.TaskPage{
		Items: items,
		Total: int64(len(items)),
		Start: query.Start,
		Size:  query.Size,
	}
	_ = c.enrichTaskPage(ctx, user, page, scopeCache, query.IncludeProgress)
	return page, nil
}

func matchesHistoricTaskUser(task historicTaskRecord, user *workflowcontext.UserContext) bool {
	if user == nil || strings.TrimSpace(user.UserID) == "" {
		return false
	}
	userID := strings.TrimSpace(user.UserID)
	return strings.TrimSpace(task.Assignee) == userID || strings.TrimSpace(task.Owner) == userID
}

func (c *RESTClient) matchesProcessScope(ctx stdcontext.Context, processInstanceID string, user *workflowcontext.UserContext, cache map[string]*types.ProcessProgressSummary) (bool, error) {
	if user == nil {
		return false, nil
	}
	if strings.TrimSpace(processInstanceID) == "" {
		return false, nil
	}
	if strings.TrimSpace(user.TenantID) == "" && strings.TrimSpace(user.SystemCode) == "" {
		return true, nil
	}
	if summary, ok := cache[strings.TrimSpace(processInstanceID)]; ok {
		return matchesSummaryScope(summary, user), nil
	}
	summaryProcessInstanceID, err := c.resolveSummaryProcessInstanceID(ctx, processInstanceID)
	if err != nil {
		return false, err
	}
	if summary, ok := cache[strings.TrimSpace(summaryProcessInstanceID)]; ok {
		cache[strings.TrimSpace(processInstanceID)] = summary
		return matchesSummaryScope(summary, user), nil
	}
	summary, err := c.getProcessSummary(ctx, strings.TrimSpace(summaryProcessInstanceID), user)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(summary.ProcessInstanceID) == "" {
		return false, nil
	}
	cache[strings.TrimSpace(summaryProcessInstanceID)] = &summary
	cache[strings.TrimSpace(processInstanceID)] = &summary
	return matchesSummaryScope(&summary, user), nil
}

func matchesSummaryScope(summary *types.ProcessProgressSummary, user *workflowcontext.UserContext) bool {
	if user == nil {
		return false
	}
	if summary == nil {
		return strings.TrimSpace(user.TenantID) == "" && strings.TrimSpace(user.SystemCode) == ""
	}
	userTenantID := strings.TrimSpace(user.TenantID)
	summaryTenantID := strings.TrimSpace(summary.TenantID)
	if userTenantID != "" && summaryTenantID != "" && summaryTenantID != userTenantID {
		return false
	}
	if strings.TrimSpace(user.SystemCode) != "" && strings.TrimSpace(summary.SystemCode) != "" && strings.TrimSpace(summary.SystemCode) != strings.TrimSpace(user.SystemCode) {
		return false
	}
	return true
}

func (c *RESTClient) GetTaskContext(ctx stdcontext.Context, taskID string, user *workflowcontext.UserContext) (*types.TaskContextResponse, error) {
	task, err := c.getRuntimeTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	currentProcess, err := c.getProcessInstance(ctx, task.ProcessInstanceID)
	if err != nil {
		return nil, err
	}
	rootProcess, err := c.resolveRootProcessInstance(ctx, currentProcess)
	if err != nil {
		return nil, err
	}
	currentVariables, err := c.getProcessVariables(ctx, task.ProcessInstanceID)
	if err != nil {
		return nil, err
	}
	rootVariables := currentVariables
	if strings.TrimSpace(rootProcess.ID) != "" && strings.TrimSpace(rootProcess.ID) != strings.TrimSpace(task.ProcessInstanceID) {
		rootVariables, err = c.getProcessVariables(ctx, rootProcess.ID)
		if err != nil {
			return nil, err
		}
	}
	variables := mergeProcessVariables(rootVariables, currentVariables)
	taskInfo := c.toTaskInfo(task, nil, false)
	taskInfo.RootProcessInstanceID = firstNonBlank(rootProcess.ID, task.ProcessInstanceID)
	taskInfo.BizID = firstNonBlank(taskInfo.BizID, stringValue(rootVariables["bizId"]), stringValue(currentVariables["bizId"]), task.BusinessKey)
	taskInfo.BizType = firstNonBlank(taskInfo.BizType, stringValue(rootVariables["bizType"]), stringValue(currentVariables["bizType"]))
	taskInfo.Title = firstNonBlank(taskInfo.Title, stringValue(rootVariables["title"]), stringValue(currentVariables["title"]))
	taskInfo.PayloadRef = firstNonBlank(taskInfo.PayloadRef, stringValue(rootVariables["payloadRef"]), stringValue(currentVariables["payloadRef"]), taskInfo.BizID)
	taskInfo.TenantID = firstNonBlank(taskInfo.TenantID, stringValue(rootVariables["tenantId"]), stringValue(currentVariables["tenantId"]), rootProcess.TenantID, currentProcess.TenantID, task.TenantID)
	return &types.TaskContextResponse{
		Task: taskInfo,
		Business: types.TaskBusinessContext{
			BizID:      firstNonBlank(stringValue(rootVariables["bizId"]), stringValue(currentVariables["bizId"]), task.BusinessKey),
			BizType:    firstNonBlank(stringValue(rootVariables["bizType"]), stringValue(currentVariables["bizType"])),
			Title:      firstNonBlank(stringValue(rootVariables["title"]), stringValue(currentVariables["title"])),
			PayloadRef: firstNonBlank(stringValue(rootVariables["payloadRef"]), stringValue(currentVariables["payloadRef"]), stringValue(rootVariables["bizId"]), stringValue(currentVariables["bizId"])),
			SystemCode: firstNonBlank(stringValue(rootVariables["systemCode"]), stringValue(currentVariables["systemCode"])),
			TenantID:   firstNonBlank(stringValue(rootVariables["tenantId"]), stringValue(currentVariables["tenantId"]), rootProcess.TenantID, currentProcess.TenantID, task.TenantID),
		},
		Variables: variables,
		FormRef:   nil,
	}, nil
}

func (c *RESTClient) CompleteTask(ctx stdcontext.Context, taskID string, req *types.CompleteTaskRequest, user *workflowcontext.UserContext) error {
	task, err := c.getRuntimeTask(ctx, taskID)
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(task.DelegationState), "PENDING") {
		return errors.New("forbidden: delegated task must be resolved before complete")
	}
	if err := c.ensureTaskAssignedToCurrentUser(ctx, task, user, true); err != nil {
		return err
	}
	if strings.TrimSpace(task.Assignee) == "" {
		if err := c.claimRuntimeTask(ctx, taskID, strings.TrimSpace(user.UserID)); err != nil {
			return err
		}
		task.Assignee = strings.TrimSpace(user.UserID)
	}

	variables := map[string]interface{}{}
	if req != nil {
		for key, value := range req.Variables {
			variables[key] = value
		}
		putIfNonBlank(variables, "activityId", req.ActivityID)
		putIfNonBlank(variables, "result", req.Result)
		putIfNonBlank(variables, "comment", req.Comment)
		putIfNonBlank(variables, "reworkComment", req.ReworkComment)
		putIfNonBlank(variables, "payloadRef", req.PayloadRef)
		mergeOptionalNeedExpert(variables, req)
	}

	payload := map[string]interface{}{
		"action": "complete",
	}
	if len(variables) > 0 {
		payload["variables"] = toFlowableVariables(variables)
	}
	_, err = c.completeTaskWithRetry(ctx, taskID, payload)
	if err == nil {
		c.storeDoneTaskProjection(ctx, c.buildDoneTaskProjection(task, variables, user, req))
		comment := ""
		reason := ""
		if req != nil {
			comment = firstNonBlank(req.Comment, req.ReworkComment)
			reason = strings.TrimSpace(req.Result)
		}
		afterTask := task
		if user != nil && strings.TrimSpace(afterTask.Assignee) == "" {
			afterTask.Assignee = strings.TrimSpace(user.UserID)
		}
		c.recordTaskActionRecord(ctx, types.TaskActionTypeComplete, task, afterTask, user, comment, reason)
		c.invalidateActionTimelineCache(ctx, task.ProcessInstanceID)
		c.invalidateProcessSummaryCache(ctx, task.ProcessInstanceID)
		c.invalidateCandidateTaskIndex(ctx, taskID)
		c.removeRuntimeTaskProjection(ctx, taskID)
	}
	return err
}

func (c *RESTClient) ClaimTask(ctx stdcontext.Context, taskID string, user *workflowcontext.UserContext) (*types.TaskActionResponse, error) {
	task, err := c.getRuntimeTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	beforeTask := task
	if err := c.ensureTaskAssignedToCurrentUser(ctx, task, user, true); err != nil {
		return nil, err
	}
	if strings.TrimSpace(task.Assignee) == "" {
		if err := c.claimRuntimeTask(ctx, taskID, strings.TrimSpace(user.UserID)); err != nil {
			return nil, err
		}
		task, err = c.getRuntimeTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		c.syncRuntimeTaskProjection(ctx, taskID, task.ProcessInstanceID)
		recordBefore := beforeTask
		if strings.TrimSpace(recordBefore.Assignee) == "" && user != nil {
			recordBefore.Assignee = ""
		}
		c.recordWorkflowAction(ctx, types.TaskActionTypeClaim, recordBefore, task, user, "")
		c.recordTaskActionRecord(ctx, types.TaskActionTypeClaim, recordBefore, task, user, "", "")
	}
	return buildTaskActionResponse(task, "claimed"), nil
}

func (c *RESTClient) UnclaimTask(ctx stdcontext.Context, taskID string, user *workflowcontext.UserContext) (*types.TaskActionResponse, error) {
	task, err := c.getRuntimeTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	beforeTask := task
	if strings.EqualFold(strings.TrimSpace(task.DelegationState), "PENDING") {
		return nil, errors.New("forbidden: delegated task cannot be unclaimed")
	}
	if err := c.ensureTaskAssignedToCurrentUser(ctx, task, user, false); err != nil {
		return nil, err
	}
	if strings.TrimSpace(task.Assignee) == "" {
		return nil, errors.New("invalid task state: task is not claimed")
	}
	if err := c.unclaimRuntimeTask(ctx, taskID); err != nil {
		return nil, err
	}
	task, err = c.getRuntimeTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	c.syncRuntimeTaskProjection(ctx, taskID, task.ProcessInstanceID)
	c.invalidateCandidateTaskIndex(ctx, taskID)
	c.recordWorkflowAction(ctx, types.TaskActionTypeUnclaim, beforeTask, task, user, "")
	c.recordTaskActionRecord(ctx, types.TaskActionTypeUnclaim, beforeTask, task, user, "", "")
	return buildTaskActionResponse(task, "unclaimed"), nil
}

func (c *RESTClient) DelegateTask(ctx stdcontext.Context, taskID string, req *types.TaskDelegateRequest, user *workflowcontext.UserContext) (*types.TaskActionResponse, error) {
	if req == nil || strings.TrimSpace(req.Assignee) == "" {
		return nil, errors.New("assignee is required")
	}
	task, err := c.getRuntimeTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	beforeTask := task
	if strings.EqualFold(strings.TrimSpace(task.DelegationState), "PENDING") {
		return nil, errors.New("forbidden: task is already delegated")
	}
	if err := c.ensureTaskAssignedToCurrentUser(ctx, task, user, true); err != nil {
		return nil, err
	}
	if strings.TrimSpace(task.Assignee) == "" {
		if err := c.claimRuntimeTask(ctx, taskID, strings.TrimSpace(user.UserID)); err != nil {
			return nil, err
		}
		beforeTask.Assignee = strings.TrimSpace(user.UserID)
	}
	_, err = c.doJSON(ctx, http.MethodPost, "/runtime/tasks/"+taskID, nil, map[string]interface{}{
		"action":   "delegate",
		"assignee": strings.TrimSpace(req.Assignee),
	})
	if err != nil {
		return nil, err
	}
	task, err = c.getRuntimeTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	c.syncRuntimeTaskProjection(ctx, taskID, task.ProcessInstanceID)
	c.invalidateCandidateTaskIndex(ctx, taskID)
	c.recordWorkflowAction(ctx, types.TaskActionTypeDelegate, beforeTask, task, user, "")
	c.recordTaskActionRecord(ctx, types.TaskActionTypeDelegate, beforeTask, task, user, "", "")
	return buildTaskActionResponse(task, "delegated"), nil
}

func (c *RESTClient) ResolveTask(ctx stdcontext.Context, taskID string, req *types.TaskResolveRequest, user *workflowcontext.UserContext) (*types.TaskActionResponse, error) {
	task, err := c.getRuntimeTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	beforeTask := task
	if !strings.EqualFold(strings.TrimSpace(task.DelegationState), "PENDING") {
		return nil, errors.New("forbidden: task is not in delegated pending state")
	}
	if err := c.ensureTaskAssignedToCurrentUser(ctx, task, user, false); err != nil {
		return nil, err
	}
	payload := map[string]interface{}{
		"action": "resolve",
	}
	if req != nil && len(req.Variables) > 0 {
		payload["variables"] = toFlowableVariables(req.Variables)
	}
	_, err = c.doJSON(ctx, http.MethodPost, "/runtime/tasks/"+taskID, nil, payload)
	if err != nil {
		return nil, err
	}
	task, err = c.getRuntimeTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	c.syncRuntimeTaskProjection(ctx, taskID, task.ProcessInstanceID)
	c.invalidateCandidateTaskIndex(ctx, taskID)
	c.recordWorkflowAction(ctx, types.TaskActionTypeResolve, beforeTask, task, user, "")
	c.recordTaskActionRecord(ctx, types.TaskActionTypeResolve, beforeTask, task, user, "", "")
	return buildTaskActionResponse(task, "resolved"), nil
}

func (c *RESTClient) TransferTask(ctx stdcontext.Context, taskID string, req *types.TaskTransferRequest, user *workflowcontext.UserContext) (*types.TaskActionResponse, error) {
	if req == nil || strings.TrimSpace(req.Assignee) == "" {
		return nil, errors.New("assignee is required")
	}
	targetAssignee := strings.TrimSpace(req.Assignee)
	task, err := c.getRuntimeTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	beforeTask := task
	if strings.EqualFold(strings.TrimSpace(task.DelegationState), "PENDING") {
		return nil, errors.New("forbidden: delegated task cannot be transferred")
	}
	if err := c.ensureTaskAssignedToCurrentUser(ctx, task, user, true); err != nil {
		return nil, err
	}
	currentUserID := ""
	if user != nil {
		currentUserID = strings.TrimSpace(user.UserID)
	}
	if targetAssignee == currentUserID {
		return nil, errors.New("invalid transfer target: assignee cannot be current user")
	}
	if strings.TrimSpace(task.Assignee) == "" {
		if err := c.claimRuntimeTask(ctx, taskID, currentUserID); err != nil {
			return nil, err
		}
		beforeTask.Assignee = currentUserID
	}
	if err := c.assignRuntimeTask(ctx, taskID, targetAssignee); err != nil {
		return nil, err
	}
	comment := buildTransferComment(currentUserID, targetAssignee, firstNonBlank(req.Reason, ""))
	if comment != "" {
		_ = c.addTaskComment(ctx, taskID, comment)
	}
	task, err = c.getRuntimeTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	c.syncRuntimeTaskProjection(ctx, taskID, task.ProcessInstanceID)
	c.invalidateCandidateTaskIndex(ctx, taskID)
	c.recordWorkflowAction(ctx, types.TaskActionTypeTransfer, beforeTask, task, user, req.Reason)
	c.recordTaskActionRecord(ctx, types.TaskActionTypeTransfer, beforeTask, task, user, "", req.Reason)
	return buildTaskActionResponse(task, "transferred"), nil
}

func mergeOptionalNeedExpert(target map[string]interface{}, req *types.CompleteTaskRequest) {
	if target == nil || req == nil {
		return
	}
	if _, exists := target["needExpert"]; exists {
		return
	}
	if req.NeedExpert {
		target["needExpert"] = true
	}
}

func (c *RESTClient) completeTaskWithRetry(ctx stdcontext.Context, taskID string, payload map[string]interface{}) ([]byte, error) {
	body, err := c.doJSON(ctx, http.MethodPost, "/runtime/tasks/"+taskID, nil, payload)
	if err == nil || !shouldRetryFlowableDeadlock(err) {
		return body, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(150 * time.Millisecond):
	}
	return c.doJSON(ctx, http.MethodPost, "/runtime/tasks/"+taskID, nil, payload)
}

func (c *RESTClient) ensureTaskAssignedToCurrentUser(ctx stdcontext.Context, task runtimeTaskRecord, user *workflowcontext.UserContext, allowCandidate bool) error {
	if user == nil || strings.TrimSpace(user.UserID) == "" {
		return errors.New("workflow user context is required")
	}
	userID := strings.TrimSpace(user.UserID)
	assignee := strings.TrimSpace(task.Assignee)
	if assignee == userID {
		return nil
	}
	if assignee != "" {
		return errors.New("forbidden: task not assigned to current user")
	}
	if !allowCandidate {
		return errors.New("forbidden: task not assigned to current user")
	}
	allowed, err := c.matchesCandidateRuntimeTask(ctx, task, user)
	if err != nil {
		return err
	}
	if !allowed {
		return errors.New("forbidden: current user cannot operate this task")
	}
	return nil
}

func (c *RESTClient) claimRuntimeTask(ctx stdcontext.Context, taskID, assignee string) error {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(assignee) == "" {
		return errors.New("task id and assignee are required")
	}
	_, err := c.doJSON(ctx, http.MethodPost, "/runtime/tasks/"+taskID, nil, map[string]interface{}{
		"action":   "claim",
		"assignee": strings.TrimSpace(assignee),
	})
	return err
}

func (c *RESTClient) assignRuntimeTask(ctx stdcontext.Context, taskID, assignee string) error {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(assignee) == "" {
		return errors.New("task id and assignee are required")
	}
	_, err := c.doJSON(ctx, http.MethodPut, "/runtime/tasks/"+taskID, nil, map[string]interface{}{
		"assignee": strings.TrimSpace(assignee),
	})
	return err
}

func (c *RESTClient) unclaimRuntimeTask(ctx stdcontext.Context, taskID string) error {
	if strings.TrimSpace(taskID) == "" {
		return errors.New("task id is required")
	}
	_, err := c.doJSON(ctx, http.MethodPut, "/runtime/tasks/"+taskID, nil, map[string]interface{}{
		"assignee": nil,
	})
	return err
}

func (c *RESTClient) addTaskComment(ctx stdcontext.Context, taskID, message string) error {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(message) == "" {
		return nil
	}
	_, err := c.doJSON(ctx, http.MethodPost, "/runtime/tasks/"+taskID+"/comments", nil, map[string]interface{}{
		"message": strings.TrimSpace(message),
	})
	return err
}

func buildTaskActionResponse(task runtimeTaskRecord, status string) *types.TaskActionResponse {
	return &types.TaskActionResponse{
		TaskID:          strings.TrimSpace(task.ID),
		Status:          strings.TrimSpace(status),
		Assignee:        strings.TrimSpace(task.Assignee),
		Owner:           strings.TrimSpace(task.Owner),
		DelegationState: strings.TrimSpace(task.DelegationState),
	}
}

func shouldRetryFlowableDeadlock(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "deadlock found when trying to get lock") ||
		strings.Contains(message, "mysqltransactionrollbackexception")
}

func buildTransferComment(fromUserID, toUserID, reason string) string {
	fromUserID = strings.TrimSpace(fromUserID)
	toUserID = strings.TrimSpace(toUserID)
	reason = strings.TrimSpace(reason)
	if fromUserID == "" || toUserID == "" {
		return ""
	}
	if reason == "" {
		return fmt.Sprintf("[TRANSFER] from=%s to=%s", fromUserID, toUserID)
	}
	return fmt.Sprintf("[TRANSFER] from=%s to=%s reason=%s", fromUserID, toUserID, reason)
}

func (c *RESTClient) getRawDefinitionXML(ctx stdcontext.Context, processInstanceID string) ([]byte, error) {
	process, err := c.getProcessInstance(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	path := "/repository/process-definitions/" + process.ProcessDefinitionID + "/resourcedata"
	return c.doRaw(ctx, http.MethodGet, path, nil, nil, "application/xml")
}

func (c *RESTClient) GetDefinitionXML(ctx stdcontext.Context, processInstanceID string, user *workflowcontext.UserContext) ([]byte, error) {
	return c.getRawDefinitionXML(ctx, processInstanceID)
}

func (c *RESTClient) GetTaskFormData(ctx stdcontext.Context, taskID string) (map[string]interface{}, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, errors.New("task id is required")
	}
	body, err := c.doJSON(ctx, http.MethodGet, "/form/form-data", map[string]string{
		"taskId": strings.TrimSpace(taskID),
	}, nil)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *RESTClient) ListDeploymentResources(ctx stdcontext.Context, deploymentID string) ([]DeploymentResource, error) {
	if strings.TrimSpace(deploymentID) == "" {
		return nil, errors.New("deployment id is required")
	}
	body, err := c.doJSON(ctx, http.MethodGet, "/repository/deployments/"+strings.TrimSpace(deploymentID)+"/resources", nil, nil)
	if err != nil {
		return nil, err
	}
	var raw interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	items := asObjectSlice(raw)
	if root, ok := raw.(map[string]interface{}); ok {
		items = asObjectSlice(root["data"])
	}
	result := make([]DeploymentResource, 0, len(items))
	for _, item := range items {
		id := stringValue(item["id"])
		name := stringValue(item["name"])
		if id == "" || name == "" {
			continue
		}
		result = append(result, DeploymentResource{
			ID:   id,
			Name: name,
		})
	}
	return result, nil
}

func (c *RESTClient) GetDeploymentResourceData(ctx stdcontext.Context, deploymentID, resourceID string) ([]byte, error) {
	if strings.TrimSpace(deploymentID) == "" || strings.TrimSpace(resourceID) == "" {
		return nil, errors.New("deployment id and resource id are required")
	}
	return c.doRaw(ctx, http.MethodGet, "/repository/deployments/"+strings.TrimSpace(deploymentID)+"/resourcedata/"+strings.TrimSpace(resourceID), nil, nil, "application/json")
}

func (c *RESTClient) doRaw(ctx stdcontext.Context, method, path string, query map[string]string, body interface{}, accept string) ([]byte, error) {
	targetURL := c.baseURL + path
	if len(query) > 0 {
		values := url.Values{}
		for key, value := range query {
			if strings.TrimSpace(value) == "" {
				continue
			}
			values.Set(key, value)
		}
		if encoded := values.Encode(); encoded != "" {
			targetURL += "?" + encoded
		}
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = strings.NewReader(string(payload))
	}
	request, err := http.NewRequestWithContext(ctx, method, targetURL, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if accept == "" {
		accept = "application/json"
	}
	request.Header.Set("Accept", accept)
	if c.username != "" {
		request.SetBasicAuth(c.username, c.password)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s failed: status=%d body=%s", method, path, response.StatusCode, string(data))
	}
	return data, nil
}

func (c *RESTClient) doJSON(ctx stdcontext.Context, method, path string, query map[string]string, body interface{}) ([]byte, error) {
	return c.doRaw(ctx, method, path, query, body, "application/json")
}

func (c *RESTClient) getRuntimeTask(ctx stdcontext.Context, taskID string) (runtimeTaskRecord, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/runtime/tasks/"+taskID, nil, nil)
	if err != nil {
		return runtimeTaskRecord{}, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return runtimeTaskRecord{}, err
	}
	task := parseRuntimeTask(raw)
	links, err := c.getTaskIdentityLinks(ctx, taskID)
	if err == nil {
		task.CandidateUsers, task.CandidateGroups = splitCandidateIdentityLinks(links)
	}
	return task, nil
}

func (c *RESTClient) getProcessInstance(ctx stdcontext.Context, processInstanceID string) (processInstanceRecord, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/runtime/process-instances/"+processInstanceID, nil, nil)
	if err == nil {
		return parseProcessInstance(body), nil
	}
	body, err = c.doJSON(ctx, http.MethodGet, "/history/historic-process-instances/"+processInstanceID, nil, nil)
	if err != nil {
		return processInstanceRecord{}, err
	}
	return parseProcessInstance(body), nil
}

func (c *RESTClient) getProcessVariables(ctx stdcontext.Context, processInstanceID string) (map[string]interface{}, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/runtime/process-instances/"+processInstanceID+"/variables", nil, nil)
	if err == nil {
		result, parseErr := parseRuntimeVariableRows(body)
		if parseErr != nil {
			return nil, parseErr
		}
		if len(result) > 0 {
			return result, nil
		}
	}

	body, err = c.doJSON(ctx, http.MethodGet, "/history/historic-variable-instances", map[string]string{
		"processInstanceId": strings.TrimSpace(processInstanceID),
		"size":              "500",
	}, nil)
	if err != nil {
		return map[string]interface{}{}, nil
	}
	result, parseErr := parseHistoricVariableRows(body)
	if parseErr != nil {
		return nil, parseErr
	}
	return result, nil
}

func (c *RESTClient) resolveRootProcessInstance(ctx stdcontext.Context, process processInstanceRecord) (processInstanceRecord, error) {
	current := process
	for depth := 0; depth < 10; depth++ {
		parentID := strings.TrimSpace(current.SuperProcessInstanceID)
		if parentID == "" {
			return current, nil
		}
		parent, err := c.getProcessInstance(ctx, parentID)
		if err != nil {
			return current, err
		}
		current = parent
	}
	return current, nil
}

func mergeProcessVariables(base, override map[string]interface{}) map[string]interface{} {
	if len(base) == 0 && len(override) == 0 {
		return map[string]interface{}{}
	}
	merged := make(map[string]interface{}, len(base)+len(override))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func parseRuntimeVariableRows(body []byte) (map[string]interface{}, error) {
	var rows []map[string]interface{}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, err
	}
	result := map[string]interface{}{}
	for _, row := range rows {
		name := stringValue(row["name"])
		if name == "" {
			continue
		}
		result[name] = row["value"]
	}
	return result, nil
}

func parseHistoricVariableRows(body []byte) (map[string]interface{}, error) {
	data, err := extractDataArray(body)
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{}
	for _, row := range data {
		variable, ok := row["variable"].(map[string]interface{})
		if !ok {
			continue
		}
		name := stringValue(variable["name"])
		if name == "" {
			continue
		}
		result[name] = variable["value"]
	}
	return result, nil
}

func (c *RESTClient) getProcessSummary(ctx stdcontext.Context, processInstanceID string, user *workflowcontext.UserContext) (types.ProcessProgressSummary, error) {
	if cached, ok := c.loadProcessSummaryCache(ctx, processInstanceID); ok {
		return cached, nil
	}
	view, err := c.GetProgressView(ctx, strings.TrimSpace(processInstanceID), user)
	if err != nil {
		return types.ProcessProgressSummary{}, err
	}
	if view == nil {
		return types.ProcessProgressSummary{}, nil
	}
	return view.Summary, nil
}

func (c *RESTClient) resolveSummaryProcessInstanceID(ctx stdcontext.Context, processInstanceID string) (string, error) {
	processInstanceID = strings.TrimSpace(processInstanceID)
	if processInstanceID == "" {
		return "", nil
	}
	process, err := c.getProcessInstance(ctx, processInstanceID)
	if err != nil {
		return processInstanceID, err
	}
	root, err := c.resolveRootProcessInstance(ctx, process)
	if err != nil {
		return processInstanceID, err
	}
	return firstNonBlank(strings.TrimSpace(root.ID), processInstanceID), nil
}

func (c *RESTClient) loadProcessSummaryCache(ctx stdcontext.Context, processInstanceID string) (types.ProcessProgressSummary, bool) {
	if orm.Redis == nil || c == nil || c.summaryCacheTTL <= 0 {
		return types.ProcessProgressSummary{}, false
	}
	key := c.processSummaryCacheKey(processInstanceID)
	if key == "" {
		return types.ProcessProgressSummary{}, false
	}
	value, err := orm.Redis.Get(ctx, key)
	if err != nil || strings.TrimSpace(value) == "" {
		return types.ProcessProgressSummary{}, false
	}
	var summary types.ProcessProgressSummary
	if err := json.Unmarshal([]byte(value), &summary); err != nil {
		return types.ProcessProgressSummary{}, false
	}
	if strings.TrimSpace(summary.ProcessInstanceID) == "" {
		return types.ProcessProgressSummary{}, false
	}
	return summary, true
}

func (c *RESTClient) storeProcessSummaryCache(ctx stdcontext.Context, summary *types.ProcessProgressSummary) {
	if orm.Redis == nil || c == nil || summary == nil || c.summaryCacheTTL <= 0 {
		return
	}
	key := c.processSummaryCacheKey(summary.ProcessInstanceID)
	if key == "" {
		return
	}
	payload, err := json.Marshal(summary)
	if err != nil {
		return
	}
	_ = orm.Redis.Set(ctx, key, string(payload), c.summaryCacheTTL)
}

func (c *RESTClient) invalidateProcessSummaryCache(ctx stdcontext.Context, processInstanceID string) {
	if orm.Redis == nil || c == nil {
		return
	}
	key := c.processSummaryCacheKey(processInstanceID)
	if key == "" {
		return
	}
	_, _ = orm.Redis.Del(ctx, key)
}

func (c *RESTClient) processSummaryCacheKey(processInstanceID string) string {
	processInstanceID = strings.TrimSpace(processInstanceID)
	if c == nil || processInstanceID == "" {
		return ""
	}
	return firstNonBlank(c.summaryCachePrefix, defaultSummaryCachePrefix) + processInstanceID
}

type candidateTaskIndex struct {
	TaskID          string   `json:"taskId"`
	CandidateUsers  []string `json:"candidateUsers,omitempty"`
	CandidateGroups []string `json:"candidateGroups,omitempty"`
	CachedAt        string   `json:"cachedAt,omitempty"`
}

type doneTaskProjection struct {
	TaskID                string `json:"taskId"`
	TaskName              string `json:"taskName,omitempty"`
	ActivityID            string `json:"activityId,omitempty"`
	ActivityName          string `json:"activityName,omitempty"`
	ProcessInstanceID     string `json:"processInstanceId,omitempty"`
	RootProcessInstanceID string `json:"rootProcessInstanceId,omitempty"`
	ProcessDefinitionID   string `json:"processDefinitionId,omitempty"`
	ProcessDefinitionKey  string `json:"processDefinitionKey,omitempty"`
	BizID                 string `json:"bizId,omitempty"`
	BizType               string `json:"bizType,omitempty"`
	Title                 string `json:"title,omitempty"`
	PayloadRef            string `json:"payloadRef,omitempty"`
	CreatedAt             string `json:"createdAt,omitempty"`
	CompletedAt           string `json:"completedAt,omitempty"`
	Assignee              string `json:"assignee,omitempty"`
	Owner                 string `json:"owner,omitempty"`
	DelegationState       string `json:"delegationState,omitempty"`
	TenantID              string `json:"tenantId,omitempty"`
	SystemCode            string `json:"systemCode,omitempty"`
	FormKey               string `json:"formKey,omitempty"`
}

func (i candidateTaskIndex) matches(userID string, groups []string) bool {
	userID = strings.TrimSpace(userID)
	for _, candidateUser := range i.CandidateUsers {
		if strings.TrimSpace(candidateUser) == userID {
			return true
		}
	}
	groupSet := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		groupSet[strings.TrimSpace(group)] = struct{}{}
	}
	for _, candidateGroup := range i.CandidateGroups {
		if _, ok := groupSet[strings.TrimSpace(candidateGroup)]; ok {
			return true
		}
	}
	return false
}

func (p doneTaskProjection) toTaskInfo() types.TaskInfo {
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
		CompletedAt:           p.CompletedAt,
		Assignee:              p.Assignee,
		Owner:                 p.Owner,
		DelegationState:       p.DelegationState,
		TenantID:              p.TenantID,
		FormKey:               p.FormKey,
	}
}

func (c *RESTClient) loadCandidateTaskIndex(ctx stdcontext.Context, taskID string) (candidateTaskIndex, bool) {
	if orm.Redis == nil || c == nil || c.candidateIndexTTL <= 0 {
		return candidateTaskIndex{}, false
	}
	key := c.candidateTaskIndexKey(taskID)
	if key == "" {
		return candidateTaskIndex{}, false
	}
	value, err := orm.Redis.Get(ctx, key)
	if err != nil || strings.TrimSpace(value) == "" {
		return candidateTaskIndex{}, false
	}
	var index candidateTaskIndex
	if err := json.Unmarshal([]byte(value), &index); err != nil {
		return candidateTaskIndex{}, false
	}
	if strings.TrimSpace(index.TaskID) == "" {
		return candidateTaskIndex{}, false
	}
	return index, true
}

func (c *RESTClient) storeCandidateTaskIndex(ctx stdcontext.Context, taskID string, users, groups []string) {
	if orm.Redis == nil || c == nil || c.candidateIndexTTL <= 0 {
		return
	}
	key := c.candidateTaskIndexKey(taskID)
	if key == "" {
		return
	}
	index := candidateTaskIndex{
		TaskID:          strings.TrimSpace(taskID),
		CandidateUsers:  append([]string(nil), users...),
		CandidateGroups: append([]string(nil), groups...),
		CachedAt:        time.Now().Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(index)
	if err != nil {
		return
	}
	_ = orm.Redis.Set(ctx, key, string(payload), c.candidateIndexTTL)
}

func (c *RESTClient) invalidateCandidateTaskIndex(ctx stdcontext.Context, taskID string) {
	if orm.Redis == nil || c == nil {
		return
	}
	key := c.candidateTaskIndexKey(taskID)
	if key == "" {
		return
	}
	_, _ = orm.Redis.Del(ctx, key)
}

func (c *RESTClient) candidateTaskIndexKey(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if c == nil || taskID == "" {
		return ""
	}
	return firstNonBlank(c.candidateIndexKey, defaultCandidateIndexPrefix) + taskID
}

func (c *RESTClient) buildDoneTaskProjection(task runtimeTaskRecord, variables map[string]interface{}, user *workflowcontext.UserContext, req *types.CompleteTaskRequest) doneTaskProjection {
	merged := make(map[string]interface{}, len(variables)+8)
	for key, value := range variables {
		merged[key] = value
	}
	if req != nil {
		for key, value := range req.Variables {
			merged[key] = value
		}
		putIfNonBlank(merged, "payloadRef", req.PayloadRef)
		putIfNonBlank(merged, "activityId", req.ActivityID)
		putIfNonBlank(merged, "result", req.Result)
		putIfNonBlank(merged, "comment", req.Comment)
		putIfNonBlank(merged, "reworkComment", req.ReworkComment)
	}
	completedAt := time.Now().UTC().Format(time.RFC3339Nano)
	assignee := strings.TrimSpace(task.Assignee)
	if assignee == "" && user != nil {
		assignee = strings.TrimSpace(user.UserID)
	}
	return doneTaskProjection{
		TaskID:               task.ID,
		TaskName:             task.Name,
		ActivityID:           task.TaskDefinitionKey,
		ActivityName:         firstNonBlank(task.Name, task.TaskDefinitionKey),
		ProcessInstanceID:    task.ProcessInstanceID,
		ProcessDefinitionID:  task.ProcessDefinitionID,
		ProcessDefinitionKey: processDefinitionKey(task.ProcessDefinitionID),
		BizID:                firstNonBlank(stringValue(merged["bizId"]), task.BusinessKey),
		BizType:              stringValue(merged["bizType"]),
		Title:                stringValue(merged["title"]),
		PayloadRef:           firstNonBlank(stringValue(merged["payloadRef"]), stringValue(merged["bizId"]), task.BusinessKey),
		CreatedAt:            task.CreateTimeRaw,
		CompletedAt:          completedAt,
		Assignee:             assignee,
		Owner:                task.Owner,
		DelegationState:      task.DelegationState,
		TenantID:             firstNonBlank(stringValue(merged["tenantId"]), task.TenantID),
		SystemCode:           stringValue(merged["systemCode"]),
		FormKey:              task.FormKey,
	}
}

func (c *RESTClient) storeDoneTaskProjection(ctx stdcontext.Context, projection doneTaskProjection) {
	if orm.Redis == nil || c == nil || c.doneProjectionTTL <= 0 || strings.TrimSpace(projection.TaskID) == "" {
		return
	}
	taskKey := c.doneTaskProjectionKey(projection.TaskID)
	userKeys := c.doneTaskProjectionUserKeys(projection)
	if taskKey == "" || len(userKeys) == 0 {
		return
	}
	payload, err := json.Marshal(projection)
	if err != nil {
		return
	}
	score := float64(parseTime(projection.CompletedAt).UnixMilli())
	if score == 0 {
		score = float64(time.Now().UnixMilli())
	}
	client := orm.Redis.GetClient()
	_ = orm.Redis.Set(ctx, taskKey, string(payload), c.doneProjectionTTL)
	for _, userKey := range userKeys {
		if strings.TrimSpace(userKey) == "" {
			continue
		}
		_ = client.ZAdd(ctx, userKey, redisv9.Z{Score: score, Member: projection.TaskID}).Err()
		_ = client.Expire(ctx, userKey, c.doneProjectionTTL).Err()
	}
}

func (c *RESTClient) listDoneFromProjection(ctx stdcontext.Context, user *workflowcontext.UserContext, query *types.TaskQuery) (*types.TaskPage, map[string]struct{}) {
	if orm.Redis == nil || c == nil || user == nil || strings.TrimSpace(user.UserID) == "" {
		return nil, nil
	}
	userKey := c.doneTaskProjectionUserKey(user.UserID)
	if userKey == "" {
		return nil, nil
	}
	client := orm.Redis.GetClient()
	stop := int64(query.Start + max(1, query.Size)*3 - 1)
	taskIDs, err := client.ZRevRange(ctx, userKey, int64(query.Start), stop).Result()
	if err != nil || len(taskIDs) == 0 {
		return nil, nil
	}
	items := make([]types.TaskInfo, 0, len(taskIDs))
	seen := make(map[string]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		projection, ok := c.loadDoneTaskProjection(ctx, taskID)
		if !ok {
			continue
		}
		if !matchesDoneProjectionScope(projection, user) {
			continue
		}
		if !matchesDoneProjectionQuery(projection, query) {
			continue
		}
		items = append(items, projection.toTaskInfo())
		seen[strings.TrimSpace(taskID)] = struct{}{}
		if len(items) >= query.Size {
			break
		}
	}
	if len(items) == 0 {
		return nil, seen
	}
	total, _ := client.ZCard(ctx, userKey).Result()
	return &types.TaskPage{
		Items: items,
		Total: total,
		Start: query.Start,
		Size:  query.Size,
	}, seen
}

func (c *RESTClient) loadDoneTaskProjection(ctx stdcontext.Context, taskID string) (doneTaskProjection, bool) {
	if orm.Redis == nil || c == nil {
		return doneTaskProjection{}, false
	}
	key := c.doneTaskProjectionKey(taskID)
	if key == "" {
		return doneTaskProjection{}, false
	}
	value, err := orm.Redis.Get(ctx, key)
	if err != nil || strings.TrimSpace(value) == "" {
		return doneTaskProjection{}, false
	}
	var projection doneTaskProjection
	if err := json.Unmarshal([]byte(value), &projection); err != nil {
		return doneTaskProjection{}, false
	}
	if strings.TrimSpace(projection.TaskID) == "" {
		return doneTaskProjection{}, false
	}
	return projection, true
}

func (c *RESTClient) doneTaskProjectionKey(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if c == nil || taskID == "" {
		return ""
	}
	return firstNonBlank(c.doneTaskPrefix, defaultDoneTaskPrefix) + taskID
}

func (c *RESTClient) doneTaskProjectionUserKey(userID string) string {
	userID = strings.TrimSpace(userID)
	if c == nil || userID == "" {
		return ""
	}
	return firstNonBlank(c.doneUserPrefix, defaultDoneUserPrefix) + userID
}

func (c *RESTClient) doneTaskProjectionUserKeys(projection doneTaskProjection) []string {
	result := make([]string, 0, 2)
	if key := c.doneTaskProjectionUserKey(projection.Assignee); key != "" {
		result = append(result, key)
	}
	if key := c.doneTaskProjectionUserKey(projection.Owner); key != "" && key != firstNonBlank(result...) {
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

func (c *RESTClient) queryRuntimeTasks(ctx stdcontext.Context, query map[string]string) ([]runtimeTaskRecord, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/runtime/tasks", query, nil)
	if err != nil {
		return nil, err
	}
	return parseRuntimeTasks(body)
}

func (c *RESTClient) getTaskIdentityLinks(ctx stdcontext.Context, taskID string) ([]taskIdentityLinkRecord, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/runtime/tasks/"+taskID+"/identitylinks", nil, nil)
	if err != nil {
		return nil, err
	}
	links, err := parseTaskIdentityLinks(body)
	if err == nil {
		users, groups := splitCandidateIdentityLinks(links)
		c.storeCandidateTaskIndex(ctx, taskID, users, groups)
	}
	return links, err
}

func (c *RESTClient) queryHistoricTasks(ctx stdcontext.Context, query map[string]string) ([]historicTaskRecord, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/history/historic-task-instances", query, nil)
	if err != nil {
		return nil, err
	}
	return parseHistoricTasks(body)
}

func (c *RESTClient) queryHistoricActivities(ctx stdcontext.Context, query map[string]string) ([]historicActivityRecord, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/history/historic-activity-instances", query, nil)
	if err != nil {
		return nil, err
	}
	return parseHistoricActivities(body)
}

func parseVariableMap(raw interface{}) map[string]interface{} {
	rows, ok := raw.([]interface{})
	if !ok || len(rows) == 0 {
		return nil
	}
	result := make(map[string]interface{}, len(rows))
	for _, row := range rows {
		current, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		name := stringValue(current["name"])
		if name == "" {
			continue
		}
		result[name] = current["value"]
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (c *RESTClient) enrichTaskPage(ctx stdcontext.Context, user *workflowcontext.UserContext, page *types.TaskPage, cache map[string]*types.ProcessProgressSummary, includeProgress bool) error {
	if page == nil {
		return nil
	}
	for index := range page.Items {
		processInstanceID := strings.TrimSpace(page.Items[index].ProcessInstanceID)
		if processInstanceID == "" {
			continue
		}
		summaryProcessInstanceID := strings.TrimSpace(page.Items[index].RootProcessInstanceID)
		if summaryProcessInstanceID == "" {
			resolved, err := c.resolveSummaryProcessInstanceID(ctx, processInstanceID)
			if err == nil {
				summaryProcessInstanceID = strings.TrimSpace(resolved)
			}
		}
		if summaryProcessInstanceID == "" {
			summaryProcessInstanceID = processInstanceID
		}
		page.Items[index].RootProcessInstanceID = summaryProcessInstanceID
		if summary := lookupProcessSummary(cache, summaryProcessInstanceID); summary != nil {
			c.applyTaskSummary(&page.Items[index], summary, includeProgress)
			if cache != nil {
				cache[processInstanceID] = summary
			}
			continue
		}
		view, err := c.GetProgressView(ctx, summaryProcessInstanceID, user)
		if err != nil || view == nil {
			continue
		}
		summary := view.Summary
		if cache != nil {
			cache[summaryProcessInstanceID] = &summary
			cache[processInstanceID] = &summary
		}
		c.applyTaskSummary(&page.Items[index], &summary, includeProgress)
	}
	return nil
}

func (c *RESTClient) applyTaskSummary(info *types.TaskInfo, summary *types.ProcessProgressSummary, includeProgress bool) {
	if info == nil || summary == nil {
		return
	}
	info.Title = firstNonBlank(info.Title, summary.Title)
	info.BizID = firstNonBlank(info.BizID, summary.BizID)
	info.BizType = firstNonBlank(info.BizType, summary.BizType)
	info.TenantID = firstNonBlank(info.TenantID, summary.TenantID)
	copySummary := summarizeTaskListProgress(summary, includeProgress)
	info.Progress = &copySummary
}

func summarizeTaskListProgress(summary *types.ProcessProgressSummary, includeProgress bool) types.ProcessProgressSummary {
	if summary == nil {
		return types.ProcessProgressSummary{}
	}
	if includeProgress {
		return *summary
	}
	return types.ProcessProgressSummary{
		ProcessInstanceID:      summary.ProcessInstanceID,
		ProcessDefinitionID:    summary.ProcessDefinitionID,
		ProcessDefinitionKey:   summary.ProcessDefinitionKey,
		BizID:                  summary.BizID,
		BizType:                summary.BizType,
		Title:                  summary.Title,
		TenantID:               summary.TenantID,
		SystemCode:             summary.SystemCode,
		Status:                 summary.Status,
		ProgressPercent:        summary.ProgressPercent,
		StartTime:              summary.StartTime,
		EndTime:                summary.EndTime,
		CurrentActivityIDs:     append([]string(nil), summary.CurrentActivityIDs...),
		CurrentActivityNames:   append([]string(nil), summary.CurrentActivityNames...),
		CurrentAssignees:       append([]string(nil), summary.CurrentAssignees...),
		CurrentCandidateUsers:  append([]string(nil), summary.CurrentCandidateUsers...),
		CurrentCandidateGroups: append([]string(nil), summary.CurrentCandidateGroups...),
	}
}

func lookupProcessSummary(cache map[string]*types.ProcessProgressSummary, processInstanceID string) *types.ProcessProgressSummary {
	if len(cache) == 0 || strings.TrimSpace(processInstanceID) == "" {
		return nil
	}
	return cache[strings.TrimSpace(processInstanceID)]
}

func (c *RESTClient) toTaskInfo(runtimeTask runtimeTaskRecord, historicTask *historicTaskRecord, done bool) types.TaskInfo {
	info := types.TaskInfo{}
	if done && historicTask != nil {
		info.TaskID = historicTask.ID
		info.TaskName = historicTask.Name
		info.ActivityID = historicTask.TaskDefinitionKey
		info.ActivityName = firstNonBlank(historicTask.Name, historicTask.TaskDefinitionKey)
		info.ProcessInstanceID = historicTask.ProcessInstanceID
		info.ProcessDefinitionID = historicTask.ProcessDefinitionID
		info.CreatedAt = historicTask.CreateTimeRaw
		info.CompletedAt = historicTask.EndTimeRaw
		info.Assignee = historicTask.Assignee
		info.Owner = historicTask.Owner
		info.DelegationState = historicTask.DelegationState
		info.TenantID = historicTask.TenantID
		info.FormKey = historicTask.FormKey
		info.BizID = firstNonBlank(stringValue(historicTask.ProcessVariables["bizId"]), historicTask.BusinessKey)
		info.BizType = stringValue(historicTask.ProcessVariables["bizType"])
		info.Title = stringValue(historicTask.ProcessVariables["title"])
		info.PayloadRef = firstNonBlank(stringValue(historicTask.ProcessVariables["payloadRef"]), stringValue(historicTask.ProcessVariables["bizId"]), historicTask.BusinessKey)
		return info
	}

	info.TaskID = runtimeTask.ID
	info.TaskName = runtimeTask.Name
	info.ActivityID = runtimeTask.TaskDefinitionKey
	info.ActivityName = firstNonBlank(runtimeTask.Name, runtimeTask.TaskDefinitionKey)
	info.ProcessInstanceID = runtimeTask.ProcessInstanceID
	info.ProcessDefinitionID = runtimeTask.ProcessDefinitionID
	info.CreatedAt = runtimeTask.CreateTimeRaw
	info.Assignee = runtimeTask.Assignee
	info.Owner = runtimeTask.Owner
	info.DelegationState = runtimeTask.DelegationState
	info.TenantID = runtimeTask.TenantID
	info.FormKey = runtimeTask.FormKey
	info.BizID = firstNonBlank(stringValue(runtimeTask.ProcessVariables["bizId"]), runtimeTask.BusinessKey)
	info.BizType = stringValue(runtimeTask.ProcessVariables["bizType"])
	info.Title = stringValue(runtimeTask.ProcessVariables["title"])
	info.PayloadRef = firstNonBlank(stringValue(runtimeTask.ProcessVariables["payloadRef"]), stringValue(runtimeTask.ProcessVariables["bizId"]), runtimeTask.BusinessKey)
	return info
}

func candidateGroups(user *workflowcontext.UserContext, groupPrefix, rolePrefix string) []string {
	if user == nil {
		return nil
	}
	normalizer := identity.NewNormalizerFromConfig()
	normalizedGroups := normalizer.NormalizeGroups(user.Groups)
	normalizedRoles := normalizer.NormalizeRoles(user.Roles)
	result := make([]string, 0, len(normalizedGroups)+len(normalizedRoles))
	for _, group := range normalizedGroups {
		result = append(result, groupPrefix+group)
	}
	for _, role := range normalizedRoles {
		result = append(result, rolePrefix+role)
	}
	return result
}

func (c *RESTClient) buildRuntimeTaskQuery(query *types.TaskQuery, base map[string]string) map[string]string {
	result := cloneStringMap(base)
	result["start"] = fmt.Sprintf("%d", query.Start)
	result["size"] = normalizePageSize(query.Size)
	result["includeProcessVariables"] = "true"
	if query.ProcessDefinitionKey != "" {
		result["processDefinitionKey"] = query.ProcessDefinitionKey
	}
	if query.ActivityID != "" {
		result["taskDefinitionKey"] = query.ActivityID
	}
	if query.CreatedAfter != "" {
		result["createdAfter"] = query.CreatedAfter
	}
	if query.CreatedBefore != "" {
		result["createdBefore"] = query.CreatedBefore
	}
	return result
}

func (c *RESTClient) buildHistoricTaskQuery(query *types.TaskQuery, base map[string]string) map[string]string {
	result := cloneStringMap(base)
	result["start"] = fmt.Sprintf("%d", query.Start)
	result["size"] = normalizePageSize(query.Size)
	result["includeProcessVariables"] = "true"
	if query.ProcessDefinitionKey != "" {
		result["processDefinitionKey"] = query.ProcessDefinitionKey
	}
	if query.ActivityID != "" {
		result["taskDefinitionKey"] = query.ActivityID
	}
	if query.CreatedAfter != "" {
		result["taskCreatedAfter"] = query.CreatedAfter
	}
	if query.CreatedBefore != "" {
		result["taskCreatedBefore"] = query.CreatedBefore
	}
	if query.CompletedAfter != "" {
		result["taskCompletedAfter"] = query.CompletedAfter
	}
	if query.CompletedBefore != "" {
		result["taskCompletedBefore"] = query.CompletedBefore
	}
	switch normalizeStatus(query.Status) {
	case "COMPLETED", "FINISHED":
		result["processFinished"] = "true"
	case "RUNNING", "ACTIVE":
		result["processFinished"] = "false"
	}
	return result
}

func toFlowableVariables(values map[string]interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(values))
	for key, value := range values {
		result = append(result, map[string]interface{}{
			"name":  key,
			"value": value,
		})
	}
	return result
}

func normalizePageSize(size int) string {
	if size <= 0 {
		return fmt.Sprintf("%d", defaultTaskPageSize)
	}
	if size > maxTaskPageSize {
		size = maxTaskPageSize
	}
	return fmt.Sprintf("%d", size)
}

func normalizeTaskQuery(query *types.TaskQuery) *types.TaskQuery {
	if query == nil {
		return &types.TaskQuery{
			Start: 0,
			Size:  defaultTaskPageSize,
		}
	}
	if query.Size <= 0 {
		query.Size = defaultTaskPageSize
	}
	if query.Size > maxTaskPageSize {
		query.Size = maxTaskPageSize
	}
	if query.Start < 0 {
		query.Start = 0
	}
	query.Title = strings.TrimSpace(query.Title)
	query.BizType = strings.TrimSpace(query.BizType)
	query.ProcessDefinitionKey = strings.TrimSpace(query.ProcessDefinitionKey)
	query.ActivityID = strings.TrimSpace(query.ActivityID)
	query.Status = strings.TrimSpace(query.Status)
	query.CreatedAfter = strings.TrimSpace(query.CreatedAfter)
	query.CreatedBefore = strings.TrimSpace(query.CreatedBefore)
	query.CompletedAfter = strings.TrimSpace(query.CompletedAfter)
	query.CompletedBefore = strings.TrimSpace(query.CompletedBefore)
	return query
}

func sortTaskInfos(items []types.TaskInfo, done bool) {
	sort.Slice(items, func(i, j int) bool {
		if done {
			return items[i].CompletedAt > items[j].CompletedAt
		}
		return items[i].CreatedAt < items[j].CreatedAt
	})
}

func putIfNonBlank(target map[string]interface{}, key, value string) {
	if target == nil || strings.TrimSpace(value) == "" {
		return
	}
	target[key] = strings.TrimSpace(value)
}

func stringValue(value interface{}) string {
	switch current := value.(type) {
	case string:
		return strings.TrimSpace(current)
	default:
		if current == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprintf("%v", current))
	}
}

func asObjectSlice(value interface{}) []map[string]interface{} {
	rows, ok := value.([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		current, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, current)
	}
	return result
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func matchesTaskQuery(info types.TaskInfo, query *types.TaskQuery) bool {
	if query == nil {
		return true
	}
	if query.Title != "" && !strings.Contains(strings.ToLower(info.Title), strings.ToLower(query.Title)) {
		return false
	}
	if query.BizType != "" && !strings.EqualFold(strings.TrimSpace(info.BizType), query.BizType) {
		return false
	}
	switch normalizeStatus(query.Status) {
	case "", "TODO", "ACTIVE", "RUNNING", "DONE", "COMPLETED", "FINISHED":
		return true
	default:
		return false
	}
}

func matchesDoneProjectionScope(projection doneTaskProjection, user *workflowcontext.UserContext) bool {
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

func matchesDoneProjectionQuery(projection doneTaskProjection, query *types.TaskQuery) bool {
	return matchesTaskQuery(projection.toTaskInfo(), query) &&
		(query.ProcessDefinitionKey == "" || strings.EqualFold(strings.TrimSpace(projection.ProcessDefinitionKey), query.ProcessDefinitionKey)) &&
		(query.ActivityID == "" || strings.EqualFold(strings.TrimSpace(projection.ActivityID), query.ActivityID))
}

func mergeDoneTaskInfos(projected []types.TaskInfo, fallback []types.TaskInfo, projectedIDs map[string]struct{}) []types.TaskInfo {
	result := make([]types.TaskInfo, 0, len(projected)+len(fallback))
	result = append(result, projected...)
	for _, item := range fallback {
		if _, ok := projectedIDs[strings.TrimSpace(item.TaskID)]; ok {
			continue
		}
		result = append(result, item)
	}
	return result
}

func matchesTodoStatus(status string) bool {
	switch normalizeStatus(status) {
	case "", "TODO", "ACTIVE", "RUNNING":
		return true
	default:
		return false
	}
}

func projectedPageItems(page *types.TaskPage) []types.TaskInfo {
	if page == nil {
		return nil
	}
	return page.Items
}

func normalizeStatus(status string) string {
	return strings.ToUpper(strings.TrimSpace(status))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (c *RESTClient) ResolveProcessInstanceIDByBizID(ctx stdcontext.Context, bizID string, user *workflowcontext.UserContext) (string, error) {
	if strings.TrimSpace(bizID) == "" {
		return "", errors.New("biz id is required")
	}
	tenantID := ""
	if user != nil {
		tenantID = user.TenantID
	}
	runtimeQuery := map[string]string{
		"businessKey": strings.TrimSpace(bizID),
		"sort":        "id",
		"order":       "desc",
		"size":        "1",
	}
	if strings.TrimSpace(tenantID) != "" {
		runtimeQuery["tenantId"] = strings.TrimSpace(tenantID)
	}
	runtime, err := c.queryProcessInstances(ctx, "/runtime/process-instances", runtimeQuery)
	if err == nil && len(runtime) > 0 && runtime[0].ID != "" {
		return runtime[0].ID, nil
	}
	if strings.TrimSpace(tenantID) != "" {
		runtime, err = c.queryProcessInstances(ctx, "/runtime/process-instances", map[string]string{
			"businessKey": strings.TrimSpace(bizID),
			"sort":        "id",
			"order":       "desc",
			"size":        "1",
		})
		if err == nil && len(runtime) > 0 && runtime[0].ID != "" {
			return runtime[0].ID, nil
		}
	}
	historicQuery := map[string]string{
		"businessKey": strings.TrimSpace(bizID),
		"sort":        "startTime",
		"order":       "desc",
		"size":        "1",
	}
	if strings.TrimSpace(tenantID) != "" {
		historicQuery["tenantId"] = strings.TrimSpace(tenantID)
	}
	historic, err := c.queryProcessInstances(ctx, "/history/historic-process-instances", historicQuery)
	if (err != nil || len(historic) == 0 || historic[0].ID == "") && strings.TrimSpace(tenantID) != "" {
		historic, err = c.queryProcessInstances(ctx, "/history/historic-process-instances", map[string]string{
			"businessKey": strings.TrimSpace(bizID),
			"sort":        "startTime",
			"order":       "desc",
			"size":        "1",
		})
	}
	if err != nil {
		return "", err
	}
	if len(historic) == 0 || historic[0].ID == "" {
		return "", fmt.Errorf("process instance not found by biz id: %s", strings.TrimSpace(bizID))
	}
	return historic[0].ID, nil
}

func (c *RESTClient) queryProcessInstances(ctx stdcontext.Context, path string, query map[string]string) ([]processInstanceRecord, error) {
	body, err := c.doJSON(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err == nil {
		if rows, ok := payload["data"].([]interface{}); ok {
			items := make([]processInstanceRecord, 0, len(rows))
			for _, row := range rows {
				current, ok := row.(map[string]interface{})
				if !ok {
					continue
				}
				items = append(items, parseProcessInstanceMap(current))
			}
			return items, nil
		}
	}
	return nil, errors.New("unexpected process instance query response")
}

func (c *RESTClient) queryProcessDefinitions(ctx stdcontext.Context, query map[string]string) ([]processDefinitionRecord, error) {
	body, err := c.doJSON(ctx, http.MethodGet, "/repository/process-definitions", query, nil)
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err == nil {
		if rows, ok := payload["data"].([]interface{}); ok {
			items := make([]processDefinitionRecord, 0, len(rows))
			for _, row := range rows {
				current, ok := row.(map[string]interface{})
				if !ok {
					continue
				}
				items = append(items, processDefinitionRecord{
					ID:       stringValue(current["id"]),
					Key:      stringValue(current["key"]),
					Version:  int64Value(current["version"]),
					TenantID: stringValue(current["tenantId"]),
				})
			}
			sort.Slice(items, func(i, j int) bool {
				return items[i].Version > items[j].Version
			})
			return items, nil
		}
	}
	return nil, errors.New("unexpected process definition query response")
}

type processInstanceRecord struct {
	ID                     string
	ProcessDefinitionID    string
	ProcessDefinitionKey   string
	BusinessKey            string
	SuperProcessInstanceID string
	TenantID               string
	StartTime              time.Time
	EndTime                *time.Time
	DeleteReason           string
}

type processDefinitionRecord struct {
	ID       string
	Key      string
	Version  int64
	TenantID string
}

type runtimeTaskRecord struct {
	ID                  string
	Name                string
	Description         string
	TaskDefinitionKey   string
	Assignee            string
	Owner               string
	DelegationState     string
	CandidateUsers      []string
	CandidateGroups     []string
	ProcessInstanceID   string
	ProcessDefinitionID string
	TenantID            string
	FormKey             string
	BusinessKey         string
	ProcessVariables    map[string]interface{}
	CreateTime          time.Time
	CreateTimeRaw       string
}

type taskIdentityLinkRecord struct {
	User  string
	Group string
	Type  string
}

type historicTaskRecord struct {
	ID                  string
	Name                string
	Description         string
	TaskDefinitionKey   string
	Assignee            string
	Owner               string
	DelegationState     string
	ProcessInstanceID   string
	ProcessDefinitionID string
	TenantID            string
	FormKey             string
	BusinessKey         string
	ProcessVariables    map[string]interface{}
	StartTime           time.Time
	StartTimeRaw        string
	CreateTimeRaw       string
	EndTime             *time.Time
	EndTimeRaw          string
	DurationInMillis    int64
}

type historicActivityRecord struct {
	ID                      string
	ActivityID              string
	ActivityName            string
	ActivityType            string
	ProcessInstanceID       string
	CalledProcessInstanceID string
	StartTime               time.Time
	EndTime                 *time.Time
	StartTimeRaw            string
	EndTimeRaw              string
	TenantID                string
}

func parseProcessInstance(body []byte) processInstanceRecord {
	var raw map[string]interface{}
	_ = json.Unmarshal(body, &raw)
	return parseProcessInstanceMap(raw)
}

func parseProcessInstanceMap(raw map[string]interface{}) processInstanceRecord {
	record := processInstanceRecord{
		ID:                     stringValue(raw["id"]),
		ProcessDefinitionID:    stringValue(raw["processDefinitionId"]),
		ProcessDefinitionKey:   processDefinitionKey(stringValue(raw["processDefinitionId"])),
		BusinessKey:            stringValue(raw["businessKey"]),
		SuperProcessInstanceID: stringValue(raw["superProcessInstanceId"]),
		TenantID:               stringValue(raw["tenantId"]),
		StartTime:              parseTime(stringValue(raw["startTime"])),
		DeleteReason:           stringValue(raw["deleteReason"]),
	}
	if endTimeRaw := stringValue(raw["endTime"]); endTimeRaw != "" {
		endTime := parseTime(endTimeRaw)
		record.EndTime = &endTime
	}
	return record
}

func parseRuntimeTasks(body []byte) ([]runtimeTaskRecord, error) {
	data, err := extractDataArray(body)
	if err != nil {
		return nil, err
	}
	result := make([]runtimeTaskRecord, 0, len(data))
	for _, item := range data {
		result = append(result, parseRuntimeTask(item))
	}
	return result, nil
}

func parseRuntimeTask(raw map[string]interface{}) runtimeTaskRecord {
	createTimeRaw := firstNonBlank(stringValue(raw["createTime"]), stringValue(raw["startTime"]))
	return runtimeTaskRecord{
		ID:                  stringValue(raw["id"]),
		Name:                stringValue(raw["name"]),
		Description:         stringValue(raw["description"]),
		TaskDefinitionKey:   stringValue(raw["taskDefinitionKey"]),
		Assignee:            stringValue(raw["assignee"]),
		Owner:               stringValue(raw["owner"]),
		DelegationState:     stringValue(raw["delegationState"]),
		ProcessInstanceID:   stringValue(raw["processInstanceId"]),
		ProcessDefinitionID: stringValue(raw["processDefinitionId"]),
		TenantID:            stringValue(raw["tenantId"]),
		FormKey:             stringValue(raw["formKey"]),
		BusinessKey:         stringValue(raw["businessKey"]),
		ProcessVariables:    parseVariableMap(raw["processVariables"]),
		CreateTime:          parseTime(createTimeRaw),
		CreateTimeRaw:       createTimeRaw,
	}
}

func parseHistoricActivities(body []byte) ([]historicActivityRecord, error) {
	data, err := extractDataArray(body)
	if err != nil {
		return nil, err
	}
	result := make([]historicActivityRecord, 0, len(data))
	for _, item := range data {
		result = append(result, parseHistoricActivity(item))
	}
	return result, nil
}

func parseHistoricActivity(raw map[string]interface{}) historicActivityRecord {
	record := historicActivityRecord{
		ID:                      stringValue(raw["id"]),
		ActivityID:              stringValue(raw["activityId"]),
		ActivityName:            stringValue(raw["activityName"]),
		ActivityType:            stringValue(raw["activityType"]),
		ProcessInstanceID:       stringValue(raw["processInstanceId"]),
		CalledProcessInstanceID: stringValue(raw["calledProcessInstanceId"]),
		StartTimeRaw:            stringValue(raw["startTime"]),
		EndTimeRaw:              stringValue(raw["endTime"]),
		TenantID:                stringValue(raw["tenantId"]),
	}
	record.StartTime = parseTime(record.StartTimeRaw)
	if record.EndTimeRaw != "" {
		endTime := parseTime(record.EndTimeRaw)
		record.EndTime = &endTime
	}
	return record
}

func parseTaskIdentityLinks(body []byte) ([]taskIdentityLinkRecord, error) {
	data, err := extractDataArray(body)
	if err != nil {
		return nil, err
	}
	result := make([]taskIdentityLinkRecord, 0, len(data))
	for _, item := range data {
		result = append(result, taskIdentityLinkRecord{
			User:  stringValue(item["user"]),
			Group: stringValue(item["group"]),
			Type:  stringValue(item["type"]),
		})
	}
	return result, nil
}

func splitCandidateIdentityLinks(links []taskIdentityLinkRecord) ([]string, []string) {
	users := make([]string, 0)
	groups := make([]string, 0)
	for _, link := range links {
		if !strings.EqualFold(link.Type, "candidate") {
			continue
		}
		if link.User != "" {
			users = appendIfMissing(users, link.User)
		}
		if link.Group != "" {
			groups = appendIfMissing(groups, link.Group)
		}
	}
	return users, groups
}

func parseHistoricTasks(body []byte) ([]historicTaskRecord, error) {
	data, err := extractDataArray(body)
	if err != nil {
		return nil, err
	}
	result := make([]historicTaskRecord, 0, len(data))
	for _, item := range data {
		startTimeRaw := firstNonBlank(stringValue(item["startTime"]), stringValue(item["createTime"]))
		record := historicTaskRecord{
			ID:                  stringValue(item["id"]),
			Name:                stringValue(item["name"]),
			Description:         stringValue(item["description"]),
			TaskDefinitionKey:   stringValue(item["taskDefinitionKey"]),
			Assignee:            stringValue(item["assignee"]),
			Owner:               stringValue(item["owner"]),
			DelegationState:     stringValue(item["delegationState"]),
			ProcessInstanceID:   stringValue(item["processInstanceId"]),
			ProcessDefinitionID: stringValue(item["processDefinitionId"]),
			TenantID:            stringValue(item["tenantId"]),
			FormKey:             stringValue(item["formKey"]),
			BusinessKey:         firstNonBlank(stringValue(item["businessKey"]), stringValue(item["processBusinessKey"])),
			ProcessVariables:    parseVariableMap(item["processVariables"]),
			StartTime:           parseTime(startTimeRaw),
			StartTimeRaw:        startTimeRaw,
			CreateTimeRaw:       stringValue(item["createTime"]),
			DurationInMillis:    int64Value(item["durationInMillis"]),
		}
		if endTimeRaw := stringValue(item["endTime"]); endTimeRaw != "" {
			endTime := parseTime(endTimeRaw)
			record.EndTime = &endTime
			record.EndTimeRaw = endTimeRaw
		}
		result = append(result, record)
	}
	return result, nil
}

func extractDataArray(body []byte) ([]map[string]interface{}, error) {
	if len(body) == 0 {
		return nil, nil
	}
	var array []map[string]interface{}
	if err := json.Unmarshal(body, &array); err == nil {
		return array, nil
	}
	var root map[string]interface{}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	if raw, ok := root["data"].([]interface{}); ok {
		result := make([]map[string]interface{}, 0, len(raw))
		for _, item := range raw {
			row, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			result = append(result, row)
		}
		return result, nil
	}
	return nil, nil
}

func parseTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed
	}
	return time.Time{}
}

func int64Value(value interface{}) int64 {
	switch current := value.(type) {
	case float64:
		return int64(current)
	case int64:
		return current
	case int:
		return int64(current)
	default:
		return 0
	}
}

func processDefinitionKey(definitionID string) string {
	if definitionID == "" {
		return ""
	}
	parts := strings.Split(definitionID, ":")
	if len(parts) > 0 {
		return parts[0]
	}
	return definitionID
}
