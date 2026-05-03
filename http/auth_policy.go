package http

import (
	"strings"

	"github.com/gin-gonic/gin"
)

type FailureMode string

const (
	FailureModeUnauthorized FailureMode = "unauthorized"
	FailureModeForbidden    FailureMode = "forbidden"
)

type ResourceScope struct {
	TenantMode    string
	WorkspaceMode string
	OwnerMode     string
}

type Guard interface {
	Name() string
	Check(*gin.Context, *Principal) error
}

type AuthPolicy struct {
	Name                  string
	RequireAuth           bool
	AllowAnonymous        bool
	AllowedPrincipalTypes []PrincipalType
	AllowedTokenSources   []string
	RequiredRoles         []string
	EnforceRBAC           bool
	ResourceScope         *ResourceScope
	Guards                []Guard
	FailureMode           FailureMode
	Description           string
}

type PolicyOption func(*AuthPolicy)

func WithAllowedPrincipalTypes(types ...PrincipalType) PolicyOption {
	return func(p *AuthPolicy) {
		p.AllowedPrincipalTypes = append([]PrincipalType{}, types...)
	}
}

func WithAllowedTokenSources(sources ...string) PolicyOption {
	return func(p *AuthPolicy) {
		p.AllowedTokenSources = append([]string{}, sources...)
	}
}

func BearerOnly() PolicyOption {
	return WithAllowedTokenSources(TokenSourceBearer)
}

func CookieOnly() PolicyOption {
	return WithAllowedTokenSources(TokenSourceCookie)
}

func BearerOrCookie() PolicyOption {
	return WithAllowedTokenSources(TokenSourceBearer, TokenSourceCookie)
}

func WithRequiredRoles(roles ...string) PolicyOption {
	return func(p *AuthPolicy) {
		p.RequiredRoles = append([]string{}, roles...)
		p.EnforceRBAC = true
	}
}

func WithGuard(guards ...Guard) PolicyOption {
	return func(p *AuthPolicy) {
		p.Guards = append(p.Guards, guards...)
	}
}

func WithResourceScope(scope *ResourceScope) PolicyOption {
	return func(p *AuthPolicy) {
		p.ResourceScope = scope
	}
}

func WithDescription(description string) PolicyOption {
	return func(p *AuthPolicy) {
		p.Description = strings.TrimSpace(description)
	}
}

func WithEnforceRBAC(enabled bool) PolicyOption {
	return func(p *AuthPolicy) {
		p.EnforceRBAC = enabled
	}
}

func WithFailureMode(mode FailureMode) PolicyOption {
	return func(p *AuthPolicy) {
		p.FailureMode = mode
	}
}

func newAuthPolicy(name string, requireAuth bool, allowAnonymous bool, allowedTypes []PrincipalType, opts ...PolicyOption) AuthPolicy {
	policy := AuthPolicy{
		Name:                  name,
		RequireAuth:           requireAuth,
		AllowAnonymous:        allowAnonymous,
		AllowedPrincipalTypes: append([]PrincipalType{}, allowedTypes...),
		AllowedTokenSources:   nil,
		RequiredRoles:         nil,
		EnforceRBAC:           false,
		Guards:                nil,
		FailureMode:           FailureModeUnauthorized,
	}
	for _, opt := range opts {
		opt(&policy)
	}
	return policy
}

func Public(opts ...PolicyOption) AuthPolicy {
	return newAuthPolicy("public", false, true, nil, opts...)
}

func Admin(opts ...PolicyOption) AuthPolicy {
	return newAuthPolicy("admin", true, false, []PrincipalType{PrincipalAdmin}, opts...)
}

func Customer(opts ...PolicyOption) AuthPolicy {
	return newAuthPolicy("customer", true, false, []PrincipalType{PrincipalCustomer}, opts...)
}

func AnyUser(opts ...PolicyOption) AuthPolicy {
	return newAuthPolicy("any_user", true, false, []PrincipalType{PrincipalAdmin, PrincipalCustomer}, opts...)
}

func Internal(opts ...PolicyOption) AuthPolicy {
	return newAuthPolicy("internal", true, false, []PrincipalType{PrincipalInternal, PrincipalService}, opts...)
}

func (p AuthPolicy) AllowsAnonymous() bool {
	return !p.RequireAuth || p.AllowAnonymous
}
