package flowable

import (
	"bytes"
	stdcontext "context"
	"encoding/xml"
	"sort"
	"strconv"
	"strings"
)

const (
	mergedCallPaddingX = 50
	mergedCallPaddingY = 40
	mergedCallGapX     = 80
)

type mergedShape struct {
	ElementID       string
	Bounds          syntheticBounds
	IsExpanded      bool
	IsMarkerVisible bool
}

type mergedEdge struct {
	ElementID string
	Waypoints [][2]int
}

type mergedDiagram struct {
	Shapes map[string]mergedShape
	Edges  map[string]mergedEdge
}

type mergedCallActivityAsset struct {
	CallActivityID string
	Model          *parsedModel
	Diagram        *mergedDiagram
}

type mergedSemanticNode struct {
	ID            string
	XMLID         string
	ParentID      string
	Type          string
	Name          string
	FormKey       string
	CalledElement string
	DefaultFlowID string
}

type mergedSemanticFlow struct {
	ID                  string
	XMLID               string
	OwnerID             string
	SourceID            string
	SourceXMLID         string
	TargetID            string
	TargetXMLID         string
	ConditionExpression string
}

func (c *RESTClient) buildMergedDefinitionXML(ctx stdcontext.Context, snapshot *processSnapshot, parentRaw []byte) ([]byte, error) {
	if snapshot == nil || snapshot.model == nil {
		return nil, nil
	}
	parentModel, err := parseBPMNModel(parentRaw)
	if err != nil || parentModel == nil {
		return nil, err
	}
	parentDiagram, err := parseMergedDiagram(parentRaw)
	if err != nil || parentDiagram == nil || len(parentDiagram.Shapes) == 0 {
		return nil, err
	}
	callAssets, err := c.loadMergedCallActivityAssets(ctx, snapshot, parentModel)
	if err != nil || len(callAssets) == 0 {
		return nil, err
	}

	parentShapes := cloneMergedShapes(parentDiagram.Shapes)
	parentEdges := cloneMergedEdges(parentDiagram.Edges)

	callIDs := make([]string, 0, len(callAssets))
	for callActivityID := range callAssets {
		if _, ok := parentShapes[callActivityID]; !ok {
			continue
		}
		callIDs = append(callIDs, callActivityID)
	}
	sort.Slice(callIDs, func(i, j int) bool {
		return parentShapes[callIDs[i]].Bounds.X < parentShapes[callIDs[j]].Bounds.X
	})

	childShapes := map[string]mergedShape{}
	childEdges := map[string]mergedEdge{}
	expandedCalls := map[string]struct{}{}

	for _, callActivityID := range callIDs {
		asset := callAssets[callActivityID]
		callShape, ok := parentShapes[callActivityID]
		if !ok || asset == nil || asset.Diagram == nil {
			continue
		}
		childMinX, childMinY, childMaxX, childMaxY, ok := mergedDiagramExtents(asset.Diagram.Shapes)
		if !ok {
			continue
		}
		containerWidth := (childMaxX - childMinX) + mergedCallPaddingX*2
		containerHeight := (childMaxY - childMinY) + mergedCallPaddingY*2
		containerY := callShape.Bounds.Y + callShape.Bounds.Height/2 - containerHeight/2
		if containerY < 40 {
			containerY = 40
		}
		container := mergedShape{
			ElementID:       callActivityID,
			Bounds:          syntheticBounds{X: callShape.Bounds.X, Y: containerY, Width: containerWidth, Height: containerHeight},
			IsExpanded:      true,
			IsMarkerVisible: false,
		}
		shiftThresholdX := callShape.Bounds.X + callShape.Bounds.Width
		shiftX := maxInt(0, containerWidth+mergedCallGapX-callShape.Bounds.Width)
		if shiftX > 0 {
			shiftMergedShapes(parentShapes, shiftThresholdX, shiftX, callActivityID)
			shiftMergedEdges(parentEdges, shiftThresholdX, shiftX, callActivityID)
		}
		parentShapes[callActivityID] = container
		expandedCalls[callActivityID] = struct{}{}

		offsetX := container.Bounds.X + mergedCallPaddingX - childMinX
		offsetY := container.Bounds.Y + mergedCallPaddingY - childMinY
		for elementID, shape := range asset.Diagram.Shapes {
			logicalID := namespaceCallActivityNodeID(callActivityID, elementID)
			childShapes[logicalID] = mergedShape{
				ElementID:       logicalID,
				Bounds:          syntheticBounds{X: shape.Bounds.X + offsetX, Y: shape.Bounds.Y + offsetY, Width: shape.Bounds.Width, Height: shape.Bounds.Height},
				IsExpanded:      shape.IsExpanded,
				IsMarkerVisible: shape.IsMarkerVisible,
			}
		}
		for elementID, edge := range asset.Diagram.Edges {
			logicalID := namespaceCallActivityNodeID(callActivityID, elementID)
			childEdges[logicalID] = mergedEdge{
				ElementID: logicalID,
				Waypoints: offsetWaypoints(edge.Waypoints, offsetX, offsetY),
			}
		}
	}

	regenerateCallActivityEdges(parentModel, parentShapes, parentEdges, expandedCalls)

	processID := firstNonBlank(snapshot.process.ProcessDefinitionKey, snapshot.process.ProcessDefinitionID, snapshot.process.ID)
	if strings.TrimSpace(processID) == "" {
		processID = "synthetic_process"
	}
	processID = processID + "__visual"
	diagramID := processID + "__diagram"
	planeID := processID + "__plane"

	nodes, flows, flowByID := buildMergedSemanticModel(parentModel, callAssets, expandedCalls)
	if len(nodes) == 0 {
		return nil, nil
	}
	childrenByOwner := make(map[string][]mergedSemanticNode)
	for _, node := range nodes {
		childrenByOwner[node.ParentID] = append(childrenByOwner[node.ParentID], node)
	}
	flowsByOwner := make(map[string][]mergedSemanticFlow)
	for _, flow := range flows {
		flowsByOwner[flow.OwnerID] = append(flowsByOwner[flow.OwnerID], flow)
	}

	allShapes := mergeAllMergedShapes(parentShapes, childShapes)
	allEdges := mergeAllMergedEdges(parentEdges, childEdges)

	var buffer bytes.Buffer
	buffer.WriteString(xml.Header)
	encoder := xml.NewEncoder(&buffer)
	encoder.Indent("", "  ")

	root := xml.StartElement{
		Name: xml.Name{Local: "definitions"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "xmlns"}, Value: "http://www.omg.org/spec/BPMN/20100524/MODEL"},
			{Name: xml.Name{Local: "xmlns:xsi"}, Value: "http://www.w3.org/2001/XMLSchema-instance"},
			{Name: xml.Name{Local: "xmlns:bpmndi"}, Value: "http://www.omg.org/spec/BPMN/20100524/DI"},
			{Name: xml.Name{Local: "xmlns:dc"}, Value: "http://www.omg.org/spec/DD/20100524/DC"},
			{Name: xml.Name{Local: "xmlns:di"}, Value: "http://www.omg.org/spec/DD/20100524/DI"},
			{Name: xml.Name{Local: "xmlns:flowable"}, Value: "http://flowable.org/bpmn"},
			{Name: xml.Name{Local: "targetNamespace"}, Value: syntheticTargetNamespace},
		},
	}
	if err := encoder.EncodeToken(root); err != nil {
		return nil, err
	}
	processStart := xml.StartElement{
		Name: xml.Name{Local: "process"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "id"}, Value: processID},
			{Name: xml.Name{Local: "name"}, Value: "父子流程合成视图"},
			{Name: xml.Name{Local: "isExecutable"}, Value: "false"},
		},
	}
	if err := encoder.EncodeToken(processStart); err != nil {
		return nil, err
	}
	if err := encoder.EncodeElement("用于业务系统将父流程与子流程合成为单张可视化流程图。", xml.StartElement{Name: xml.Name{Local: "documentation"}}); err != nil {
		return nil, err
	}
	if err := encodeMergedScope(encoder, "", childrenByOwner, flowsByOwner, allShapes, flowByID); err != nil {
		return nil, err
	}
	if err := encoder.EncodeToken(processStart.End()); err != nil {
		return nil, err
	}

	diagramStart := xml.StartElement{
		Name: xml.Name{Local: "bpmndi:BPMNDiagram"},
		Attr: []xml.Attr{{Name: xml.Name{Local: "id"}, Value: diagramID}},
	}
	if err := encoder.EncodeToken(diagramStart); err != nil {
		return nil, err
	}
	planeStart := xml.StartElement{
		Name: xml.Name{Local: "bpmndi:BPMNPlane"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "id"}, Value: planeID},
			{Name: xml.Name{Local: "bpmnElement"}, Value: processID},
		},
	}
	if err := encoder.EncodeToken(planeStart); err != nil {
		return nil, err
	}

	shapeIDs := make([]string, 0, len(allShapes))
	for elementID := range allShapes {
		shapeIDs = append(shapeIDs, elementID)
	}
	sort.Slice(shapeIDs, func(i, j int) bool {
		left := allShapes[shapeIDs[i]].Bounds
		right := allShapes[shapeIDs[j]].Bounds
		if left.X != right.X {
			return left.X < right.X
		}
		if left.Y != right.Y {
			return left.Y < right.Y
		}
		return shapeIDs[i] < shapeIDs[j]
	})
	for _, elementID := range shapeIDs {
		node, ok := findMergedNode(nodes, elementID)
		if !ok {
			continue
		}
		if err := encodeMergedShape(encoder, node, allShapes[elementID]); err != nil {
			return nil, err
		}
	}

	edgeIDs := make([]string, 0, len(allEdges))
	for elementID := range allEdges {
		edgeIDs = append(edgeIDs, elementID)
	}
	sort.Strings(edgeIDs)
	for _, elementID := range edgeIDs {
		flow, ok := flowByID[elementID]
		if !ok {
			continue
		}
		if err := encodeMergedEdge(encoder, flow, allEdges[elementID]); err != nil {
			return nil, err
		}
	}

	if err := encoder.EncodeToken(planeStart.End()); err != nil {
		return nil, err
	}
	if err := encoder.EncodeToken(diagramStart.End()); err != nil {
		return nil, err
	}
	if err := encoder.EncodeToken(root.End()); err != nil {
		return nil, err
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (c *RESTClient) loadMergedCallActivityAssets(ctx stdcontext.Context, snapshot *processSnapshot, parentModel *parsedModel) (map[string]*mergedCallActivityAsset, error) {
	result := map[string]*mergedCallActivityAsset{}
	if snapshot == nil || snapshot.process.ID == "" || parentModel == nil {
		return result, nil
	}
	activityMap, err := c.queryCallActivityBindings(ctx, snapshot.process.ID)
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
	for _, callActivityID := range callIDs {
		node := parentModel.NodesByID[callActivityID]
		raw, ok := c.loadCallActivityRawDefinition(ctx, node, snapshot.variables, snapshot.process.TenantID, activityMap[callActivityID])
		if !ok || len(raw) == 0 {
			continue
		}
		childModel, err := parseBPMNModel(raw)
		if err != nil || childModel == nil {
			continue
		}
		childDiagram, err := parseMergedDiagram(raw)
		if err != nil || childDiagram == nil || len(childDiagram.Shapes) == 0 {
			continue
		}
		result[callActivityID] = &mergedCallActivityAsset{
			CallActivityID: callActivityID,
			Model:          childModel,
			Diagram:        childDiagram,
		}
	}
	return result, nil
}

func (c *RESTClient) loadCallActivityRawDefinition(ctx stdcontext.Context, node modelNode, variables map[string]interface{}, tenantID string, activities []historicActivityRecord) ([]byte, bool) {
	for _, childProcessID := range uniqueCalledProcessInstanceIDs(activities) {
		raw, err := c.getRawDefinitionXML(ctx, childProcessID)
		if err == nil && len(raw) > 0 {
			return raw, true
		}
	}
	calledElement := resolveCalledElement(node.CalledElement, variables)
	if strings.TrimSpace(calledElement) == "" {
		return nil, false
	}
	raw, err := c.getDefinitionXMLByKey(ctx, calledElement, tenantID)
	if err != nil || len(raw) == 0 {
		return nil, false
	}
	return raw, true
}

func parseMergedDiagram(data []byte) (*mergedDiagram, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	result := &mergedDiagram{
		Shapes: map[string]mergedShape{},
		Edges:  map[string]mergedEdge{},
	}
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "BPMNShape":
			var shape struct {
				Bounds struct {
					X      int `xml:"x,attr"`
					Y      int `xml:"y,attr"`
					Width  int `xml:"width,attr"`
					Height int `xml:"height,attr"`
				} `xml:"Bounds"`
			}
			var elementID string
			isExpanded := false
			isMarkerVisible := false
			for _, attr := range start.Attr {
				switch attr.Name.Local {
				case "bpmnElement":
					elementID = strings.TrimSpace(attr.Value)
				case "isExpanded":
					isExpanded = strings.EqualFold(strings.TrimSpace(attr.Value), "true")
				case "isMarkerVisible":
					isMarkerVisible = strings.EqualFold(strings.TrimSpace(attr.Value), "true")
				}
			}
			if err := decoder.DecodeElement(&shape, &start); err != nil || elementID == "" {
				continue
			}
			result.Shapes[elementID] = mergedShape{
				ElementID:       elementID,
				Bounds:          syntheticBounds{X: shape.Bounds.X, Y: shape.Bounds.Y, Width: shape.Bounds.Width, Height: shape.Bounds.Height},
				IsExpanded:      isExpanded,
				IsMarkerVisible: isMarkerVisible,
			}
		case "BPMNEdge":
			var edge struct {
				Waypoints []struct {
					X int `xml:"x,attr"`
					Y int `xml:"y,attr"`
				} `xml:"waypoint"`
			}
			var elementID string
			for _, attr := range start.Attr {
				if attr.Name.Local == "bpmnElement" {
					elementID = strings.TrimSpace(attr.Value)
					break
				}
			}
			if err := decoder.DecodeElement(&edge, &start); err != nil || elementID == "" {
				continue
			}
			points := make([][2]int, 0, len(edge.Waypoints))
			for _, waypoint := range edge.Waypoints {
				points = append(points, [2]int{waypoint.X, waypoint.Y})
			}
			result.Edges[elementID] = mergedEdge{ElementID: elementID, Waypoints: points}
		}
	}
	return result, nil
}

