package bpmn

type Node struct {
	ID      string
	Name    string
	Type    string
	FormKey string
	Visible bool
}

type SequenceFlow struct {
	ID                  string
	SourceRef           string
	TargetRef           string
	ConditionExpression string
}

type Model struct {
	ProcessDefinitionID string
	Nodes               []Node
	Flows               []SequenceFlow
}
