package flowable

import (
	"bytes"
	stdcontext "context"
	"encoding/xml"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
)

const (
	syntheticTargetNamespace = "http://www.flowable.org/processdef"
	syntheticNodeGapX        = 220
	syntheticNodeGapY        = 150
	syntheticStartX          = 80
	syntheticStartY          = 120
	syntheticTaskWidth       = 180
	syntheticTaskHeight      = 80
	syntheticEventSize       = 36
	syntheticGroupGapY       = 180
)

type syntheticNode struct {
	ID      string
	XMLID   string
	Name    string
	Type    string
	FormKey string
	Order   int
}

type syntheticEdge struct {
	ID          string
	XMLID       string
	SourceID    string
	SourceXMLID string
	TargetID    string
	TargetXMLID string
}

type syntheticBounds struct {
	X      int
	Y      int
	Width  int
	Height int
}

func shouldUseSyntheticDefinition(snapshot *processSnapshot) bool {
	return snapshot != nil && snapshot.model != nil && len(snapshot.model.ParentCallActivity) > 0
}

func buildSyntheticDefinitionXML(snapshot *processSnapshot) ([]byte, error) {
	nodes, edges := buildSyntheticGraph(snapshot)
	if len(nodes) == 0 {
		return nil, nil
	}
	bounds := layoutSyntheticGraph(nodes, edges)

	processID := firstNonBlank(snapshot.process.ProcessDefinitionKey, snapshot.process.ProcessDefinitionID, snapshot.process.ID)
	if strings.TrimSpace(processID) == "" {
		processID = "synthetic_process"
	}
	processID = processID + "__visual"
	diagramID := processID + "__diagram"
	planeID := processID + "__plane"

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
			{Name: xml.Name{Local: "name"}, Value: "合成流程图视图"},
			{Name: xml.Name{Local: "isExecutable"}, Value: "false"},
		},
	}
	if err := encoder.EncodeToken(processStart); err != nil {
		return nil, err
	}
	if err := encoder.EncodeElement("用于业务系统父子流程统一展示的合成流程图。", xml.StartElement{Name: xml.Name{Local: "documentation"}}); err != nil {
		return nil, err
	}

	for _, node := range nodes {
		if err := encodeSyntheticNode(encoder, node); err != nil {
			return nil, err
		}
	}
	for _, edge := range edges {
		if err := encodeSyntheticEdgeDefinition(encoder, edge); err != nil {
			return nil, err
		}
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

	for _, node := range nodes {
		if err := encodeSyntheticShape(encoder, node, bounds[node.ID]); err != nil {
			return nil, err
		}
	}
	for _, edge := range edges {
		if err := encodeSyntheticDiagramEdge(encoder, edge, bounds); err != nil {
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

func (c *RESTClient) buildAnchoredSyntheticDefinitionXML(ctx stdcontext.Context, snapshot *processSnapshot, parentRaw []byte) ([]byte, error) {
	nodes, edges := buildSyntheticGraph(snapshot)
	if len(nodes) == 0 {
		return nil, nil
	}
	parentBounds, _, parentMaxY := parseDiagramBounds(parentRaw)
	childBounds := c.loadCallActivityDiagramBounds(ctx, snapshot)
	bounds := layoutAnchoredSyntheticGraph(snapshot, nodes, parentBounds, childBounds, parentMaxY)
	if len(bounds) == 0 {
		return nil, nil
	}

	processID := firstNonBlank(snapshot.process.ProcessDefinitionKey, snapshot.process.ProcessDefinitionID, snapshot.process.ID)
	if strings.TrimSpace(processID) == "" {
		processID = "synthetic_process"
	}
	processID = processID + "__visual"
	diagramID := processID + "__diagram"
	planeID := processID + "__plane"

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
			{Name: xml.Name{Local: "name"}, Value: "合成流程图视图"},
			{Name: xml.Name{Local: "isExecutable"}, Value: "false"},
		},
	}
	if err := encoder.EncodeToken(processStart); err != nil {
		return nil, err
	}
	if err := encoder.EncodeElement("用于业务系统父子流程统一展示的合成流程图。", xml.StartElement{Name: xml.Name{Local: "documentation"}}); err != nil {
		return nil, err
	}
	for _, node := range nodes {
		if err := encodeSyntheticNode(encoder, node); err != nil {
			return nil, err
		}
	}
	for _, edge := range edges {
		if err := encodeSyntheticEdgeDefinition(encoder, edge); err != nil {
			return nil, err
		}
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
	for _, node := range nodes {
		if err := encodeSyntheticShape(encoder, node, bounds[node.ID]); err != nil {
			return nil, err
		}
	}
	for _, edge := range edges {
		if err := encodeSyntheticDiagramEdge(encoder, edge, bounds); err != nil {
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

func buildSyntheticGraph(snapshot *processSnapshot) ([]syntheticNode, []syntheticEdge) {
	if snapshot == nil || snapshot.model == nil {
		return nil, nil
	}
	orderByID := make(map[string]int, len(snapshot.model.VisibleNodes))
	nodesByID := make(map[string]syntheticNode, len(snapshot.model.VisibleNodes))
	for index, node := range snapshot.model.VisibleNodes {
		orderByID[node.ID] = index
		displayName := strings.TrimSpace(node.Name)
		if parentID := strings.TrimSpace(snapshot.model.ParentCallActivity[node.ID]); parentID != "" {
			parentName := strings.TrimSpace(snapshot.model.NodesByID[parentID].Name)
			displayName = firstNonBlank(parentName, parentID) + " / " + firstNonBlank(displayName, node.ID)
		} else if parentID := strings.TrimSpace(snapshot.model.ParentSubProcess[node.ID]); parentID != "" {
			parentName := strings.TrimSpace(snapshot.model.NodesByID[parentID].Name)
			displayName = firstNonBlank(parentName, parentID) + " / " + firstNonBlank(displayName, node.ID)
		}
		nodesByID[node.ID] = syntheticNode{
			ID:      node.ID,
			XMLID:   syntheticXMLID("node", node.ID),
			Name:    firstNonBlank(displayName, node.ID),
			Type:    node.Type,
			FormKey: node.FormKey,
			Order:   index,
		}
	}

	startID := strings.TrimSpace(snapshot.model.StartNodeID)
	if startID == "" {
		for _, node := range snapshot.model.VisibleNodes {
			if node.Type == "startEvent" {
				startID = node.ID
				break
			}
		}
	}
	if startID == "" && len(snapshot.model.VisibleNodes) > 0 {
		startID = snapshot.model.VisibleNodes[0].ID
	}

	completed := completedActivityIDs(snapshot)
	current := currentActivityIDs(snapshot)
	nodeSet := map[string]struct{}{}
	edgeSet := map[string]syntheticEdge{}
	visited := map[string]bool{}

	var walk func(string)
	walk = func(sourceID string) {
		sourceID = strings.TrimSpace(sourceID)
		if sourceID == "" || visited[sourceID] {
			return
		}
		visited[sourceID] = true
		if _, ok := nodesByID[sourceID]; ok {
			nodeSet[sourceID] = struct{}{}
		}
		for _, targetID := range immediateVisibleTargets(sourceID, snapshot.model, snapshot.variables, completed, current, map[string]bool{}) {
			if _, ok := nodesByID[targetID]; !ok {
				continue
			}
			nodeSet[targetID] = struct{}{}
			edgeID := syntheticEdgeID(sourceID, targetID)
			edgeSet[edgeID] = syntheticEdge{
				ID:          edgeID,
				XMLID:       syntheticXMLID("flow", edgeID),
				SourceID:    sourceID,
				SourceXMLID: nodesByID[sourceID].XMLID,
				TargetID:    targetID,
				TargetXMLID: nodesByID[targetID].XMLID,
			}
			walk(targetID)
		}
	}

	walk(startID)
	for _, activityID := range append(append([]string{}, completed...), current...) {
		if _, ok := nodesByID[activityID]; ok {
			nodeSet[activityID] = struct{}{}
			walk(activityID)
		}
	}

	nodes := make([]syntheticNode, 0, len(nodeSet))
	for nodeID := range nodeSet {
		nodes = append(nodes, nodesByID[nodeID])
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Order < nodes[j].Order
	})

	edges := make([]syntheticEdge, 0, len(edgeSet))
	for _, edge := range edgeSet {
		if _, ok := nodeSet[edge.SourceID]; !ok {
			continue
		}
		if _, ok := nodeSet[edge.TargetID]; !ok {
			continue
		}
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		leftSource := orderByID[edges[i].SourceID]
		rightSource := orderByID[edges[j].SourceID]
		if leftSource != rightSource {
			return leftSource < rightSource
		}
		return orderByID[edges[i].TargetID] < orderByID[edges[j].TargetID]
	})
	return nodes, edges
}

func (c *RESTClient) loadCallActivityDiagramBounds(ctx stdcontext.Context, snapshot *processSnapshot) map[string]map[string]syntheticBounds {
	result := map[string]map[string]syntheticBounds{}
	if snapshot == nil || snapshot.model == nil {
		return result
	}
	activities, err := c.queryCallActivityBindings(ctx, snapshot.process.ID)
	if err != nil {
		return result
	}
	for callActivityID, rows := range activities {
		childProcessIDs := uniqueCalledProcessInstanceIDs(rows)
		if len(childProcessIDs) == 0 {
			continue
		}
		raw, rawErr := c.getRawDefinitionXML(ctx, childProcessIDs[0])
		if rawErr != nil {
			continue
		}
		bounds, _, _ := parseDiagramBounds(raw)
		if len(bounds) > 0 {
			result[callActivityID] = bounds
		}
	}
	return result
}

func layoutAnchoredSyntheticGraph(snapshot *processSnapshot, nodes []syntheticNode, parentBounds map[string]syntheticBounds, childBounds map[string]map[string]syntheticBounds, parentMaxY int) map[string]syntheticBounds {
	bounds := make(map[string]syntheticBounds, len(nodes))
	fallback := layoutSyntheticGraph(nodes, nil)
	orderByCallActivity := make(map[string]int)
	for index, node := range snapshot.model.VisibleNodes {
		if node.Type == "callActivity" {
			orderByCallActivity[node.ID] = index
		}
	}
	grouped := make(map[string][]syntheticNode)
	for _, node := range nodes {
		callActivityID := strings.TrimSpace(snapshot.model.ParentCallActivity[node.ID])
		if callActivityID == "" {
			if current, ok := parentBounds[node.ID]; ok {
				bounds[node.ID] = current
			} else {
				bounds[node.ID] = fallback[node.ID]
			}
			continue
		}
		grouped[callActivityID] = append(grouped[callActivityID], node)
	}
	callActivityIDs := make([]string, 0, len(grouped))
	for callActivityID := range grouped {
		callActivityIDs = append(callActivityIDs, callActivityID)
	}
	sort.Slice(callActivityIDs, func(i, j int) bool {
		return orderByCallActivity[callActivityIDs[i]] < orderByCallActivity[callActivityIDs[j]]
	})
	nextBaseY := parentMaxY + syntheticGroupGapY
	for _, callActivityID := range callActivityIDs {
		callBound, hasCallBound := parentBounds[callActivityID]
		baseX := syntheticStartX
		if hasCallBound {
			baseX = callBound.X
		}
		groupNodes := grouped[callActivityID]
		sort.Slice(groupNodes, func(i, j int) bool { return groupNodes[i].Order < groupNodes[j].Order })

		layout := childBounds[callActivityID]
		groupMinX := 0
		groupMinY := 0
		groupMaxY := 0
		initialized := false
		for _, node := range groupNodes {
			originalID := originalSyntheticNodeID(node.ID)
			current, ok := layout[originalID]
			if !ok {
				continue
			}
			if !initialized {
				groupMinX = current.X
				groupMinY = current.Y
				groupMaxY = current.Y + current.Height
				initialized = true
			} else {
				if current.X < groupMinX {
					groupMinX = current.X
				}
				if current.Y < groupMinY {
					groupMinY = current.Y
				}
				if current.Y+current.Height > groupMaxY {
					groupMaxY = current.Y + current.Height
				}
			}
		}
		if initialized {
			for _, node := range groupNodes {
				originalID := originalSyntheticNodeID(node.ID)
				current, ok := layout[originalID]
				if !ok {
					bounds[node.ID] = fallback[node.ID]
					continue
				}
				bounds[node.ID] = syntheticBounds{
					X:      baseX + (current.X - groupMinX),
					Y:      nextBaseY + (current.Y - groupMinY),
					Width:  current.Width,
					Height: current.Height,
				}
			}
			nextBaseY += (groupMaxY - groupMinY) + syntheticGroupGapY
			continue
		}
		for row, node := range groupNodes {
			fb := fallback[node.ID]
			bounds[node.ID] = syntheticBounds{
				X:      baseX,
				Y:      nextBaseY + row*syntheticNodeGapY,
				Width:  fb.Width,
				Height: fb.Height,
			}
		}
		nextBaseY += len(groupNodes)*syntheticNodeGapY + syntheticGroupGapY
	}
	return bounds
}

func parseDiagramBounds(data []byte) (map[string]syntheticBounds, int, int) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	result := make(map[string]syntheticBounds)
	maxX := 0
	maxY := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "BPMNShape" {
			continue
		}
		var elementID string
		for _, attr := range start.Attr {
			if attr.Name.Local == "bpmnElement" {
				elementID = strings.TrimSpace(attr.Value)
				break
			}
		}
		if elementID == "" {
			continue
		}
		var shape struct {
			Bounds struct {
				X      int `xml:"x,attr"`
				Y      int `xml:"y,attr"`
				Width  int `xml:"width,attr"`
				Height int `xml:"height,attr"`
			} `xml:"Bounds"`
		}
		if err := decoder.DecodeElement(&shape, &start); err != nil {
			continue
		}
		result[elementID] = syntheticBounds{
			X:      shape.Bounds.X,
			Y:      shape.Bounds.Y,
			Width:  shape.Bounds.Width,
			Height: shape.Bounds.Height,
		}
		if shape.Bounds.X+shape.Bounds.Width > maxX {
			maxX = shape.Bounds.X + shape.Bounds.Width
		}
		if shape.Bounds.Y+shape.Bounds.Height > maxY {
			maxY = shape.Bounds.Y + shape.Bounds.Height
		}
	}
	return result, maxX, maxY
}

func originalSyntheticNodeID(nodeID string) string {
	if !strings.Contains(nodeID, "::") {
		return nodeID
	}
	parts := strings.SplitN(nodeID, "::", 2)
	return strings.TrimSpace(parts[1])
}

func immediateVisibleTargets(sourceID string, model *parsedModel, variables map[string]interface{}, completed, current []string, visiting map[string]bool) []string {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" || model == nil || visiting[sourceID] {
		return nil
	}
	visiting[sourceID] = true
	flows := selectOutgoingFlows(sourceID, model, variables, completed, current)
	result := make([]string, 0, len(flows))
	for _, flow := range flows {
		targetID := strings.TrimSpace(flow.TargetRef)
		if targetID == "" {
			continue
		}
		target := model.NodesByID[targetID]
		if target.ID == "" {
			continue
		}
		if target.Visible {
			result = appendIfMissing(result, targetID)
			continue
		}
		for _, nested := range immediateVisibleTargets(targetID, model, variables, completed, current, visiting) {
			result = appendIfMissing(result, nested)
		}
	}
	delete(visiting, sourceID)
	return result
}