func buildMergedSemanticModel(parentModel *parsedModel, callAssets map[string]*mergedCallActivityAsset, expandedCalls map[string]struct{}) ([]mergedSemanticNode, []mergedSemanticFlow, map[string]mergedSemanticFlow) {
	nodesByID := map[string]mergedSemanticNode{}
	flowsByID := map[string]mergedSemanticFlow{}

	for nodeID, node := range parentModel.NodesByID {
		parentID := strings.TrimSpace(parentModel.ParentSubProcess[nodeID])
		nodeType := node.Type
		calledElement := node.CalledElement
		if _, ok := expandedCalls[nodeID]; ok {
			nodeType = "subProcess"
			calledElement = ""
		}
		nodesByID[nodeID] = mergedSemanticNode{
			ID:            nodeID,
			XMLID:         syntheticXMLID("node", nodeID),
			ParentID:      parentID,
			Type:          nodeType,
			Name:          firstNonBlank(node.Name, nodeID),
			FormKey:       node.FormKey,
			CalledElement: calledElement,
			DefaultFlowID: node.DefaultFlowID,
		}
	}
	for sourceID, outgoing := range parentModel.Outgoing {
		for _, flow := range outgoing {
			ownerID := resolveFlowOwner(parentModel.ParentSubProcess[sourceID], parentModel.ParentSubProcess[flow.TargetRef])
			flowsByID[flow.ID] = mergedSemanticFlow{
				ID:                  flow.ID,
				XMLID:               syntheticXMLID("flow", flow.ID),
				OwnerID:             ownerID,
				SourceID:            sourceID,
				SourceXMLID:         syntheticXMLID("node", sourceID),
				TargetID:            flow.TargetRef,
				TargetXMLID:         syntheticXMLID("node", flow.TargetRef),
				ConditionExpression: flow.ConditionExpression,
			}
		}
	}

	for callActivityID, asset := range callAssets {
		if _, ok := expandedCalls[callActivityID]; !ok || asset == nil || asset.Model == nil {
			continue
		}
		for originalID, node := range asset.Model.NodesByID {
			logicalID := namespaceCallActivityNodeID(callActivityID, originalID)
			parentID := callActivityID
			if subProcessID := strings.TrimSpace(asset.Model.ParentSubProcess[originalID]); subProcessID != "" {
				parentID = namespaceCallActivityNodeID(callActivityID, subProcessID)
			}
			nodesByID[logicalID] = mergedSemanticNode{
				ID:            logicalID,
				XMLID:         syntheticXMLID("node", logicalID),
				ParentID:      parentID,
				Type:          node.Type,
				Name:          firstNonBlank(node.Name, originalID),
				FormKey:       node.FormKey,
				CalledElement: "",
				DefaultFlowID: namespaceCallActivityNodeID(callActivityID, node.DefaultFlowID),
			}
		}
		for sourceID, outgoing := range asset.Model.Outgoing {
			logicalSource := namespaceCallActivityNodeID(callActivityID, sourceID)
			for _, flow := range outgoing {
				logicalFlowID := namespaceCallActivityNodeID(callActivityID, flow.ID)
				logicalTarget := namespaceCallActivityNodeID(callActivityID, flow.TargetRef)
				ownerID := callActivityID
				if sourceParent := strings.TrimSpace(asset.Model.ParentSubProcess[sourceID]); sourceParent != "" && sourceParent == strings.TrimSpace(asset.Model.ParentSubProcess[flow.TargetRef]) {
					ownerID = namespaceCallActivityNodeID(callActivityID, sourceParent)
				}
				flowsByID[logicalFlowID] = mergedSemanticFlow{
					ID:                  logicalFlowID,
					XMLID:               syntheticXMLID("flow", logicalFlowID),
					OwnerID:             ownerID,
					SourceID:            logicalSource,
					SourceXMLID:         syntheticXMLID("node", logicalSource),
					TargetID:            logicalTarget,
					TargetXMLID:         syntheticXMLID("node", logicalTarget),
					ConditionExpression: flow.ConditionExpression,
				}
			}
		}
	}

	nodes := make([]mergedSemanticNode, 0, len(nodesByID))
	for _, node := range nodesByID {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].ParentID != nodes[j].ParentID {
			return nodes[i].ParentID < nodes[j].ParentID
		}
		return nodes[i].ID < nodes[j].ID
	})

	flows := make([]mergedSemanticFlow, 0, len(flowsByID))
	for _, flow := range flowsByID {
		flows = append(flows, flow)
	}
	sort.Slice(flows, func(i, j int) bool {
		if flows[i].OwnerID != flows[j].OwnerID {
			return flows[i].OwnerID < flows[j].OwnerID
		}
		return flows[i].ID < flows[j].ID
	})

	return nodes, flows, flowsByID
}

