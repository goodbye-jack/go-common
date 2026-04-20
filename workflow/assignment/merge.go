package assignment

import (
	"strings"

	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/workflow/types"
)

const (
	configAssigneeVariableKey         = "workflow.assignment.variable_keys.assignee"
	configCandidateUsersVariableKey   = "workflow.assignment.variable_keys.candidate_users"
	configCandidateGroupsVariableKey  = "workflow.assignment.variable_keys.candidate_groups"
	defaultAssigneeVariableKey        = "nextAssignee"
	defaultCandidateUsersVariableKey  = "nextCandidateUsers"
	defaultCandidateGroupsVariableKey = "nextCandidateGroups"
)

func MergeResolvedVariables(base map[string]interface{}, resolved *types.AssignmentResolveResponse) map[string]interface{} {
	if base == nil {
		base = map[string]interface{}{}
	}
	if resolved == nil {
		return base
	}
	for key, value := range resolved.Variables {
		base[key] = value
	}
	if strings.TrimSpace(resolved.Assignee) != "" {
		base[firstNonBlank(config.GetConfigString(configAssigneeVariableKey), defaultAssigneeVariableKey)] = strings.TrimSpace(resolved.Assignee)
	}
	if len(resolved.CandidateUsers) > 0 {
		base[firstNonBlank(config.GetConfigString(configCandidateUsersVariableKey), defaultCandidateUsersVariableKey)] = resolved.CandidateUsers
	}
	if len(resolved.CandidateGroups) > 0 {
		base[firstNonBlank(config.GetConfigString(configCandidateGroupsVariableKey), defaultCandidateGroupsVariableKey)] = resolved.CandidateGroups
	}
	return base
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