func layoutSyntheticGraph(nodes []syntheticNode, edges []syntheticEdge) map[string]syntheticBounds {
	levelByID := make(map[string]int, len(nodes))
	orderByID := make(map[string]int, len(nodes))
	adjacency := make(map[string][]string, len(nodes))
	for index, node := range nodes {
		orderByID[node.ID] = index
	}
	for _, edge := range edges {
		adjacency[edge.SourceID] = append(adjacency[edge.SourceID], edge.TargetID)
	}
	for _, node := range nodes {
		sourceLevel := levelByID[node.ID]
		for _, targetID := range adjacency[node.ID] {
			if orderByID[targetID] <= orderByID[node.ID] {
				continue
			}
			targetLevel := sourceLevel + 1
			if targetLevel > levelByID[targetID] {
				levelByID[targetID] = targetLevel
			}
		}
	}
	grouped := map[int][]syntheticNode{}
	maxLevel := 0
	for _, node := range nodes {
		level := levelByID[node.ID]
		grouped[level] = append(grouped[level], node)
		if level > maxLevel {
			maxLevel = level
		}
	}
	bounds := make(map[string]syntheticBounds, len(nodes))
	for level := 0; level <= maxLevel; level++ {
		column := grouped[level]
		sort.Slice(column, func(i, j int) bool {
			return column[i].Order < column[j].Order
		})
		for row, node := range column {
			width, height := syntheticNodeSize(node.Type)
			bounds[node.ID] = syntheticBounds{
				X:      syntheticStartX + level*syntheticNodeGapX,
				Y:      syntheticStartY + row*syntheticNodeGapY,
				Width:  width,
				Height: height,
			}
		}
	}
	return bounds
}