func encodeMergedScope(encoder *xml.Encoder, ownerID string, childrenByOwner map[string][]mergedSemanticNode, flowsByOwner map[string][]mergedSemanticFlow, shapes map[string]mergedShape, flowByID map[string]mergedSemanticFlow) error {
	children := append([]mergedSemanticNode(nil), childrenByOwner[ownerID]...)
	sort.Slice(children, func(i, j int) bool {
		left, leftOK := shapes[children[i].ID]
		right, rightOK := shapes[children[j].ID]
		if leftOK && rightOK {
			if left.Bounds.X != right.Bounds.X {
				return left.Bounds.X < right.Bounds.X
			}
			if left.Bounds.Y != right.Bounds.Y {
				return left.Bounds.Y < right.Bounds.Y
			}
		}
		return children[i].ID < children[j].ID
	})
	for _, node := range children {
		start := xml.StartElement{
			Name: xml.Name{Local: firstNonBlank(strings.TrimSpace(node.Type), "userTask")},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "id"}, Value: node.XMLID},
				{Name: xml.Name{Local: "name"}, Value: node.Name},
			},
		}
		if strings.TrimSpace(node.FormKey) != "" {
			start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "flowable:formKey"}, Value: node.FormKey})
		}
		if strings.TrimSpace(node.CalledElement) != "" && node.Type == "callActivity" {
			start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "calledElement"}, Value: node.CalledElement})
		}
		if strings.TrimSpace(node.DefaultFlowID) != "" {
			start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "default"}, Value: syntheticXMLID("flow", node.DefaultFlowID)})
		}
		if err := encoder.EncodeToken(start); err != nil {
			return err
		}
		if node.Type == "subProcess" {
			if err := encodeMergedScope(encoder, node.ID, childrenByOwner, flowsByOwner, shapes, flowByID); err != nil {
				return err
			}
		}
		if err := encoder.EncodeToken(start.End()); err != nil {
			return err
		}
	}
	for _, flow := range flowsByOwner[ownerID] {
		start := xml.StartElement{
			Name: xml.Name{Local: "sequenceFlow"},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "id"}, Value: flow.XMLID},
				{Name: xml.Name{Local: "sourceRef"}, Value: flow.SourceXMLID},
				{Name: xml.Name{Local: "targetRef"}, Value: flow.TargetXMLID},
			},
		}
		if err := encoder.EncodeToken(start); err != nil {
			return err
		}
		if strings.TrimSpace(flow.ConditionExpression) != "" {
			if err := encoder.EncodeElement(flow.ConditionExpression, xml.StartElement{Name: xml.Name{Local: "conditionExpression"}}); err != nil {
				return err
			}
		}
		if err := encoder.EncodeToken(start.End()); err != nil {
			return err
		}
	}
	return nil
}

