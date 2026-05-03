package http

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goodbye-jack/go-common/log"
)

type AuthRouteRegistryEntry struct {
	ServiceName            string    `json:"service_name"`
	Path                   string    `json:"path"`
	Methods                []string  `json:"methods"`
	Tips                   string    `json:"tips"`
	AuthPolicyName         string    `json:"auth_policy_name"`
	RequireAuth            bool      `json:"require_auth"`
	AllowAnonymous         bool      `json:"allow_anonymous"`
	PrincipalTypes         []string  `json:"principal_types"`
	TokenSources           []string  `json:"token_sources"`
	RequiredRoles          []string  `json:"required_roles"`
	EnforceRBAC            bool      `json:"enforce_rbac"`
	ResourceScopeTenant    string    `json:"resource_scope_tenant"`
	ResourceScopeWorkspace string    `json:"resource_scope_workspace"`
	ResourceScopeOwner     string    `json:"resource_scope_owner"`
	GuardNames             []string  `json:"guard_names"`
	LegacySso              bool      `json:"legacy_sso"`
	BusinessApproval       bool      `json:"business_approval"`
	UpdatedAt              time.Time `json:"updated_at"`
}

func BuildAuthRouteRegistry(routes []*Route) []AuthRouteRegistryEntry {
	entries := make([]AuthRouteRegistryEntry, 0, len(routes))
	now := time.Now()
	for _, route := range routes {
		entry := AuthRouteRegistryEntry{
			ServiceName:      route.ServiceName,
			Path:             route.Url,
			Methods:          append([]string{}, route.Methods...),
			Tips:             route.Tips,
			LegacySso:        route.Sso,
			BusinessApproval: route.BusinessApproval,
			UpdatedAt:        now,
		}
		policy := route.EffectiveAuthPolicy()
		if policy != nil {
			entry.AuthPolicyName = policy.Name
			entry.RequireAuth = policy.RequireAuth
			entry.AllowAnonymous = policy.AllowAnonymous
			entry.TokenSources = append([]string{}, policy.AllowedTokenSources...)
			entry.RequiredRoles = append([]string{}, policy.RequiredRoles...)
			entry.EnforceRBAC = policy.EnforceRBAC
			entry.PrincipalTypes = principalTypesToStrings(policy.AllowedPrincipalTypes)
			entry.GuardNames = guardNames(policy.Guards)
			if policy.ResourceScope != nil {
				entry.ResourceScopeTenant = policy.ResourceScope.TenantMode
				entry.ResourceScopeWorkspace = policy.ResourceScope.WorkspaceMode
				entry.ResourceScopeOwner = policy.ResourceScope.OwnerMode
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

func WriteAuthRouteRegistrySnapshot(serviceName string, routes []*Route) {
	entries := BuildAuthRouteRegistry(routes)
	if len(entries) == 0 {
		return
	}

	fileName := buildAuthRouteRegistryFileName(serviceName)
	payload, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		log.Warnf("auth route registry marshal failed, service=%s, err=%v", serviceName, err)
		return
	}
	if err := os.WriteFile(fileName, payload, 0644); err != nil {
		log.Warnf("auth route registry write failed, file=%s, err=%v", fileName, err)
		return
	}

	authPolicyCount := 0
	legacyCount := 0
	for _, entry := range entries {
		if entry.AuthPolicyName == "legacy" {
			legacyCount++
			continue
		}
		authPolicyCount++
	}
	log.Infof("AuthRouteRegistry snapshot written, service=%s, file=%s, routes=%d, policy_routes=%d, legacy_routes=%d", serviceName, fileName, len(entries), authPolicyCount, legacyCount)
}

func buildAuthRouteRegistryFileName(serviceName string) string {
	safeName := strings.TrimSpace(serviceName)
	if safeName == "" {
		safeName = "app"
	}
	safeName = strings.ReplaceAll(safeName, "/", "_")
	return filepath.Join(".", safeName+".auth.routes.json")
}

func principalTypesToStrings(types []PrincipalType) []string {
	if len(types) == 0 {
		return nil
	}
	result := make([]string, 0, len(types))
	for _, item := range types {
		result = append(result, string(item))
	}
	return result
}

func guardNames(guards []Guard) []string {
	if len(guards) == 0 {
		return nil
	}
	result := make([]string, 0, len(guards))
	for _, item := range guards {
		if item == nil {
			continue
		}
		result = append(result, item.Name())
	}
	return result
}
