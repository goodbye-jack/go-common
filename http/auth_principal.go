package http

import "github.com/gin-gonic/gin"

type PrincipalType string

const (
	PrincipalAnonymous PrincipalType = "anonymous"
	PrincipalAdmin     PrincipalType = "admin"
	PrincipalCustomer  PrincipalType = "customer"
	PrincipalInternal  PrincipalType = "internal"
	PrincipalService   PrincipalType = "service"
)

type Principal struct {
	Type        PrincipalType  `json:"type"`
	Subject     string         `json:"subject"`
	UserID      uint           `json:"user_id"`
	TenantCode  string         `json:"tenant_code"`
	WorkspaceID uint           `json:"workspace_id"`
	RoleCodes   []string       `json:"role_codes"`
	TokenID     string         `json:"token_id"`
	TokenSource string         `json:"token_source"`
	Issuer      string         `json:"issuer"`
	ServiceName string         `json:"service_name"`
	DisplayName string         `json:"display_name"`
	Attributes  map[string]any `json:"attributes"`
	RawClaims   map[string]any `json:"raw_claims"`
}

func NewAnonymousPrincipal() *Principal {
	return &Principal{
		Type:        PrincipalAnonymous,
		Subject:     "anonymous",
		TokenSource: "anonymous",
		Attributes:  map[string]any{},
		RawClaims:   map[string]any{},
		DisplayName: "anonymous",
		ServiceName: "",
		Issuer:      "",
		RoleCodes:   nil,
		TenantCode:  "",
		WorkspaceID: 0,
		UserID:      0,
		TokenID:     "",
	}
}

type principalContextKey struct{}

func SetPrincipal(c *gin.Context, principal *Principal) {
	if c == nil || principal == nil {
		return
	}
	c.Set("Principal", principal)
	c.Set("UserID", principal.Subject)
}

func GetPrincipal(c *gin.Context) (*Principal, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get("Principal")
	if !ok || value == nil {
		return nil, false
	}
	principal, ok := value.(*Principal)
	return principal, ok && principal != nil
}
