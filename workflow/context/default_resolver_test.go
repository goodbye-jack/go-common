package context

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func TestDefaultResolverResolveUserIDStrategy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetViper := backupWorkflowConfig()
	defer resetViper()

	tests := []struct {
		name       string
		configure  func()
		userID     string
		headerName string
		headerVal  string
		wantUserID string
		wantErr    bool
	}{
		{
			name: "raw strategy keeps original value",
			configure: func() {
				viper.Set("workflow.context.user_id_strategy", "raw")
			},
			userID:     "general_tenant#admin",
			wantUserID: "general_tenant#admin",
		},
		{
			name: "username suffix extracts suffix",
			configure: func() {
				viper.Set("workflow.context.user_id_strategy", "username_suffix")
				viper.Set("workflow.context.user_id_delimiter", "#")
			},
			userID:     "general_tenant#admin",
			wantUserID: "admin",
		},
		{
			name: "header strategy reads configured header",
			configure: func() {
				viper.Set("workflow.context.user_id_strategy", "header")
				viper.Set("workflow.context.user_id_header", "X-Custom-Workflow-User")
			},
			userID:     "general_tenant#admin",
			headerName: "X-Custom-Workflow-User",
			headerVal:  "custom-admin",
			wantUserID: "custom-admin",
		},
		{
			name: "header strategy requires header",
			configure: func() {
				viper.Set("workflow.context.user_id_strategy", "header")
				viper.Set("workflow.context.user_id_header", "X-Custom-Workflow-User")
			},
			userID:  "general_tenant#admin",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearWorkflowConfig()
			tt.configure()
			resolver := NewDefaultResolver()
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("GET", "/", nil)
			c.Set("UserID", tt.userID)
			if tt.headerName != "" {
				c.Request.Header.Set(tt.headerName, tt.headerVal)
			}

			user, err := resolver.Resolve(c)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Resolve() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if user.UserID != tt.wantUserID {
				t.Fatalf("Resolve() userID = %q, want %q", user.UserID, tt.wantUserID)
			}
		})
	}
}

func TestDefaultResolverResolveNormalizesRolesAndGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetViper := backupWorkflowConfig()
	defer resetViper()

	clearWorkflowConfig()
	viper.Set("workflow.context.user_id_strategy", "raw")
	viper.Set("workflow.identity.role_aliases", map[string]string{
		"CITY_ADMIN": "LEGACY_ROLE_17",
	})
	viper.Set("workflow.identity.group_aliases", map[string]string{
		"CITY_REVIEWERS": "city_reviewers",
	})

	resolver := NewDefaultResolver()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set("UserID", "alarm_admin_02")
	c.Request.Header.Set(defaultRolesHeader, "CITY_ADMIN")
	c.Request.Header.Set(defaultGroupsHeader, "CITY_REVIEWERS")

	user, err := resolver.Resolve(c)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(user.Roles) != 1 || user.Roles[0] != "LEGACY_ROLE_17" {
		t.Fatalf("expected normalized role LEGACY_ROLE_17, got %#v", user.Roles)
	}
	if len(user.Groups) != 1 || user.Groups[0] != "city_reviewers" {
		t.Fatalf("expected normalized group city_reviewers, got %#v", user.Groups)
	}
}

func TestDefaultResolverResolveDerivesTenantFromUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetViper := backupWorkflowConfig()
	defer resetViper()

	clearWorkflowConfig()
	viper.Set("workflow.context.user_id_strategy", "username_suffix")
	viper.Set("workflow.context.user_id_delimiter", "#")

	resolver := NewDefaultResolver()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set("UserID", "general_tenant#alarm_admin_01")

	user, err := resolver.Resolve(c)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if user.UserID != "alarm_admin_01" {
		t.Fatalf("Resolve() userID = %q, want %q", user.UserID, "alarm_admin_01")
	}
	if user.TenantID != "general_tenant" {
		t.Fatalf("Resolve() tenantID = %q, want %q", user.TenantID, "general_tenant")
	}
}

func backupWorkflowConfig() func() {
	backup := map[string]interface{}{}
	for _, key := range workflowConfigKeys() {
		if viper.IsSet(key) {
			backup[key] = viper.Get(key)
		}
	}
	return func() {
		clearWorkflowConfig()
		for key, value := range backup {
			viper.Set(key, value)
		}
	}
}

func clearWorkflowConfig() {
	for _, key := range workflowConfigKeys() {
		viper.Set(key, "")
		_ = os.Unsetenv(key)
	}
}

func workflowConfigKeys() []string {
	return []string{
		"workflow.context.user_id_strategy",
		"workflow.context.user_id_delimiter",
		"workflow.context.user_id_header",
		"workflow.context.user_name_header",
		"workflow.context.system_code_header",
		"workflow.context.groups_header",
		"workflow.context.roles_header",
		"workflow.identity.role_aliases",
		"workflow.identity.group_aliases",
		"workflow.flowable.role_prefix",
		"workflow.flowable.group_prefix",
	}
}
