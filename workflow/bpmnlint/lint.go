package bpmnlint

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

type RuleSet struct {
	AllowedExpressionVars     []string
	AllowFixedAssignee        bool
	AllowFixedCandidateUsers  bool
	AllowFixedCandidateGroups bool
}

type Issue struct {
	Level        string `json:"level"`
	Code         string `json:"code"`
	ActivityID   string `json:"activityId,omitempty"`
	ActivityName string `json:"activityName,omitempty"`
	Field        string `json:"field,omitempty"`
	Expression   string `json:"expression,omitempty"`
	Message      string `json:"message"`
	Suggestion   string `json:"suggestion,omitempty"`
}

type Report struct {
	Valid  bool    `json:"valid"`
	Issues []Issue `json:"issues,omitempty"`
}

func ValidateXML(xmlContent []byte, rules RuleSet) (*Report, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmlContent))
	report := &Report{Valid: true}
	allowedVars := toSet(rules.AllowedExpressionVars)

	for {
		token, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "userTask" {
			continue
		}
		activityID := attrValue(start.Attr, "id")
		activityName := attrValue(start.Attr, "name")
		validateField(report, rules.AllowFixedAssignee, allowedVars, activityID, activityName, "assignee", attrValue(start.Attr, "assignee"))
		validateField(report, rules.AllowFixedCandidateUsers, allowedVars, activityID, activityName, "candidateUsers", attrValue(start.Attr, "candidateUsers"))
		validateField(report, rules.AllowFixedCandidateGroups, allowedVars, activityID, activityName, "candidateGroups", attrValue(start.Attr, "candidateGroups"))
	}

	report.Valid = len(report.Issues) == 0
	return report, nil
}

func validateField(report *Report, allowFixed bool, allowedVars map[string]struct{}, activityID, activityName, field, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || report == nil {
		return
	}

	items := splitAssignmentValues(raw)
	if len(items) == 0 {
		return
	}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if variableName, ok := parseExactExpressionVar(item); ok {
			if _, allowed := allowedVars[variableName]; allowed {
				continue
			}
			report.Issues = append(report.Issues, Issue{
				Level:        "error",
				Code:         "private_assignment_var",
				ActivityID:   activityID,
				ActivityName: activityName,
				Field:        field,
				Expression:   item,
				Message:      fmt.Sprintf("assignment field %s uses non-standard variable %s", field, variableName),
				Suggestion:   "replace private variable with starterId/managerId/nextAssignee/nextCandidateUsers/nextCandidateGroups or use fixed candidate groups",
			})
			continue
		}
		if strings.Contains(item, "${") {
			report.Issues = append(report.Issues, Issue{
				Level:        "error",
				Code:         "unsupported_assignment_expression",
				ActivityID:   activityID,
				ActivityName: activityName,
				Field:        field,
				Expression:   item,
				Message:      fmt.Sprintf("assignment field %s contains unsupported expression %s", field, item),
				Suggestion:   "use exact expressions like ${nextAssignee} or fixed values only",
			})
			continue
		}
		if !allowFixed {
			report.Issues = append(report.Issues, Issue{
				Level:        "error",
				Code:         "fixed_assignment_not_allowed",
				ActivityID:   activityID,
				ActivityName: activityName,
				Field:        field,
				Expression:   item,
				Message:      fmt.Sprintf("assignment field %s uses fixed value %s but fixed values are disabled", field, item),
			})
		}
	}
}

func parseExactExpressionVar(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "${") || !strings.HasSuffix(trimmed, "}") {
		return "", false
	}
	inner := strings.TrimSpace(trimmed[2 : len(trimmed)-1])
	if inner == "" || strings.ContainsAny(inner, " (){}[]+-/*") {
		return "", false
	}
	return inner, true
}

func splitAssignmentValues(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func attrValue(attrs []xml.Attr, local string) string {
	for _, attr := range attrs {
		if attr.Name.Local == local {
			return attr.Value
		}
	}
	return ""
}

func toSet(items []string) map[string]struct{} {
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		result[trimmed] = struct{}{}
	}
	return result
}
