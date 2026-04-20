package flowable

import (
	stdcontext "context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/types"
)

func (c *RESTClient) GetProgressView(ctx stdcontext.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessProgressViewResponse, error) {
	snapshot, err := c.loadSnapshot(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	summary := buildSummary(snapshot)
	c.storeProcessSummaryCache(ctx, &summary)
	steps := buildSteps(snapshot)
	return &types.ProcessProgressViewResponse{
		Summary:              summary,
		Steps:                steps,
		CompletedActivityIDs: completedActivityIDs(snapshot),
		CurrentActivityIDs:   currentActivityIDs(snapshot),
	}, nil
}

func (c *RESTClient) GetProgressViewByBizID(ctx stdcontext.Context, bizID string, user *workflowcontext.UserContext) (*types.ProcessProgressViewResponse, error) {
	processInstanceID, err := c.ResolveProcessInstanceIDByBizID(ctx, bizID, user)
	if err != nil {
		return nil, err
	}
	return c.GetProgressView(ctx, processInstanceID, user)
}

func (c *RESTClient) GetProgressTimeline(ctx stdcontext.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessProgressTimelineResponse, error) {
	snapshot, err := c.loadSnapshot(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	summary := buildSummary(snapshot)
	c.storeProcessSummaryCache(ctx, &summary)
	return &types.ProcessProgressTimelineResponse{
		Summary: summary,
		Items:   buildTimeline(snapshot),
	}, nil
}

func (c *RESTClient) GetProgressTimelineByBizID(ctx stdcontext.Context, bizID string, user *workflowcontext.UserContext) (*types.ProcessProgressTimelineResponse, error) {
	processInstanceID, err := c.ResolveProcessInstanceIDByBizID(ctx, bizID, user)
	if err != nil {
		return nil, err
	}
	return c.GetProgressTimeline(ctx, processInstanceID, user)
}

func (c *RESTClient) loadSnapshot(ctx stdcontext.Context, processInstanceID string) (*processSnapshot, error) {
	process, err := c.getProcessInstance(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	variables, err := c.getProcessVariables(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	runtimeTasks, err := c.queryRuntimeTasks(ctx, map[string]string{
		"processInstanceId": processInstanceID,
		"tenantId":          process.TenantID,
		"size":              "200",
		"sort":              "createTime",
		"order":             "asc",
	})
	if err != nil {
		return nil, err
	}
	for index := range runtimeTasks {
		links, linkErr := c.getTaskIdentityLinks(ctx, runtimeTasks[index].ID)
		if linkErr != nil {
			continue
		}
		runtimeTasks[index].CandidateUsers, runtimeTasks[index].CandidateGroups = splitCandidateIdentityLinks(links)
	}
	historicTasks, err := c.queryHistoricTasks(ctx, map[string]string{
		"processInstanceId": processInstanceID,
		"tenantId":          process.TenantID,
		"finished":          "true",
		"size":              "200",
		"sort":              "endTime",
		"order":             "asc",
	})
	if err != nil {
		return nil, err
	}
	historicActivities, err := c.queryHistoricActivities(ctx, map[string]string{
		"processInstanceId": processInstanceID,
		"tenantId":          process.TenantID,
		"size":              "500",
		"sort":              "startTime",
		"order":             "asc",
	})
	if err != nil {
		historicActivities = nil
	}
	definitionXML, err := c.getRawDefinitionXML(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	model, err := parseBPMNModel(definitionXML)
	if err != nil {
		return nil, err
	}
	snapshot := &processSnapshot{
		process:            process,
		variables:          variables,
		runtimeTasks:       runtimeTasks,
		historicTasks:      historicTasks,
		historicActivities: historicActivities,
		model:              model,
	}
	c.expandCallActivitySnapshot(ctx, snapshot)
	return snapshot, nil
}

type callActivityExpansion struct {
	VisibleNodes       []modelNode
	NodesByID          map[string]modelNode
	Outgoing           map[string][]sequenceFlow
	ParentSubProcess   map[string]string
	ParentCallActivity map[string]string
	SubProcessEntry    map[string][]string
	SubProcessExit     map[string][]string
	CallActivityEntry  map[string][]string
	CallActivityExit   map[string][]string
	EntryNodeIDs       []string
	ExitNodeIDs        []string
	RuntimeTasks       []runtimeTaskRecord
	HistoricTasks      []historicTaskRecord
}

func (c *RESTClient) expandCallActivitySnapshot(ctx stdcontext.Context, snapshot *processSnapshot) {
	if snapshot == nil || snapshot.model == nil {
		return
	}
	activityMap, err := c.queryCallActivityBindings(ctx, snapshot.process.ID)
	if err != nil {
		activityMap = map[string][]historicActivityRecord{}
	}
	visible := make([]modelNode, 0, len(snapshot.model.VisibleNodes))
	for _, node := range snapshot.model.VisibleNodes {
		if node.Type != "callActivity" {
			visible = append(visible, node)
			continue
		}
		expansion, ok := c.buildCallActivityExpansion(ctx, snapshot, node, activityMap[node.ID])
		if !ok {
			visible = append(visible, node)
			continue
		}
		container := snapshot.model.NodesByID[node.ID]
		container.Visible = false
		snapshot.model.NodesByID[node.ID] = container
		if len(expansion.EntryNodeIDs) > 0 {
			snapshot.model.CallActivityEntry[node.ID] = append([]string(nil), expansion.EntryNodeIDs...)
		}
		if len(expansion.ExitNodeIDs) > 0 {
			snapshot.model.CallActivityExit[node.ID] = append([]string(nil), expansion.ExitNodeIDs...)
		}
		mergeModelNodes(snapshot.model.NodesByID, expansion.NodesByID)
		mergeOutgoingFlows(snapshot.model.Outgoing, expansion.Outgoing)
		mergeStringMaps(snapshot.model.ParentSubProcess, expansion.ParentSubProcess)
		mergeStringMaps(snapshot.model.ParentCallActivity, expansion.ParentCallActivity)
		mergeStringSliceMaps(snapshot.model.SubProcessEntry, expansion.SubProcessEntry)
		mergeStringSliceMaps(snapshot.model.SubProcessExit, expansion.SubProcessExit)
		mergeStringSliceMaps(snapshot.model.CallActivityEntry, expansion.CallActivityEntry)
		mergeStringSliceMaps(snapshot.model.CallActivityExit, expansion.CallActivityExit)
		visible = append(visible, expansion.VisibleNodes...)
		snapshot.runtimeTasks = append(snapshot.runtimeTasks, expansion.RuntimeTasks...)
		snapshot.historicTasks = append(snapshot.historicTasks, expansion.HistoricTasks...)
	}
	snapshot.model.VisibleNodes = visible
	sort.Slice(snapshot.runtimeTasks, func(i, j int) bool {
		return snapshot.runtimeTasks[i].CreateTime.Before(snapshot.runtimeTasks[j].CreateTime)
	})
	sort.Slice(snapshot.historicTasks, func(i, j int) bool {
		left := snapshot.historicTasks[i].EndTime
		right := snapshot.historicTasks[j].EndTime
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		return left.Before(*right)
	})
}

func (c *RESTClient) buildCallActivityExpansion(ctx stdcontext.Context, snapshot *processSnapshot, node modelNode, activities []historicActivityRecord) (*callActivityExpansion, bool) {
	resolvedCalledElement := resolveCalledElement(node.CalledElement, snapshot.variables)
	childProcessIDs := uniqueCalledProcessInstanceIDs(activities)
	modelSource, ok := c.loadCallActivityModelSource(ctx, resolvedCalledElement, snapshot.process.TenantID, childProcessIDs)
	if !ok || modelSource == nil {
		return nil, false
	}
	expansion, ok := normalizeCallActivityExpansion(node.ID, modelSource)
	if !ok {
		return nil, false
	}
	for _, childProcessID := range childProcessIDs {
		childProcess, err := c.getProcessInstance(ctx, childProcessID)
		if err != nil {
			continue
		}
		runtimeTasks, err := c.queryRuntimeTasks(ctx, map[string]string{
			"processInstanceId": childProcessID,
			"tenantId":          childProcess.TenantID,
			"size":              "200",
			"sort":              "createTime",
			"order":             "asc",
		})
		if err == nil {
			for index := range runtimeTasks {
				links, linkErr := c.getTaskIdentityLinks(ctx, runtimeTasks[index].ID)
				if linkErr == nil {
					runtimeTasks[index].CandidateUsers, runtimeTasks[index].CandidateGroups = splitCandidateIdentityLinks(links)
				}
				runtimeTasks[index] = normalizeRuntimeTaskForCallActivity(node.ID, runtimeTasks[index])
			}
			expansion.RuntimeTasks = append(expansion.RuntimeTasks, runtimeTasks...)
		}
		historicTasks, err := c.queryHistoricTasks(ctx, map[string]string{
			"processInstanceId": childProcessID,
			"tenantId":          childProcess.TenantID,
			"finished":          "true",
			"size":              "200",
			"sort":              "endTime",
			"order":             "asc",
		})
		if err == nil {
			for index := range historicTasks {
				historicTasks[index] = normalizeHistoricTaskForCallActivity(node.ID, historicTasks[index])
			}
			expansion.HistoricTasks = append(expansion.HistoricTasks, historicTasks...)
		}
	}
	return expansion, true
}

func (c *RESTClient) loadCallActivityModelSource(ctx stdcontext.Context, calledElement, tenantID string, childProcessIDs []string) (*parsedModel, bool) {
	for _, childProcessID := range childProcessIDs {
		definitionXML, err := c.getRawDefinitionXML(ctx, childProcessID)
		if err != nil {
			continue
		}
		model, parseErr := parseBPMNModel(definitionXML)
		if parseErr == nil && model != nil {
			return model, true
		}
	}
	calledElement = strings.TrimSpace(calledElement)
	if calledElement == "" {
		return nil, false
	}
	definitionXML, err := c.getDefinitionXMLByKey(ctx, calledElement, tenantID)
	if err != nil {
		return nil, false
	}
	model, err := parseBPMNModel(definitionXML)
	if err != nil {
		return nil, false
	}
	return model, true
}

func (c *RESTClient) getDefinitionXMLByKey(ctx stdcontext.Context, processDefinitionKey, tenantID string) ([]byte, error) {
	definitionID, err := c.resolveProcessDefinitionID(ctx, processDefinitionKey, tenantID)
	if err != nil {
		return nil, err
	}
	return c.doRaw(ctx, http.MethodGet, "/repository/process-definitions/"+definitionID+"/resourcedata", nil, nil, "application/xml")
}

func (c *RESTClient) queryCallActivityBindings(ctx stdcontext.Context, processInstanceID string) (map[string][]historicActivityRecord, error) {
	activities, err := c.queryHistoricActivities(ctx, map[string]string{
		"processInstanceId": strings.TrimSpace(processInstanceID),
		"activityType":      "callActivity",
		"size":              "200",
		"sort":              "startTime",
		"order":             "asc",
	})
	if err != nil {
		return nil, err
	}
	result := make(map[string][]historicActivityRecord)
	for _, activity := range activities {
		if strings.TrimSpace(activity.ActivityID) == "" {
			continue
		}
		result[activity.ActivityID] = append(result[activity.ActivityID], activity)
	}
	return result, nil
}

func normalizeCallActivityExpansion(callActivityID string, childModel *parsedModel) (*callActivityExpansion, bool) {
	if childModel == nil || strings.TrimSpace(callActivityID) == "" {
		return nil, false
	}
	expansion := &callActivityExpansion{
		NodesByID:          map[string]modelNode{},
		Outgoing:           map[string][]sequenceFlow{},
		ParentSubProcess:   map[string]string{},
		ParentCallActivity: map[string]string{},
		SubProcessEntry:    map[string][]string{},
		SubProcessExit:     map[string][]string{},
		CallActivityEntry:  map[string][]string{},
		CallActivityExit:   map[string][]string{},
	}
	startSet := make(map[string]struct{}, len(childModel.StartNodeIDs))
	endSet := make(map[string]struct{}, len(childModel.EndNodeIDs))
	for _, nodeID := range childModel.StartNodeIDs {
		startSet[nodeID] = struct{}{}
		expansion.EntryNodeIDs = append(expansion.EntryNodeIDs, namespaceCallActivityNodeID(callActivityID, nodeID))
	}
	for _, nodeID := range childModel.EndNodeIDs {
		endSet[nodeID] = struct{}{}
		expansion.ExitNodeIDs = append(expansion.ExitNodeIDs, namespaceCallActivityNodeID(callActivityID, nodeID))
	}
	for originalID, node := range childModel.NodesByID {
		namespacedID := namespaceCallActivityNodeID(callActivityID, originalID)
		cloned := node
		cloned.ID = namespacedID
		if _, ok := startSet[originalID]; ok {
			cloned.Visible = false
		}
		if _, ok := endSet[originalID]; ok {
			cloned.Visible = false
		}
		expansion.NodesByID[namespacedID] = cloned
		if parentID := strings.TrimSpace(childModel.ParentSubProcess[originalID]); parentID != "" {
			expansion.ParentSubProcess[namespacedID] = namespaceCallActivityNodeID(callActivityID, parentID)
		} else {
			expansion.ParentCallActivity[namespacedID] = callActivityID
		}
	}
	for sourceID, flows := range childModel.Outgoing {
		namespacedSource := namespaceCallActivityNodeID(callActivityID, sourceID)
		for _, flow := range flows {
			expansion.Outgoing[namespacedSource] = append(expansion.Outgoing[namespacedSource], sequenceFlow{
				ID:                  namespaceCallActivityNodeID(callActivityID, flow.ID),
				SourceRef:           namespacedSource,
				TargetRef:           namespaceCallActivityNodeID(callActivityID, flow.TargetRef),
				ConditionExpression: flow.ConditionExpression,
			})
		}
	}
	for nodeID, entries := range childModel.SubProcessEntry {
		expansion.SubProcessEntry[namespaceCallActivityNodeID(callActivityID, nodeID)] = namespaceCallActivityNodeIDs(callActivityID, entries)
	}
	for nodeID, exits := range childModel.SubProcessExit {
		expansion.SubProcessExit[namespaceCallActivityNodeID(callActivityID, nodeID)] = namespaceCallActivityNodeIDs(callActivityID, exits)
	}
	for nodeID, entries := range childModel.CallActivityEntry {
		expansion.CallActivityEntry[namespaceCallActivityNodeID(callActivityID, nodeID)] = namespaceCallActivityNodeIDs(callActivityID, entries)
	}
	for nodeID, exits := range childModel.CallActivityExit {
		expansion.CallActivityExit[namespaceCallActivityNodeID(callActivityID, nodeID)] = namespaceCallActivityNodeIDs(callActivityID, exits)
	}
	for _, node := range childModel.VisibleNodes {
		if _, ok := startSet[node.ID]; ok {
			continue
		}
		if _, ok := endSet[node.ID]; ok {
			continue
		}
		cloned := expansion.NodesByID[namespaceCallActivityNodeID(callActivityID, node.ID)]
		if cloned.Visible {
			expansion.VisibleNodes = append(expansion.VisibleNodes, cloned)
		}
	}
	return expansion, len(expansion.VisibleNodes) > 0 || len(expansion.EntryNodeIDs) > 0
}

func normalizeRuntimeTaskForCallActivity(callActivityID string, task runtimeTaskRecord) runtimeTaskRecord {
	task.TaskDefinitionKey = namespaceCallActivityNodeID(callActivityID, task.TaskDefinitionKey)
	return task
}

func normalizeHistoricTaskForCallActivity(callActivityID string, task historicTaskRecord) historicTaskRecord {
	task.TaskDefinitionKey = namespaceCallActivityNodeID(callActivityID, task.TaskDefinitionKey)
	return task
}

func resolveCalledElement(calledElement string, variables map[string]interface{}) string {
	value := strings.TrimSpace(calledElement)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}"))
		if name == "" || strings.ContainsAny(name, " ()+-*/&|=!<>") {
			return ""
		}
		return stringValue(variables[name])
	}
	return value
}

func uniqueCalledProcessInstanceIDs(activities []historicActivityRecord) []string {
	result := make([]string, 0, len(activities))
	seen := make(map[string]struct{}, len(activities))
	for _, activity := range activities {
		processInstanceID := strings.TrimSpace(activity.CalledProcessInstanceID)
		if processInstanceID == "" {
			continue
		}
		if _, ok := seen[processInstanceID]; ok {
			continue
		}
		seen[processInstanceID] = struct{}{}
		result = append(result, processInstanceID)
	}
	return result
}

func namespaceCallActivityNodeID(callActivityID, nodeID string) string {
	callActivityID = strings.TrimSpace(callActivityID)
	nodeID = strings.TrimSpace(nodeID)
	if callActivityID == "" {
		return nodeID
	}
	if nodeID == "" {
		return callActivityID
	}
	return callActivityID + "::" + nodeID
}

func namespaceCallActivityNodeIDs(callActivityID string, nodeIDs []string) []string {
	result := make([]string, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		result = append(result, namespaceCallActivityNodeID(callActivityID, nodeID))
	}
	return result
}

func mergeModelNodes(target, source map[string]modelNode) {
	for key, value := range source {
		target[key] = value
	}
}

func mergeOutgoingFlows(target, source map[string][]sequenceFlow) {
	for key, values := range source {
		target[key] = append(target[key], values...)
	}
}

func mergeStringMaps(target, source map[string]string) {
	for key, value := range source {
		target[key] = value
	}
}

func mergeStringSliceMaps(target, source map[string][]string) {
	for key, values := range source {
		target[key] = append([]string(nil), values...)
	}
}

func buildSummary(snapshot *processSnapshot) types.ProcessProgressSummary {
	currentIDs := currentActivityIDs(snapshot)
	currentNames := make([]string, 0, len(snapshot.runtimeTasks))
	currentAssignees := make([]string, 0, len(snapshot.runtimeTasks))
	currentCandidateUsers := make([]string, 0, len(snapshot.runtimeTasks))
	currentCandidateGroups := make([]string, 0, len(snapshot.runtimeTasks))
	for _, task := range snapshot.runtimeTasks {
		currentNames = appendIfMissing(currentNames, firstNonBlank(task.Name, task.TaskDefinitionKey))
		if task.Assignee != "" {
			currentAssignees = appendIfMissing(currentAssignees, task.Assignee)
		}
		for _, user := range task.CandidateUsers {
			currentCandidateUsers = appendIfMissing(currentCandidateUsers, user)
		}
		for _, group := range task.CandidateGroups {
			currentCandidateGroups = appendIfMissing(currentCandidateGroups, group)
		}
	}

	completed := completedActivityIDs(snapshot)
	totalUserTasks := 0
	completedUserTasks := 0
	for _, node := range snapshot.model.VisibleNodes {
		if node.Type != "userTask" {
			continue
		}
		totalUserTasks++
		if contains(completed, node.ID) && !contains(currentIDs, node.ID) {
			completedUserTasks++
		}
	}

	progressPercent := 0
	if snapshot.process.EndTime != nil {
		progressPercent = 100
	} else if totalUserTasks > 0 {
		progressPercent = min(99, completedUserTasks*100/totalUserTasks)
	}

	return types.ProcessProgressSummary{
		ProcessInstanceID:      snapshot.process.ID,
		ProcessDefinitionID:    snapshot.process.ProcessDefinitionID,
		ProcessDefinitionKey:   snapshot.process.ProcessDefinitionKey,
		BizID:                  firstNonBlank(stringValue(snapshot.variables["bizId"]), snapshot.process.BusinessKey),
		BizType:                stringValue(snapshot.variables["bizType"]),
		Title:                  stringValue(snapshot.variables["title"]),
		TenantID:               firstNonBlank(snapshot.process.TenantID, stringValue(snapshot.variables["tenantId"])),
		SystemCode:             stringValue(snapshot.variables["systemCode"]),
		Status:                 processStatus(snapshot.process),
		ProgressPercent:        progressPercent,
		CompletedTaskCount:     completedUserTasks,
		TotalTaskCount:         totalUserTasks,
		CurrentTaskCount:       len(snapshot.runtimeTasks),
		StartTime:              formatTime(snapshot.process.StartTime),
		EndTime:                formatTimePtr(snapshot.process.EndTime),
		CurrentActivityIDs:     currentIDs,
		CurrentActivityNames:   currentNames,
		CurrentAssignees:       currentAssignees,
		CurrentCandidateUsers:  currentCandidateUsers,
		CurrentCandidateGroups: currentCandidateGroups,
		DiagramURL:             "/api/process/instance/" + snapshot.process.ID + "/definition-xml",
	}
}

func buildSteps(snapshot *processSnapshot) []types.ProcessProgressStep {
	historicByActivity := map[string][]historicTaskRecord{}
	for _, task := range snapshot.historicTasks {
		historicByActivity[task.TaskDefinitionKey] = append(historicByActivity[task.TaskDefinitionKey], task)
	}
	historicActivitiesByID := map[string][]historicActivityRecord{}
	for _, activity := range snapshot.historicActivities {
		if activity.ActivityID != "" && activity.EndTime != nil {
			historicActivitiesByID[activity.ActivityID] = append(historicActivitiesByID[activity.ActivityID], activity)
		}
	}
	runtimeByActivity := map[string][]runtimeTaskRecord{}
	for _, task := range snapshot.runtimeTasks {
		runtimeByActivity[task.TaskDefinitionKey] = append(runtimeByActivity[task.TaskDefinitionKey], task)
	}

	current := currentActivityIDs(snapshot)
	completed := completedActivityIDs(snapshot)
	steps := make([]types.ProcessProgressStep, 0, len(snapshot.model.VisibleNodes))
	for index, node := range snapshot.model.VisibleNodes {
		step := types.ProcessProgressStep{
			Order:        index + 1,
			ActivityID:   node.ID,
			ActivityName: node.Name,
			ActivityType: node.Type,
			FormKey:      node.FormKey,
			Status:       "PENDING",
		}
		step.OccurrenceCount = len(historicByActivity[node.ID]) + len(runtimeByActivity[node.ID])
		if contains(current, node.ID) {
			step.Status = "ACTIVE"
		} else if contains(completed, node.ID) {
			step.Status = "COMPLETED"
		}

		if node.Type == "startEvent" {
			step.StartTime = formatTime(snapshot.process.StartTime)
			step.EndTime = formatTime(snapshot.process.StartTime)
		}
		if historicActivities := historicActivitiesByID[node.ID]; len(historicActivities) > 0 {
			activity := historicActivities[len(historicActivities)-1]
			step.StartTime = firstNonBlank(step.StartTime, activity.StartTimeRaw)
			step.EndTime = firstNonBlank(step.EndTime, activity.EndTimeRaw)
		}
		if runtimeTasks := runtimeByActivity[node.ID]; len(runtimeTasks) > 0 {
			runtimeTask := runtimeTasks[len(runtimeTasks)-1]
			step.TaskID = runtimeTask.ID
			step.Assignee = runtimeTask.Assignee
			step.Owner = runtimeTask.Owner
			step.CandidateUsers = append([]string(nil), runtimeTask.CandidateUsers...)
			step.CandidateGroups = append([]string(nil), runtimeTask.CandidateGroups...)
			step.StartTime = runtimeTask.CreateTimeRaw
		}
		if historicTasks := historicByActivity[node.ID]; len(historicTasks) > 0 {
			historicTask := historicTasks[len(historicTasks)-1]
			step.TaskID = firstNonBlank(step.TaskID, historicTask.ID)
			step.Assignee = firstNonBlank(step.Assignee, historicTask.Assignee)
			step.Owner = firstNonBlank(step.Owner, historicTask.Owner)
			step.StartTime = firstNonBlank(step.StartTime, historicTask.StartTimeRaw)
			step.EndTime = historicTask.EndTimeRaw
			step.DurationInMillis = historicTask.DurationInMillis
		}
		steps = append(steps, step)
	}
	return steps
}

func buildTimeline(snapshot *processSnapshot) []types.ProcessProgressTimelineItem {
	items := buildActualTimeline(snapshot)
	items = append(items, buildFutureTimeline(snapshot)...)
	for index := range items {
		items[index].Sequence = index + 1
	}
	return items
}

func buildActualTimeline(snapshot *processSnapshot) []types.ProcessProgressTimelineItem {
	type timedItem struct {
		time time.Time
		item types.ProcessProgressTimelineItem
	}
	timed := []timedItem{{
		time: snapshot.process.StartTime,
		item: types.ProcessProgressTimelineItem{
			ItemType:     "PROCESS",
			Status:       "STARTED",
			ActivityID:   "process_start",
			ActivityName: "流程发起",
			ActivityType: "process",
			Assignee:     stringValue(snapshot.variables["starterId"]),
			Owner:        stringValue(snapshot.variables["starterName"]),
			StartTime:    formatTime(snapshot.process.StartTime),
			EndTime:      formatTime(snapshot.process.StartTime),
		},
	}}

	historicIDs := map[string]bool{}
	for _, task := range snapshot.historicTasks {
		historicIDs[task.ID] = true
		status := "ACTIVE"
		if task.EndTime != nil {
			status = "COMPLETED"
		}
		timed = append(timed, timedItem{
			time: task.StartTime,
			item: types.ProcessProgressTimelineItem{
				ItemType:          "TASK",
				Status:            status,
				ActivityID:        task.TaskDefinitionKey,
				ActivityName:      firstNonBlank(task.Name, task.TaskDefinitionKey),
				ActivityType:      "userTask",
				TaskID:            task.ID,
				TaskDefinitionKey: task.TaskDefinitionKey,
				Assignee:          task.Assignee,
				Owner:             task.Owner,
				FormKey:           task.FormKey,
				StartTime:         task.StartTimeRaw,
				EndTime:           task.EndTimeRaw,
				DurationInMillis:  task.DurationInMillis,
			},
		})
	}
	for _, task := range snapshot.runtimeTasks {
		if historicIDs[task.ID] {
			continue
		}
		timed = append(timed, timedItem{
			time: task.CreateTime,
			item: types.ProcessProgressTimelineItem{
				ItemType:          "TASK",
				Status:            "ACTIVE",
				ActivityID:        task.TaskDefinitionKey,
				ActivityName:      firstNonBlank(task.Name, task.TaskDefinitionKey),
				ActivityType:      "userTask",
				TaskID:            task.ID,
				TaskDefinitionKey: task.TaskDefinitionKey,
				Assignee:          task.Assignee,
				Owner:             task.Owner,
				CandidateUsers:    append([]string(nil), task.CandidateUsers...),
				CandidateGroups:   append([]string(nil), task.CandidateGroups...),
				FormKey:           task.FormKey,
				StartTime:         task.CreateTimeRaw,
			},
		})
	}
	if snapshot.process.EndTime != nil {
		timed = append(timed, timedItem{
			time: *snapshot.process.EndTime,
			item: types.ProcessProgressTimelineItem{
				ItemType:     "PROCESS",
				Status:       "COMPLETED",
				ActivityID:   "process_end",
				ActivityName: "流程结束",
				ActivityType: "process",
				StartTime:    formatTime(*snapshot.process.EndTime),
				EndTime:      formatTime(*snapshot.process.EndTime),
			},
		})
	}

	sort.Slice(timed, func(i, j int) bool {
		return timed[i].time.Before(timed[j].time)
	})
	items := make([]types.ProcessProgressTimelineItem, 0, len(timed))
	for _, row := range timed {
		items = append(items, row.item)
	}
	annotateTimelineOccurrences(items)
	return items
}

func buildFutureTimeline(snapshot *processSnapshot) []types.ProcessProgressTimelineItem {
	if snapshot.process.EndTime != nil {
		return nil
	}
	anchorIDs := currentActivityIDs(snapshot)
	if len(anchorIDs) == 0 && len(snapshot.historicTasks) > 0 {
		last := snapshot.historicTasks[len(snapshot.historicTasks)-1]
		anchorIDs = []string{last.TaskDefinitionKey}
	}
	if len(anchorIDs) == 0 {
		return nil
	}

	futureIDs := map[string]bool{}
	completed := completedActivityIDs(snapshot)
	current := currentActivityIDs(snapshot)
	for _, anchor := range anchorIDs {
		collectForwardVisibleNodes(anchor, snapshot.model, snapshot.variables, completed, current, futureIDs, map[string]bool{})
	}

	items := make([]types.ProcessProgressTimelineItem, 0, len(futureIDs))
	for _, node := range snapshot.model.VisibleNodes {
		if !futureIDs[node.ID] || contains(current, node.ID) {
			continue
		}
		items = append(items, types.ProcessProgressTimelineItem{
			ItemType:          "FUTURE",
			Status:            "PENDING",
			ActivityID:        node.ID,
			ActivityName:      node.Name,
			ActivityType:      node.Type,
			TaskDefinitionKey: node.ID,
			FormKey:           node.FormKey,
		})
	}
	return items
}

func annotateTimelineOccurrences(items []types.ProcessProgressTimelineItem) {
	activityCount := make(map[string]int)
	for index := range items {
		activityID := strings.TrimSpace(items[index].ActivityID)
		if activityID == "" || activityID == "process_start" || activityID == "process_end" {
			continue
		}
		activityCount[activityID]++
		items[index].Occurrence = activityCount[activityID]
	}
}

func collectForwardVisibleNodes(sourceID string, model *parsedModel, variables map[string]interface{}, completed, current []string, future map[string]bool, visiting map[string]bool) {
	if sourceID == "" || visiting[sourceID] {
		return
	}
	visiting[sourceID] = true
	outgoing := selectOutgoingFlows(sourceID, model, variables, completed, current)
	for _, flow := range outgoing {
		target := model.NodesByID[flow.TargetRef]
		if target.ID == "" {
			continue
		}
		if target.Visible && !contains(current, target.ID) {
			future[target.ID] = true
		}
		collectForwardVisibleNodes(target.ID, model, variables, completed, current, future, visiting)
	}
	delete(visiting, sourceID)
}

func selectOutgoingFlows(sourceID string, model *parsedModel, variables map[string]interface{}, completed, current []string) []sequenceFlow {
	if entries := model.SubProcessEntry[sourceID]; len(entries) > 0 {
		return syntheticFlows(sourceID, entries)
	}
	if entries := model.CallActivityEntry[sourceID]; len(entries) > 0 {
		return syntheticFlows(sourceID, entries)
	}
	flows := model.Outgoing[sourceID]
	if len(flows) == 0 {
		flows = parentSubProcessOutgoing(sourceID, model)
	}
	if len(flows) == 0 {
		flows = parentCallActivityOutgoing(sourceID, model)
	}
	if len(flows) <= 1 {
		return flows
	}
	node := model.NodesByID[sourceID]
	if node.Type == "parallelGateway" {
		return flows
	}
	if !shouldEvaluateConditionalFlows(node, flows) {
		return flows
	}
	matched := make([]sequenceFlow, 0, len(flows))
	for _, flow := range flows {
		if evaluateCondition(flow.ConditionExpression, variables) {
			matched = append(matched, flow)
		}
	}
	if len(matched) > 0 {
		if node.Type == "inclusiveGateway" {
			return matched
		}
		if len(matched) == 1 {
			return matched
		}
		best := []sequenceFlow{matched[0]}
		bestDistance := distanceToEnd(matched[0].TargetRef, model, map[string]int{}, map[string]bool{})
		for _, flow := range matched[1:] {
			distance := distanceToEnd(flow.TargetRef, model, map[string]int{}, map[string]bool{})
			if distance < bestDistance {
				bestDistance = distance
				best = []sequenceFlow{flow}
			}
		}
		return best
	}
	if node.DefaultFlowID != "" {
		for _, flow := range flows {
			if flow.ID == node.DefaultFlowID {
				return []sequenceFlow{flow}
			}
		}
	}
	if node.Type == "inclusiveGateway" {
		return nil
	}
	if active := selectFlowByHistoricTarget(flows, completed, current); len(active) > 0 {
		return active
	}
	best := []sequenceFlow{flows[0]}
	bestDistance := distanceToEnd(flows[0].TargetRef, model, map[string]int{}, map[string]bool{})
	for _, flow := range flows[1:] {
		distance := distanceToEnd(flow.TargetRef, model, map[string]int{}, map[string]bool{})
		if distance < bestDistance {
			bestDistance = distance
			best = []sequenceFlow{flow}
		}
	}
	return best
}

func syntheticFlows(sourceID string, targets []string) []sequenceFlow {
	flows := make([]sequenceFlow, 0, len(targets))
	for _, targetID := range targets {
		targetID = strings.TrimSpace(targetID)
		if targetID == "" {
			continue
		}
		flows = append(flows, sequenceFlow{
			ID:        sourceID + "->" + targetID,
			SourceRef: sourceID,
			TargetRef: targetID,
		})
	}
	return flows
}

func parentSubProcessOutgoing(sourceID string, model *parsedModel) []sequenceFlow {
	parentID := strings.TrimSpace(model.ParentSubProcess[sourceID])
	if parentID == "" || !isSubProcessExit(parentID, sourceID, model) {
		return nil
	}
	return model.Outgoing[parentID]
}

func parentCallActivityOutgoing(sourceID string, model *parsedModel) []sequenceFlow {
	parentID := strings.TrimSpace(model.ParentCallActivity[sourceID])
	if parentID == "" || !isCallActivityExit(parentID, sourceID, model) {
		return nil
	}
	return model.Outgoing[parentID]
}

func isSubProcessExit(subProcessID, nodeID string, model *parsedModel) bool {
	for _, current := range model.SubProcessExit[subProcessID] {
		if current == nodeID {
			return true
		}
	}
	return false
}

func isCallActivityExit(callActivityID, nodeID string, model *parsedModel) bool {
	for _, current := range model.CallActivityExit[callActivityID] {
		if current == nodeID {
			return true
		}
	}
	return false
}

func shouldEvaluateConditionalFlows(node modelNode, flows []sequenceFlow) bool {
	if node.Type == "inclusiveGateway" || node.Type == "exclusiveGateway" {
		return true
	}
	if strings.TrimSpace(node.DefaultFlowID) != "" {
		return true
	}
	for _, flow := range flows {
		if strings.TrimSpace(flow.ConditionExpression) != "" {
			return true
		}
	}
	return false
}

func selectFlowByHistoricTarget(flows []sequenceFlow, completed, current []string) []sequenceFlow {
	for _, flow := range flows {
		if contains(current, flow.TargetRef) || contains(completed, flow.TargetRef) {
			return []sequenceFlow{flow}
		}
	}
	return nil
}

func evaluateCondition(expression string, variables map[string]interface{}) bool {
	expression = normalizeConditionExpression(expression)
	if expression == "" {
		return false
	}
	if parts := splitLogicalExpression(expression, "||"); len(parts) > 1 {
		for _, part := range parts {
			if evaluateCondition(part, variables) {
				return true
			}
		}
		return false
	}
	if parts := splitLogicalExpression(expression, "&&"); len(parts) > 1 {
		for _, part := range parts {
			if !evaluateCondition(part, variables) {
				return false
			}
		}
		return true
	}
	if strings.HasPrefix(expression, "!") {
		return !evaluateCondition(strings.TrimSpace(expression[1:]), variables)
	}
	for _, operator := range []string{">=", "<=", "==", "!=", ">", "<"} {
		if left, right, ok := splitComparison(expression, operator); ok {
			return compareOperands(left, right, operator, variables)
		}
	}
	return truthyConditionValue(resolveConditionOperand(expression, variables))
}

func normalizeConditionExpression(expression string) string {
	expression = strings.TrimSpace(expression)
	expression = strings.TrimPrefix(expression, "${")
	expression = strings.TrimSuffix(expression, "}")
	expression = strings.TrimSpace(expression)
	for {
		if len(expression) < 2 || expression[0] != '(' || expression[len(expression)-1] != ')' {
			return expression
		}
		inner := strings.TrimSpace(expression[1 : len(expression)-1])
		if !hasBalancedParentheses(inner) {
			return expression
		}
		expression = inner
	}
}

func splitLogicalExpression(expression, operator string) []string {
	parts := make([]string, 0, 2)
	depth := 0
	start := 0
	for index := 0; index < len(expression)-1; index++ {
		switch expression[index] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if depth == 0 && strings.HasPrefix(expression[index:], operator) {
			parts = append(parts, strings.TrimSpace(expression[start:index]))
			start = index + len(operator)
			index += len(operator) - 1
		}
	}
	if len(parts) == 0 {
		return []string{strings.TrimSpace(expression)}
	}
	parts = append(parts, strings.TrimSpace(expression[start:]))
	return parts
}

func splitComparison(expression, operator string) (string, string, bool) {
	depth := 0
	for index := 0; index <= len(expression)-len(operator); index++ {
		switch expression[index] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if depth == 0 && strings.HasPrefix(expression[index:], operator) {
			left := strings.TrimSpace(expression[:index])
			right := strings.TrimSpace(expression[index+len(operator):])
			if left == "" || right == "" {
				return "", "", false
			}
			return left, right, true
		}
	}
	return "", "", false
}

func compareOperands(left, right, operator string, variables map[string]interface{}) bool {
	leftValue := resolveConditionOperand(left, variables)
	rightValue := resolveConditionOperand(right, variables)
	if leftNumber, ok := conditionNumericValue(leftValue); ok {
		if rightNumber, ok := conditionNumericValue(rightValue); ok {
			switch operator {
			case "==":
				return leftNumber == rightNumber
			case "!=":
				return leftNumber != rightNumber
			case ">":
				return leftNumber > rightNumber
			case "<":
				return leftNumber < rightNumber
			case ">=":
				return leftNumber >= rightNumber
			case "<=":
				return leftNumber <= rightNumber
			}
		}
	}
	leftText := normalizeConditionValue(leftValue)
	rightText := normalizeConditionValue(rightValue)
	switch operator {
	case "==":
		return leftText == rightText
	case "!=":
		return leftText != rightText
	case ">":
		return leftText > rightText
	case "<":
		return leftText < rightText
	case ">=":
		return leftText >= rightText
	case "<=":
		return leftText <= rightText
	default:
		return false
	}
}

func resolveConditionOperand(expression string, variables map[string]interface{}) interface{} {
	value := normalizeConditionExpression(expression)
	switch strings.ToLower(value) {
	case "null":
		return nil
	case "true":
		return true
	case "false":
		return false
	}
	if number, err := strconv.ParseFloat(strings.Trim(value, "'\""), 64); err == nil && !strings.ContainsAny(strings.TrimSpace(value), "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_") {
		return number
	}
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") || strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return strings.Trim(value, "'\"")
	}
	if current, ok := variables[value]; ok {
		return current
	}
	return strings.Trim(value, "'\"")
}

func truthyConditionValue(value interface{}) bool {
	switch current := value.(type) {
	case nil:
		return false
	case bool:
		return current
	case string:
		normalized := strings.TrimSpace(strings.ToLower(current))
		return normalized != "" && normalized != "false" && normalized != "null" && normalized != "0"
	case int:
		return current != 0
	case int64:
		return current != 0
	case float64:
		return current != 0
	default:
		normalized := normalizeConditionValue(current)
		return normalized != "" && normalized != "false" && normalized != "null" && normalized != "0"
	}
}

func conditionNumericValue(value interface{}) (float64, bool) {
	switch current := value.(type) {
	case int:
		return float64(current), true
	case int64:
		return float64(current), true
	case float64:
		return current, true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(current), 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func normalizeConditionValue(value interface{}) string {
	return strings.Trim(strings.TrimSpace(stringValue(value)), "'\"")
}

func hasBalancedParentheses(expression string) bool {
	depth := 0
	for _, char := range expression {
		switch char {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

func distanceToEnd(nodeID string, model *parsedModel, cache map[string]int, visiting map[string]bool) int {
	if nodeID == "" {
		return 1 << 20
	}
	if distance, ok := cache[nodeID]; ok {
		return distance
	}
	if visiting[nodeID] {
		return 1 << 20
	}
	visiting[nodeID] = true
	node := model.NodesByID[nodeID]
	if node.Type == "endEvent" {
		cache[nodeID] = 0
		delete(visiting, nodeID)
		return 0
	}
	best := 1 << 20
	for _, flow := range distanceOutgoingFlows(nodeID, model) {
		distance := distanceToEnd(flow.TargetRef, model, cache, visiting)
		if distance+1 < best {
			best = distance + 1
		}
	}
	cache[nodeID] = best
	delete(visiting, nodeID)
	return best
}

func distanceOutgoingFlows(sourceID string, model *parsedModel) []sequenceFlow {
	if entries := model.SubProcessEntry[sourceID]; len(entries) > 0 {
		return syntheticFlows(sourceID, entries)
	}
	if entries := model.CallActivityEntry[sourceID]; len(entries) > 0 {
		return syntheticFlows(sourceID, entries)
	}
	if flows := model.Outgoing[sourceID]; len(flows) > 0 {
		return flows
	}
	if flows := parentSubProcessOutgoing(sourceID, model); len(flows) > 0 {
		return flows
	}
	return parentCallActivityOutgoing(sourceID, model)
}

func completedActivityIDs(snapshot *processSnapshot) []string {
	result := []string{}
	if !snapshot.process.StartTime.IsZero() {
		result = append(result, snapshot.model.StartNodeID)
	}
	for _, task := range snapshot.historicTasks {
		if task.EndTime != nil && task.TaskDefinitionKey != "" {
			result = appendIfMissing(result, task.TaskDefinitionKey)
		}
	}
	for _, activity := range snapshot.historicActivities {
		if activity.EndTime == nil || activity.ActivityID == "" {
			continue
		}
		if _, ok := snapshot.model.NodesByID[activity.ActivityID]; ok {
			result = appendIfMissing(result, activity.ActivityID)
		}
	}
	if snapshot.process.EndTime != nil && snapshot.model.EndNodeID != "" && len(snapshot.model.EndNodeIDs) <= 1 {
		result = appendIfMissing(result, snapshot.model.EndNodeID)
	}
	return result
}

func currentActivityIDs(snapshot *processSnapshot) []string {
	result := make([]string, 0, len(snapshot.runtimeTasks))
	for _, task := range snapshot.runtimeTasks {
		if task.TaskDefinitionKey != "" {
			result = appendIfMissing(result, task.TaskDefinitionKey)
		}
	}
	return result
}

func processStatus(process processInstanceRecord) string {
	if process.EndTime == nil {
		return "RUNNING"
	}
	if process.DeleteReason != "" {
		return "TERMINATED"
	}
	return "COMPLETED"
}

func appendIfMissing(items []string, value string) []string {
	if value == "" || contains(items, value) {
		return items
	}
	return append(items, value)
}

func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}

func formatTimePtr(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

type processSnapshot struct {
	process            processInstanceRecord
	variables          map[string]interface{}
	runtimeTasks       []runtimeTaskRecord
	historicTasks      []historicTaskRecord
	historicActivities []historicActivityRecord
	model              *parsedModel
}

type modelNode struct {
	ID            string
	Name          string
	Type          string
	FormKey       string
	DefaultFlowID string
	CalledElement string
	Visible       bool
}

type sequenceFlow struct {
	ID                  string
	SourceRef           string
	TargetRef           string
	ConditionExpression string
}

type parsedModel struct {
	VisibleNodes       []modelNode
	NodesByID          map[string]modelNode
	Outgoing           map[string][]sequenceFlow
	ParentSubProcess   map[string]string
	ParentCallActivity map[string]string
	SubProcessEntry    map[string][]string
	SubProcessExit     map[string][]string
	CallActivityEntry  map[string][]string
	CallActivityExit   map[string][]string
	StartNodeID        string
	EndNodeID          string
	StartNodeIDs       []string
	EndNodeIDs         []string
}

type scopeParseResult struct {
	DirectNodeIDs []string
	StartNodeIDs  []string
	EndNodeIDs    []string
}

func parseBPMNModel(data []byte) (*parsedModel, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	model := &parsedModel{
		NodesByID:          map[string]modelNode{},
		Outgoing:           map[string][]sequenceFlow{},
		ParentSubProcess:   map[string]string{},
		ParentCallActivity: map[string]string{},
		SubProcessEntry:    map[string][]string{},
		SubProcessExit:     map[string][]string{},
		CallActivityEntry:  map[string][]string{},
		CallActivityExit:   map[string][]string{},
	}
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "process" {
			continue
		}
		scope, err := decodeScope(decoder, "process", model, "")
		if err != nil {
			return nil, err
		}
		if len(scope.StartNodeIDs) > 0 {
			model.StartNodeID = scope.StartNodeIDs[0]
			model.StartNodeIDs = append([]string(nil), scope.StartNodeIDs...)
		}
		if len(scope.EndNodeIDs) > 0 {
			model.EndNodeID = scope.EndNodeIDs[len(scope.EndNodeIDs)-1]
			model.EndNodeIDs = append([]string(nil), scope.EndNodeIDs...)
		}
	}
	return model, nil
}

func decodeScope(decoder *xml.Decoder, scopeName string, model *parsedModel, parentSubProcessID string) (scopeParseResult, error) {
	result := scopeParseResult{}
	localFlows := make([]sequenceFlow, 0, 16)
	for {
		token, err := decoder.Token()
		if err != nil {
			return result, err
		}
		switch current := token.(type) {
		case xml.StartElement:
			switch current.Name.Local {
			case "startEvent", "userTask", "endEvent", "exclusiveGateway", "inclusiveGateway", "parallelGateway", "subProcess", "callActivity":
				node := decodeNode(current, parentSubProcessID)
				if parentSubProcessID != "" && node.ID != "" {
					model.ParentSubProcess[node.ID] = parentSubProcessID
				}
				if node.Type == "subProcess" {
					childScope, childErr := decodeScope(decoder, "subProcess", model, node.ID)
					if childErr != nil {
						return result, childErr
					}
					model.SubProcessEntry[node.ID] = append([]string(nil), childScope.entryNodeIDs()...)
					model.SubProcessExit[node.ID] = append([]string(nil), childScope.exitNodeIDs()...)
					node.Visible = false
				} else {
					if err := decoder.Skip(); err != nil {
						return result, err
					}
				}
				model.NodesByID[node.ID] = node
				result.DirectNodeIDs = append(result.DirectNodeIDs, node.ID)
				if node.Visible {
					model.VisibleNodes = append(model.VisibleNodes, node)
				}
				if node.Type == "startEvent" {
					result.StartNodeIDs = append(result.StartNodeIDs, node.ID)
				}
				if node.Type == "endEvent" {
					result.EndNodeIDs = append(result.EndNodeIDs, node.ID)
				}
			case "sequenceFlow":
				var flow struct {
					ID                  string `xml:"id,attr"`
					SourceRef           string `xml:"sourceRef,attr"`
					TargetRef           string `xml:"targetRef,attr"`
					ConditionExpression string `xml:"conditionExpression"`
				}
				if err := decoder.DecodeElement(&flow, &current); err != nil {
					return result, err
				}
				currentFlow := sequenceFlow{
					ID:                  flow.ID,
					SourceRef:           flow.SourceRef,
					TargetRef:           flow.TargetRef,
					ConditionExpression: strings.TrimSpace(flow.ConditionExpression),
				}
				model.Outgoing[flow.SourceRef] = append(model.Outgoing[flow.SourceRef], currentFlow)
				localFlows = append(localFlows, currentFlow)
			default:
				if err := decoder.Skip(); err != nil {
					return result, err
				}
			}
		case xml.EndElement:
			if current.Name.Local == scopeName {
				return result.finalize(localFlows), nil
			}
		}
	}
}

func (s scopeParseResult) finalize(flows []sequenceFlow) scopeParseResult {
	if len(s.DirectNodeIDs) == 0 {
		return s
	}
	directSet := make(map[string]struct{}, len(s.DirectNodeIDs))
	incomingCount := make(map[string]int, len(s.DirectNodeIDs))
	outgoingCount := make(map[string]int, len(s.DirectNodeIDs))
	for _, nodeID := range s.DirectNodeIDs {
		directSet[nodeID] = struct{}{}
	}
	for _, flow := range flows {
		if _, ok := directSet[flow.SourceRef]; ok {
			outgoingCount[flow.SourceRef]++
		}
		if _, ok := directSet[flow.TargetRef]; ok {
			incomingCount[flow.TargetRef]++
		}
	}
	if len(s.StartNodeIDs) == 0 {
		for _, nodeID := range s.DirectNodeIDs {
			if incomingCount[nodeID] == 0 {
				s.StartNodeIDs = append(s.StartNodeIDs, nodeID)
			}
		}
	}
	if len(s.EndNodeIDs) == 0 {
		for _, nodeID := range s.DirectNodeIDs {
			if outgoingCount[nodeID] == 0 {
				s.EndNodeIDs = append(s.EndNodeIDs, nodeID)
			}
		}
	}
	return s
}

func (s scopeParseResult) entryNodeIDs() []string {
	return append([]string(nil), s.StartNodeIDs...)
}

func (s scopeParseResult) exitNodeIDs() []string {
	return append([]string(nil), s.EndNodeIDs...)
}

func decodeNode(start xml.StartElement, parentSubProcessID string) modelNode {
	node := modelNode{
		Type: start.Name.Local,
	}
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "id":
			node.ID = attr.Value
		case "name":
			node.Name = attr.Value
		case "formKey":
			node.FormKey = attr.Value
		case "default":
			node.DefaultFlowID = attr.Value
		case "calledElement":
			node.CalledElement = attr.Value
		}
	}
	node.Visible = shouldExposeNode(node.Type, parentSubProcessID)
	if node.Name == "" {
		node.Name = node.ID
	}
	return node
}

func shouldExposeNode(nodeType, parentSubProcessID string) bool {
	switch nodeType {
	case "userTask", "callActivity":
		return true
	case "startEvent", "endEvent":
		return strings.TrimSpace(parentSubProcessID) == ""
	default:
		return false
	}
}
