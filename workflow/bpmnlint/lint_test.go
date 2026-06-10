package bpmnlint

import "testing"

func TestValidateXMLAllowsStandardAssignmentVars(t *testing.T) {
	xmlContent := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns:flowable="http://flowable.org/bpmn">
  <process id="demo">
    <userTask id="task_review" name="Review" flowable:assignee="${nextAssignee}" flowable:candidateGroups="role_ADMIN_ROLE,${nextCandidateGroups}" />
  </process>
</definitions>`)

	report, err := ValidateXML(xmlContent, RuleSet{
		AllowedExpressionVars:     []string{"nextAssignee", "nextCandidateGroups"},
		AllowFixedAssignee:        true,
		AllowFixedCandidateUsers:  true,
		AllowFixedCandidateGroups: true,
	})
	if err != nil {
		t.Fatalf("ValidateXML() error=%v", err)
	}
	if !report.Valid {
		t.Fatalf("ValidateXML() valid=%v, want true, issues=%v", report.Valid, report.Issues)
	}
}

func TestValidateXMLRejectsPrivateAssignmentVar(t *testing.T) {
	xmlContent := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns:flowable="http://flowable.org/bpmn">
  <process id="demo">
    <userTask id="task_review" name="Review" flowable:assignee="${bureauAdminId}" />
  </process>
</definitions>`)

	report, err := ValidateXML(xmlContent, RuleSet{
		AllowedExpressionVars:     []string{"nextAssignee"},
		AllowFixedAssignee:        true,
		AllowFixedCandidateUsers:  true,
		AllowFixedCandidateGroups: true,
	})
	if err != nil {
		t.Fatalf("ValidateXML() error=%v", err)
	}
	if report.Valid {
		t.Fatalf("ValidateXML() valid=%v, want false", report.Valid)
	}
	if len(report.Issues) != 1 {
		t.Fatalf("ValidateXML() issues=%d, want 1", len(report.Issues))
	}
	if report.Issues[0].Code != "private_assignment_var" {
		t.Fatalf("issue code=%s, want private_assignment_var", report.Issues[0].Code)
	}
}

func TestValidateXMLRejectsFixedAssignmentWhenDisabled(t *testing.T) {
	xmlContent := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns:flowable="http://flowable.org/bpmn">
  <process id="demo">
    <userTask id="task_review" name="Review" flowable:candidateUsers="zhangsan,lisi" />
  </process>
</definitions>`)

	report, err := ValidateXML(xmlContent, RuleSet{
		AllowedExpressionVars:     []string{"nextCandidateUsers"},
		AllowFixedAssignee:        true,
		AllowFixedCandidateUsers:  false,
		AllowFixedCandidateGroups: true,
	})
	if err != nil {
		t.Fatalf("ValidateXML() error=%v", err)
	}
	if report.Valid {
		t.Fatalf("ValidateXML() valid=%v, want false", report.Valid)
	}
	if len(report.Issues) != 2 {
		t.Fatalf("ValidateXML() issues=%d, want 2", len(report.Issues))
	}
	for _, issue := range report.Issues {
		if issue.Code != "fixed_assignment_not_allowed" {
			t.Fatalf("issue code=%s, want fixed_assignment_not_allowed", issue.Code)
		}
	}
}
