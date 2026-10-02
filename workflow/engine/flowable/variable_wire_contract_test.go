package flowable

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestFlowableVariablesPreserveJSONWireContract(test *testing.T) {
	values := map[string]interface{}{
		"nextAssignee":        "reviewer01",
		"nextCandidateUsers":  []string{"reviewer01", "reviewer02"},
		"nextCandidateGroups": []string{"wfgrp:tenant:reviewers"},
		"nextCcUsers":         []string{"observer01"},
		"nextCcGroups":        []string{"wfgrp:tenant:observers"},
		"needExpert":          false,
		"amount":              123.5,
		"optional":            nil,
		"details": map[string]interface{}{
			"title": "variable-contract",
			"items": []interface{}{map[string]interface{}{"id": 1, "enabled": true}},
			"type":  "serializable",
		},
	}
	wire, err := json.Marshal(toFlowableVariables(values))
	if err != nil {
		test.Fatal(err)
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal(wire, &rows); err != nil {
		test.Fatal(err)
	}
	actual := make(map[string]interface{}, len(rows))
	for _, row := range rows {
		name, valid := row["name"].(string)
		if !valid || len(row) != 2 {
			test.Fatalf("unexpected variable envelope: %#v", row)
		}
		if _, typed := row["type"]; typed {
			test.Fatalf("JSON variable must not opt into Java serialization: %s", name)
		}
		if _, duplicate := actual[name]; duplicate {
			test.Fatalf("duplicate variable: %s", name)
		}
		actual[name] = row["value"]
	}
	expectedWire, err := json.Marshal(values)
	if err != nil {
		test.Fatal(err)
	}
	var expected map[string]interface{}
	if err := json.Unmarshal(expectedWire, &expected); err != nil {
		test.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		test.Fatalf("JSON variable values changed: got %#v, want %#v", actual, expected)
	}
}
