package context

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/config"
	commonhttp "github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/workflow/identity"
)

const (
	defaultUserNameHeader   = "X-Workflow-User-Name"
	defaultUserIDHeader     = "X-Workflow-User-ID"
	defaultSystemCodeHeader = "X-System-Code"
	defaultGroupsHeader     = "X-Workflow-Groups"
	defaultRolesHeader      = "X-Workflow-Roles"
)

type DefaultResolver struct {
	userIDStrategy   string
	userIDDelimiter  string
	userIDHeader     string
	userNameHeader   string
	systemCodeHeader string
	groupsHeader     string
	rolesHeader      string
}

func NewDefaultResolver() *DefaultResolver {
	return &DefaultResolver{
		userIDStrategy:   firstNonBlank(config.GetConfigString("workflow.context.user_id_strategy"), "raw"),
		userIDDelimiter:  firstNonBlank(config.GetConfigString("workflow.context.user_id_delimiter"), "#"),
		userIDHeader:     firstNonBlank(config.GetConfigString("workflow.context.user_id_header"), defaultUserIDHeader),
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

	rawUserID := strings.TrimSpace(commonhttp.GetUser(c))
	userID, err := r.resolveUserID(c, rawUserID)
	if err != nil {
		return nil, err
	}
	if userID == "" || userID == "anonymous" {
		return nil, errors.New("workflow user is missing")
	}

	user := &UserContext{
		UserID:     userID,
		UserName:   firstNonBlank(c.GetHeader(r.userNameHeader), userID),
		TenantID:   firstNonBlank(strings.TrimSpace(commonhttp.GetTenant(c)), r.resolveTenantID(rawUserID)),
		SystemCode: strings.TrimSpace(c.GetHeader(r.systemCodeHeader)),
		Groups:     splitHeaderList(c.GetHeader(r.groupsHeader)),
		Roles:      splitHeaderList(c.GetHeader(r.rolesHeader)),
	}
	normalizer := identity.NewNormalizerFromConfig()
	user.Groups = normalizer.NormalizeGroups(user.Groups)
	user.Roles = normalizer.NormalizeRoles(user.Roles)
	SetUserContext(c, user)
	return user, nil
}

func (r *DefaultResolver) resolveUserID(c *gin.Context, rawUserID string) (string, error) {
	strategy := strings.ToLower(strings.TrimSpace(r.userIDStrategy))
	switch strategy {
	case "", "raw":
		return rawUserID, nil
	case "username_suffix", "suffix":
		delimiter := firstNonBlank(r.userIDDelimiter, "#")
		if delimiter == "" || !strings.Contains(rawUserID, delimiter) {
			return rawUserID, nil
		}
		parts := strings.Split(rawUserID, delimiter)
		return strings.TrimSpace(parts[len(parts)-1]), nil
	case "header":
		headerName := firstNonBlank(r.userIDHeader, defaultUserIDHeader)
		value := strings.TrimSpace(c.GetHeader(headerName))
		if value == "" {
			return "", errors.New("workflow user id header is missing: " + headerName)
		}
		return value, nil
	default:
		return "", errors.New("unsupported workflow user id strategy: " + r.userIDStrategy)
	}
}

func (r *DefaultResolver) resolveTenantID(rawUserID string) string {
	delimiter := firstNonBlank(r.userIDDelimiter, "#")
	if delimiter == "" {
		delimiter = "#"
	}
	trimmed := strings.TrimSpace(rawUserID)
	if trimmed == "" || !strings.Contains(trimmed, delimiter) {
		return ""
	}
	parts := strings.Split(trimmed, delimiter)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[0])
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