func encodeMergedShape(encoder *xml.Encoder, node mergedSemanticNode, shape mergedShape) error {
	start := xml.StartElement{
		Name: xml.Name{Local: "bpmndi:BPMNShape"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "id"}, Value: syntheticXMLID("shape", node.ID)},
			{Name: xml.Name{Local: "bpmnElement"}, Value: node.XMLID},
		},
	}
	if shape.IsExpanded {
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "isExpanded"}, Value: "true"})
	}
	if shape.IsMarkerVisible {
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "isMarkerVisible"}, Value: "true"})
	}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	bounds := xml.StartElement{
		Name: xml.Name{Local: "dc:Bounds"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "x"}, Value: strconv.Itoa(shape.Bounds.X)},
			{Name: xml.Name{Local: "y"}, Value: strconv.Itoa(shape.Bounds.Y)},
			{Name: xml.Name{Local: "width"}, Value: strconv.Itoa(shape.Bounds.Width)},
			{Name: xml.Name{Local: "height"}, Value: strconv.Itoa(shape.Bounds.Height)},
		},
	}
	if err := encoder.EncodeToken(bounds); err != nil {
		return err
	}
	if err := encoder.EncodeToken(bounds.End()); err != nil {
		return err
	}
	return encoder.EncodeToken(start.End())
}

