package flowable

import (
	stdcontext "context"
	"sort"
	"strings"

	workflowcontext "github.com/goodbye-jack/go-common/workflow/context"
	"github.com/goodbye-jack/go-common/workflow/types"
)

func (c *RESTClient) GetDiagramView(ctx stdcontext.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessCompositeDiagramResponse, error) {
	raw, err := c.getRawDefinitionXML(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	response := &types.ProcessCompositeDiagramResponse{
		ProcessInstanceID: strings.TrimSpace(processInstanceID),
		Mode:              "single",
		Composite:         false,
		ParentXML:         string(raw),
	}
	parentModel, err := parseBPMNModel(raw)
	if err != nil || parentModel == nil {
		return response, nil
	}
	process, err := c.getProcessInstance(ctx, processInstanceID)
	if err != nil {
		return response, nil
	}
	variables, err := c.getProcessVariables(ctx, processInstanceID)
	if err != nil {
		variables = map[string]interface{}{}
	}
	activityMap, err := c.queryCallActivityBindings(ctx, processInstanceID)
	if err != nil {
		activityMap = map[string][]historicActivityRecord{}
	}
	callIDs := make([]string, 0)
	for nodeID, node := range parentModel.NodesByID {
		if node.Type == "callActivity" {
			callIDs = append(callIDs, nodeID)
		}
	}
	sort.Strings(callIDs)
	children := make([]types.ProcessCompositeDiagramChild, 0, len(callIDs))
	for _, callActivityID := range callIDs {
		node := parentModel.NodesByID[callActivityID]
		rawChild, ok := c.loadCallActivityRawDefinition(ctx, node, variables, process.TenantID, activityMap[callActivityID])
		if !ok || len(rawChild) == 0 {
			continue
		}
		child := types.ProcessCompositeDiagramChild{
			CallActivityID:   callActivityID,
			CallActivityName: firstNonBlank(strings.TrimSpace(node.Name), callActivityID),
			XML:              string(rawChild),
		}
		childProcessIDs := uniqueCalledProcessInstanceIDs(activityMap[callActivityID])
		if len(childProcessIDs) > 0 {
			child.ProcessInstanceID = childProcessIDs[0]
			if childProcess, childErr := c.getProcessInstance(ctx, child.ProcessInstanceID); childErr == nil {
				child.ProcessDefinitionID = childProcess.ProcessDefinitionID
				child.ProcessDefinitionKey = childProcess.ProcessDefinitionKey
			}
		}
		children = append(children, child)
	}
	response.Children = children
	response.Composite = len(children) > 0
	if response.Composite {
		response.Mode = "composite"
	}
	return response, nil
}

func (c *RESTClient) GetCompositeDiagram(ctx stdcontext.Context, processInstanceID string, user *workflowcontext.UserContext) (*types.ProcessCompositeDiagramResponse, error) {
	return c.GetDiagramView(ctx, processInstanceID, user)
}
