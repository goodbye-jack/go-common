package api

import (
	"testing"

	commonhttp "github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/orm"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/goodbye-jack/go-common/workflow/assignment"
	"github.com/goodbye-jack/go-common/workflow/contract"
	"github.com/goodbye-jack/go-common/workflow/directory"
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
		callbackPath: "/api/v1/workflow/callback",
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
	if last.Path != "/api/v1/workflow/callback" {
		t.Fatalf("callback route path=%s, want /api/v1/workflow/callback", last.Path)
	}
	if last.RequireSSO {
		t.Fatalf("callback route should not require SSO")
	}
}

func TestDefaultModuleRegisterAddsWorkflowRoutes(t *testing.T) {
	server := commonhttp.NewHTTPServer("test-service")
	module := &DefaultModule{
		routePrefix:  "/api",
		callbackPath: "/api/v1/workflow/callback",
		requireSSO:   true,
		logRoutes:    false,
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
	if !seen["/api/v1/workflow/callback|POST"] {
		t.Fatalf("missing workflow callback route")
	}

	for _, route := range routes {
		policy := route.EffectiveAuthPolicy()
		if route.Url == "/api/v1/workflow/callback" {
			if policy == nil || policy.Name != "public" || policy.RequireAuth {
				t.Fatalf("callback policy=%+v, want public without auth", policy)
			}
			continue
		}
		if route.Url == "/ping" {
			continue
		}
		if policy == nil || policy.Name != "any_user" || !policy.RequireAuth {
			t.Fatalf("workflow route %s policy=%+v, want any_user with auth", route.Url, policy)
		}
		if policy.EnforceRBAC {
			t.Fatalf("workflow route %s should not enforce legacy RBAC", route.Url)
		}
		if len(policy.Guards) != 1 || policy.Guards[0].Name() != "require_capability" {
			t.Fatalf("workflow route %s guards=%v, want require_capability", route.Url, policy.Guards)
		}
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

func TestValidateOptionalWorkflowDBInstances(t *testing.T) {
	originalFormref := viper.Get("workflow.formref.db_instance")
	originalRecords := viper.Get("workflow.records.db_instance")
	originalFormrefDB := orm.RelationalMap["workflow_formref_test"]
	originalRecordsDB := orm.RelationalMap["workflow_records_test"]
	t.Cleanup(func() {
		viper.Set("workflow.formref.db_instance", originalFormref)
		viper.Set("workflow.records.db_instance", originalRecords)
		if originalFormrefDB != nil {
			orm.RelationalMap["workflow_formref_test"] = originalFormrefDB
		} else {
			delete(orm.RelationalMap, "workflow_formref_test")
		}
		if originalRecordsDB != nil {
			orm.RelationalMap["workflow_records_test"] = originalRecordsDB
		} else {
			delete(orm.RelationalMap, "workflow_records_test")
		}
	})

	viper.Set("workflow.formref.db_instance", "")
	viper.Set("workflow.records.db_instance", "")
	if err := validateOptionalWorkflowDBInstances(); err != nil {
		t.Fatalf("validateOptionalWorkflowDBInstances() error=%v, want nil when unset", err)
	}

	viper.Set("workflow.formref.db_instance", "missing_formref")
	if err := validateOptionalWorkflowDBInstances(); err == nil {
		t.Fatalf("validateOptionalWorkflowDBInstances() error=nil, want missing formref db error")
	}

	viper.Set("workflow.formref.db_instance", "workflow_formref_test")
	orm.RelationalMap["workflow_formref_test"] = orm.NewOrm("file::memory:?cache=shared", utils.DBTypeSQLite, 1)
	if err := validateOptionalWorkflowDBInstances(); err != nil {
		t.Fatalf("validateOptionalWorkflowDBInstances() error=%v, want nil for existing formref db", err)
	}

	viper.Set("workflow.records.db_instance", "missing_records")
	if err := validateOptionalWorkflowDBInstances(); err == nil {
		t.Fatalf("validateOptionalWorkflowDBInstances() error=nil, want missing records db error")
	}

	viper.Set("workflow.records.db_instance", "workflow_records_test")
	orm.RelationalMap["workflow_records_test"] = orm.NewOrm("file::memory:?cache=shared", utils.DBTypeSQLite, 1)
	if err := validateOptionalWorkflowDBInstances(); err != nil {
		t.Fatalf("validateOptionalWorkflowDBInstances() error=%v, want nil for existing records db", err)
	}
}

func TestNewDefaultModuleFromConfigWithOptionsValidatesOptionalWorkflowDBInstances(t *testing.T) {
	originalBaseURL := viper.Get("workflow.flowable.base_url")
	originalUsername := viper.Get("workflow.flowable.username")
	originalPassword := viper.Get("workflow.flowable.password")
	originalFormref := viper.Get("workflow.formref.db_instance")
	t.Cleanup(func() {
		viper.Set("workflow.flowable.base_url", originalBaseURL)
		viper.Set("workflow.flowable.username", originalUsername)
		viper.Set("workflow.flowable.password", originalPassword)
		viper.Set("workflow.formref.db_instance", originalFormref)
	})

	viper.Set("workflow.flowable.base_url", "http://127.0.0.1:8087/flowable-rest/service")
	viper.Set("workflow.flowable.username", "rest-admin")
	viper.Set("workflow.flowable.password", "test")
	viper.Set("workflow.formref.db_instance", "missing_formref_for_module")

	_, err := NewDefaultModuleFromConfigWithOptions(RegisterOptions{
		DirectoryService:  directory.NewNoopService(),
		AssignmentService: assignment.NewNoopService(),
	})
	if err == nil {
		t.Fatalf("NewDefaultModuleFromConfigWithOptions() error=nil, want missing formref db error")
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