func syntheticNodeSize(nodeType string) (int, int) {
	switch nodeType {
	case "startEvent", "endEvent":
		return syntheticEventSize, syntheticEventSize
	default:
		return syntheticTaskWidth, syntheticTaskHeight
	}
}

func syntheticEdgeID(sourceID, targetID string) string {
	replacer := strings.NewReplacer(":", "_", "/", "_", " ", "_")
	return "flow__" + replacer.Replace(sourceID) + "__" + replacer.Replace(targetID)
}

func encodeSyntheticNode(encoder *xml.Encoder, node syntheticNode) error {
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
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	return encoder.EncodeToken(start.End())
}

func encodeSyntheticEdgeDefinition(encoder *xml.Encoder, edge syntheticEdge) error {
	start := xml.StartElement{
		Name: xml.Name{Local: "sequenceFlow"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "id"}, Value: edge.XMLID},
			{Name: xml.Name{Local: "sourceRef"}, Value: edge.SourceXMLID},
			{Name: xml.Name{Local: "targetRef"}, Value: edge.TargetXMLID},
		},
	}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	return encoder.EncodeToken(start.End())
}

func encodeSyntheticShape(encoder *xml.Encoder, node syntheticNode, bounds syntheticBounds) error {
	start := xml.StartElement{
		Name: xml.Name{Local: "bpmndi:BPMNShape"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "id"}, Value: syntheticXMLID("shape", node.ID)},
			{Name: xml.Name{Local: "bpmnElement"}, Value: node.XMLID},
		},
	}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	boundsStart := xml.StartElement{
		Name: xml.Name{Local: "dc:Bounds"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "x"}, Value: strconv.Itoa(bounds.X)},
			{Name: xml.Name{Local: "y"}, Value: strconv.Itoa(bounds.Y)},
			{Name: xml.Name{Local: "width"}, Value: strconv.Itoa(bounds.Width)},
			{Name: xml.Name{Local: "height"}, Value: strconv.Itoa(bounds.Height)},
		},
	}
	if err := encoder.EncodeToken(boundsStart); err != nil {
		return err
	}
	if err := encoder.EncodeToken(boundsStart.End()); err != nil {
		return err
	}
	return encoder.EncodeToken(start.End())
}

