package context

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/config"
	commonhttp "github.com/goodbye-jack/go-common/http"
)

const (
	defaultUserNameHeader   = "X-Workflow-User-Name"
	defaultSystemCodeHeader = "X-System-Code"
	defaultGroupsHeader     = "X-Workflow-Groups"
	defaultRolesHeader      = "X-Workflow-Roles"
)

type DefaultResolver struct {
	userNameHeader   string
	systemCodeHeader string
	groupsHeader     string
	rolesHeader      string
}

func NewDefaultResolver() *DefaultResolver {
	return &DefaultResolver{
		userNameHeader:   firstNonBlank(config.GetConfigString("workflow.context.user_name_header"), defaultUserNameHeader),
		systemCodeHeader: firstNonBlank(config.GetConfigString("workflow.context.system_code_header"), defaultSystemCodeHeader),
		groupsHeader:     firstNonBlank(config.GetConfigString("workflow.context.groups_header"), defaultGroupsHeader),
		rolesHeader:      firstNonBlank(config.GetConfigString("workflow.context.roles_header"), defaultRolesHeader),
	}
}

func (r *DefaultResolver) Resolve(c *gin.Context) (*UserContext, error) {
	if cached, ok := GetUserContext(c); ok && cached != nil {
		return cached, nil
	}
	if c == nil {
		return nil, errors.New("gin context is nil")
	}

	userID := strings.TrimSpace(commonhttp.GetUser(c))
	if userID == "" || userID == "anonymous" {
		return nil, errors.New("workflow user is missing")
	}

	user := &UserContext{
		UserID:     userID,
		UserName:   firstNonBlank(c.GetHeader(r.userNameHeader), userID),
		TenantID:   strings.TrimSpace(commonhttp.GetTenant(c)),
		SystemCode: strings.TrimSpace(c.GetHeader(r.systemCodeHeader)),
		Groups:     splitHeaderList(c.GetHeader(r.groupsHeader)),
		Roles:      splitHeaderList(c.GetHeader(r.rolesHeader)),
	}
	SetUserContext(c, user)
	return user, nil
}

func splitHeaderList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