func encodeMergedEdge(encoder *xml.Encoder, flow mergedSemanticFlow, edge mergedEdge) error {
	start := xml.StartElement{
		Name: xml.Name{Local: "bpmndi:BPMNEdge"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "id"}, Value: syntheticXMLID("edge", flow.ID)},
			{Name: xml.Name{Local: "bpmnElement"}, Value: flow.XMLID},
		},
	}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	for _, point := range edge.Waypoints {
		waypoint := xml.StartElement{
			Name: xml.Name{Local: "di:waypoint"},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "x"}, Value: strconv.Itoa(point[0])},
				{Name: xml.Name{Local: "y"}, Value: strconv.Itoa(point[1])},
			},
		}
		if err := encoder.EncodeToken(waypoint); err != nil {
			return err
		}
		if err := encoder.EncodeToken(waypoint.End()); err != nil {
			return err
		}
	}
	return encoder.EncodeToken(start.End())
}

func cloneMergedShapes(source map[string]mergedShape) map[string]mergedShape {
	result := make(map[string]mergedShape, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneMergedEdges(source map[string]mergedEdge) map[string]mergedEdge {
	result := make(map[string]mergedEdge, len(source))
	for key, value := range source {
		result[key] = mergedEdge{
			ElementID: value.ElementID,
			Waypoints: append([][2]int(nil), value.Waypoints...),
		}
	}
	return result
}

func mergeAllMergedShapes(base, extra map[string]mergedShape) map[string]mergedShape {
	result := cloneMergedShapes(base)
	for key, value := range extra {
		result[key] = value
	}
	return result
}

func mergeAllMergedEdges(base, extra map[string]mergedEdge) map[string]mergedEdge {
	result := cloneMergedEdges(base)
	for key, value := range extra {
		result[key] = value
	}
	return result
}

func mergedDiagramExtents(shapes map[string]mergedShape) (int, int, int, int, bool) {
	first := true
	minX, minY, maxX, maxY := 0, 0, 0, 0
	for _, shape := range shapes {
		if first {
			minX = shape.Bounds.X
			minY = shape.Bounds.Y
			maxX = shape.Bounds.X + shape.Bounds.Width
			maxY = shape.Bounds.Y + shape.Bounds.Height
			first = false
			continue
		}
		minX = minInt(minX, shape.Bounds.X)
		minY = minInt(minY, shape.Bounds.Y)
		maxX = maxInt(maxX, shape.Bounds.X+shape.Bounds.Width)
		maxY = maxInt(maxY, shape.Bounds.Y+shape.Bounds.Height)
	}
	return minX, minY, maxX, maxY, !first
}

func offsetWaypoints(points [][2]int, offsetX, offsetY int) [][2]int {
	result := make([][2]int, 0, len(points))
	for _, point := range points {
		result = append(result, [2]int{point[0] + offsetX, point[1] + offsetY})
	}
	return result
}

func shiftMergedShapes(shapes map[string]mergedShape, thresholdX, deltaX int, excludeID string) {
	for key, shape := range shapes {
		if key == excludeID {
			continue
		}
		if shape.Bounds.X >= thresholdX {
			shape.Bounds.X += deltaX
			shapes[key] = shape
		}
	}
}

func shiftMergedEdges(edges map[string]mergedEdge, thresholdX, deltaX int, callActivityID string) {
	for key, edge := range edges {
		if strings.Contains(key, callActivityID) {
			continue
		}
		shifted := append([][2]int(nil), edge.Waypoints...)
		for index, point := range shifted {
			if point[0] > thresholdX {
				shifted[index][0] = point[0] + deltaX
			}
		}
		edge.Waypoints = shifted
		edges[key] = edge
	}
}

func regenerateCallActivityEdges(parentModel *parsedModel, shapes map[string]mergedShape, edges map[string]mergedEdge, expandedCalls map[string]struct{}) {
	for sourceID, outgoing := range parentModel.Outgoing {
		for _, flow := range outgoing {
			_, sourceExpanded := expandedCalls[sourceID]
			_, targetExpanded := expandedCalls[flow.TargetRef]
			if !sourceExpanded && !targetExpanded {
				continue
			}
			sourceShape, sourceOK := shapes[sourceID]
			targetShape, targetOK := shapes[flow.TargetRef]
			if !sourceOK || !targetOK {
				continue
			}
			edges[flow.ID] = mergedEdge{
				ElementID: flow.ID,
				Waypoints: syntheticWaypoints(sourceShape.Bounds, targetShape.Bounds),
			}
		}
	}
}

func resolveFlowOwner(sourceParent, targetParent string) string {
	sourceParent = strings.TrimSpace(sourceParent)
	targetParent = strings.TrimSpace(targetParent)
	if sourceParent == targetParent {
		return sourceParent
	}
	return ""
}

func findMergedNode(nodes []mergedSemanticNode, id string) (mergedSemanticNode, bool) {
	for _, node := range nodes {
		if node.ID == id {
			return node, true
		}
	}
	return mergedSemanticNode{}, false
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
