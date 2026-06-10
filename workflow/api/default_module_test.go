package api

import (
	"testing"

	commonhttp "github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/goodbye-jack/go-common/workflow/contract"
	"github.com/goodbye-jack/go-common/workflow/types"
	"github.com/spf13/viper"
)

func TestResolveNeedExpertForAssignment(t *testing.T) {
	tests := []struct {
		name             string
		request          *types.CompleteTaskRequest
		currentVariables map[string]interface{}
		expected         bool
	}{
		{
			name: "request variables false wins",
			request: &types.CompleteTaskRequest{
				NeedExpert: true,
				Variables: map[string]interface{}{
					"needExpert": false,
				},
			},
			currentVariables: map[string]interface{}{
				"needExpert": true,
			},
			expected: false,
		},
		{
			name: "top level true preserved",
			request: &types.CompleteTaskRequest{
				NeedExpert: true,
			},
			currentVariables: map[string]interface{}{
				"needExpert": false,
			},
			expected: true,
		},
		{
			name:    "fallback to current variables",
			request: &types.CompleteTaskRequest{},
			currentVariables: map[string]interface{}{
				"needExpert": true,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := resolveNeedExpertForAssignment(tt.request, tt.currentVariables)
			if actual != tt.expected {
				t.Fatalf("resolveNeedExpertForAssignment()=%v, want %v", actual, tt.expected)
			}
		})
	}
}

func TestDefaultModuleRouteDefinitions(t *testing.T) {
	module := &DefaultModule{
		routePrefix:  "/api",
		callbackPath: "/flowable/callback",
		requireSSO:   true,
		logRoutes:    true,
	}

	routes := module.routeDefinitions()
	if len(routes) != 30 {
		t.Fatalf("routeDefinitions() len=%d, want 30", len(routes))
	}
	if routes[0].Path != "/api/me" {
		t.Fatalf("first route path=%s, want /api/me", routes[0].Path)
	}
	last := routes[len(routes)-1]
	if last.Path != "/flowable/callback" {
		t.Fatalf("callback route path=%s, want /flowable/callback", last.Path)
	}
	if last.RequireSSO {
		t.Fatalf("callback route should not require SSO")
	}
}

func TestDefaultModuleRegisterAddsWorkflowRoutes(t *testing.T) {
	server := commonhttp.NewHTTPServer("test-service")
	module := &DefaultModule{
		routePrefix:  "/api",
		callbackPath: "/flowable/callback",
		requireSSO:   true,
		logRoutes:    false,
		routeRoles:   []string{utils.RoleAdministrator},
	}

	module.Register(server)

	routes := server.GetRoutes()
	if len(routes) != 31 {
		t.Fatalf("registered route count=%d, want 31 including ping", len(routes))
	}
	seen := map[string]bool{}
	for _, route := range routes {
		for _, method := range route.Methods {
			seen[route.Url+"|"+method] = true
		}
	}
	if !seen["/api/process/start|POST"] {
		t.Fatalf("missing workflow start route")
	}
	if !seen["/api/tasks/:id/claim|POST"] {
		t.Fatalf("missing workflow claim route")
	}
	if !seen["/api/tasks/:id/delegate|POST"] {
		t.Fatalf("missing workflow delegate route")
	}
	if !seen["/api/tasks/:id/transfer|POST"] {
		t.Fatalf("missing workflow transfer route")
	}
	if !seen["/api/process-instances/:id/action-timeline|GET"] {
		t.Fatalf("missing workflow action timeline route")
	}
	if !seen["/api/process-instances/:id/task-records|GET"] {
		t.Fatalf("missing workflow task records route")
	}
	if !seen["/api/biz/:bizId/action-timeline|GET"] {
		t.Fatalf("missing workflow biz action timeline route")
	}
	if !seen["/api/biz/:bizId/task-records|GET"] {
		t.Fatalf("missing workflow biz task records route")
	}
	if !seen["/api/me/task-records|GET"] {
		t.Fatalf("missing workflow my task records route")
	}
	if !seen["/flowable/callback|POST"] {
		t.Fatalf("missing workflow callback route")
	}
}

func TestDefaultModuleValidateContractAllowsStandardKeys(t *testing.T) {
	setAssignmentVariableKeysForTest(t, "nextAssignee", "nextCandidateUsers", "nextCandidateGroups")
	module := &DefaultModule{
		contract: contract.DefaultPolicy(),
	}
	if err := module.validateContract(); err != nil {
		t.Fatalf("validateContract() error=%v, want nil", err)
	}
}

func TestDefaultModuleValidateContractFailsInStrictMode(t *testing.T) {
	setAssignmentVariableKeysForTest(t, "bureauAdminId", "candidateUsersCustom", "candidateGroupsCustom")
	module := &DefaultModule{
		contract: &contract.Policy{
			Mode:                            contract.ModeStrict,
			EnforceStandardAssignmentKeys:   true,
			FailOnNonstandardAssignmentKeys: true,
		},
	}
	if err := module.validateContract(); err == nil {
		t.Fatalf("validateContract() error=nil, want non-nil")
	}
}

func setAssignmentVariableKeysForTest(t *testing.T, assignee, candidateUsers, candidateGroups string) {
	t.Helper()
	originalAssignee := viper.Get("workflow.assignment.variable_keys.assignee")
	originalCandidateUsers := viper.Get("workflow.assignment.variable_keys.candidate_users")
	originalCandidateGroups := viper.Get("workflow.assignment.variable_keys.candidate_groups")
	t.Cleanup(func() {
		viper.Set("workflow.assignment.variable_keys.assignee", originalAssignee)
		viper.Set("workflow.assignment.variable_keys.candidate_users", originalCandidateUsers)
		viper.Set("workflow.assignment.variable_keys.candidate_groups", originalCandidateGroups)
	})
	viper.Set("workflow.assignment.variable_keys.assignee", assignee)
	viper.Set("workflow.assignment.variable_keys.candidate_users", candidateUsers)
	viper.Set("workflow.assignment.variable_keys.candidate_groups", candidateGroups)
}