func encodeSyntheticDiagramEdge(encoder *xml.Encoder, edge syntheticEdge, bounds map[string]syntheticBounds) error {
	sourceBounds, sourceOK := bounds[edge.SourceID]
	targetBounds, targetOK := bounds[edge.TargetID]
	if !sourceOK || !targetOK {
		return nil
	}
	start := xml.StartElement{
		Name: xml.Name{Local: "bpmndi:BPMNEdge"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "id"}, Value: syntheticXMLID("edge", edge.ID)},
			{Name: xml.Name{Local: "bpmnElement"}, Value: edge.XMLID},
		},
	}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	for _, point := range syntheticWaypoints(sourceBounds, targetBounds) {
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

func syntheticWaypoints(sourceBounds, targetBounds syntheticBounds) [][2]int {
	sourceX := sourceBounds.X + sourceBounds.Width
	sourceY := sourceBounds.Y + sourceBounds.Height/2
	targetX := targetBounds.X
	targetY := targetBounds.Y + targetBounds.Height/2
	if sourceY == targetY {
		return [][2]int{{sourceX, sourceY}, {targetX, targetY}}
	}
	midX := sourceX + (targetX-sourceX)/2
	return [][2]int{
		{sourceX, sourceY},
		{midX, sourceY},
		{midX, targetY},
		{targetX, targetY},
	}
}

func syntheticXMLID(kind, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "synthetic"
	}
	var builder strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	base := strings.Trim(builder.String(), "_.-")
	if base == "" {
		base = "synthetic"
	}
	if first := base[0]; !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		base = "n_" + base
	}
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(raw))
	return kind + "__" + base + "__" + strconv.FormatUint(uint64(hasher.Sum32()), 16)
}
