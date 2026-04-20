package api

import (
	"testing"

	commonhttp "github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/goodbye-jack/go-common/workflow/types"
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
	if len(routes) != 20 {
		t.Fatalf("routeDefinitions() len=%d, want 20", len(routes))
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
	if len(routes) != 21 {
		t.Fatalf("registered route count=%d, want 21 including ping", len(routes))
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
	if !seen["/flowable/callback|POST"] {
		t.Fatalf("missing workflow callback route")
	}
}
