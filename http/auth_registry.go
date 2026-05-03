package http

import (
	"errors"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/log"
)

var authRegistry = struct {
	sync.RWMutex
	extractors []CredentialExtractor
	resolvers  []PrincipalResolver
	validators []TokenValidator
}{
	extractors: []CredentialExtractor{},
	resolvers:  []PrincipalResolver{},
	validators: []TokenValidator{},
}

func init() {
	RegisterCredentialExtractor(bearerCredentialExtractor{})
	RegisterCredentialExtractor(cookieCredentialExtractor{})
	RegisterPrincipalResolver(legacyJWTResolver{})
}

func RegisterCredentialExtractor(extractor CredentialExtractor) {
	if extractor == nil {
		return
	}
	authRegistry.Lock()
	defer authRegistry.Unlock()
	authRegistry.extractors = append(authRegistry.extractors, extractor)
}

func RegisterPrincipalResolver(resolver PrincipalResolver) {
	if resolver == nil {
		return
	}
	authRegistry.Lock()
	defer authRegistry.Unlock()
	authRegistry.resolvers = append(authRegistry.resolvers, resolver)
}

func RegisterTokenValidator(validator TokenValidator) {
	if validator == nil {
		return
	}
	authRegistry.Lock()
	defer authRegistry.Unlock()
	authRegistry.validators = append(authRegistry.validators, validator)
}

func resolveCredentials(c *gin.Context) ([]*Credential, error) {
	authRegistry.RLock()
	extractors := append([]CredentialExtractor{}, authRegistry.extractors...)
	authRegistry.RUnlock()

	var credentials []*Credential
	var firstErr error
	for _, extractor := range extractors {
		cred, err := extractor.Extract(c)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if cred != nil && strings.TrimSpace(cred.Token) != "" {
			credentials = append(credentials, cred)
		}
	}
	if len(credentials) > 0 {
		return credentials, nil
	}
	return nil, firstErr
}

func matchesAllowedTokenSource(policy *AuthPolicy, source string) bool {
	if policy == nil || len(policy.AllowedTokenSources) == 0 {
		return true
	}
	for _, allowed := range policy.AllowedTokenSources {
		if strings.EqualFold(strings.TrimSpace(allowed), strings.TrimSpace(source)) {
			return true
		}
	}
	return false
}

func resolveRoutePolicy(c *gin.Context) *AuthPolicy {
	route := getCurrentRoute(c)
	if route == nil {
		return nil
	}
	return route.EffectiveAuthPolicy()
}

func prioritizeCredentialsForPolicy(policy *AuthPolicy, credentials []*Credential) []*Credential {
	if len(credentials) == 0 || policy == nil || len(policy.AllowedTokenSources) == 0 {
		return credentials
	}
	prioritized := make([]*Credential, 0, len(credentials))
	deferred := make([]*Credential, 0, len(credentials))
	for _, cred := range credentials {
		if cred == nil {
			continue
		}
		if matchesAllowedTokenSource(policy, cred.Source) {
			prioritized = append(prioritized, cred)
			continue
		}
		deferred = append(deferred, cred)
	}
	return append(prioritized, deferred...)
}

func resolvePrincipalFromCredential(c *gin.Context, policy *AuthPolicy, cred *Credential) (*Principal, error) {
	if cred == nil || strings.TrimSpace(cred.Token) == "" {
		return nil, nil
	}
	authRegistry.RLock()
	resolvers := append([]PrincipalResolver{}, authRegistry.resolvers...)
	validators := append([]TokenValidator{}, authRegistry.validators...)
	authRegistry.RUnlock()

	var lastErr error
	// Later-registered resolvers should take precedence so business-specific
	// token parsers can override the legacy catch-all resolver.
	for i := len(resolvers) - 1; i >= 0; i-- {
		resolver := resolvers[i]
		if !resolver.Supports(cred) {
			continue
		}
		principal, resolveErr := resolver.Resolve(c, cred)
		if resolveErr != nil {
			lastErr = resolveErr
			break
		}
		if principal == nil {
			continue
		}
		if strings.TrimSpace(principal.Subject) == "" {
			principal.Subject = principal.DisplayName
		}
		if principal.TokenSource == "" {
			principal.TokenSource = cred.Source
		}
		if policy != nil {
			if err := ValidateAuthPolicy(principal, policy); err != nil {
				lastErr = err
				break
			}
		}
		for _, validator := range validators {
			if validator == nil {
				continue
			}
			if err := validator.Validate(c, cred, principal); err != nil {
				lastErr = err
				principal = nil
				break
			}
		}
		if principal == nil {
			break
		}
		return principal, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("principal resolver not found")
}

func ResolveCredentialFromRequest(c *gin.Context) (*Credential, error) {
	credentials, err := resolveCredentials(c)
	if err != nil {
		return nil, err
	}
	if len(credentials) == 0 {
		return nil, nil
	}
	policy := resolveRoutePolicy(c)
	credentials = prioritizeCredentialsForPolicy(policy, credentials)
	for _, cred := range credentials {
		if cred == nil || strings.TrimSpace(cred.Token) == "" {
			continue
		}
		if policy != nil && !matchesAllowedTokenSource(policy, cred.Source) {
			continue
		}
		return cred, nil
	}
	return nil, nil
}

func ResolvePrincipalFromToken(token string, source string) (*Principal, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("missing token")
	}
	return resolvePrincipalFromCredential(nil, nil, &Credential{
		Token:  token,
		Source: strings.TrimSpace(source),
	})
}

func ResolvePrincipalFromRequest(c *gin.Context) (*Principal, error) {
	credentials, err := resolveCredentials(c)
	if err != nil {
		return nil, err
	}
	if len(credentials) == 0 {
		return nil, nil
	}

	policy := resolveRoutePolicy(c)
	credentials = prioritizeCredentialsForPolicy(policy, credentials)
	var lastErr error
	for _, cred := range credentials {
		principal, resolveErr := resolvePrincipalFromCredential(c, policy, cred)
		if resolveErr != nil {
			lastErr = resolveErr
			continue
		}
		if principal != nil {
			return principal, nil
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("principal resolver not found")
}

func ValidateAuthPolicy(principal *Principal, policy *AuthPolicy) error {
	if policy == nil {
		return nil
	}
	if principal == nil {
		if policy.AllowsAnonymous() {
			return nil
		}
		return errors.New("missing principal")
	}
	if len(policy.AllowedPrincipalTypes) > 0 {
		match := false
		for _, allowed := range policy.AllowedPrincipalTypes {
			if allowed == principal.Type {
				match = true
				break
			}
		}
		if !match {
			return errors.New("principal type not allowed")
		}
	}
	if len(policy.AllowedTokenSources) > 0 {
		match := false
		for _, allowed := range policy.AllowedTokenSources {
			if strings.EqualFold(allowed, principal.TokenSource) {
				match = true
				break
			}
		}
		if !match {
			return errors.New("token source not allowed")
		}
	}
	return nil
}

func logAuthResolveFailure(routePath string, err error) {
	if err == nil {
		return
	}
	log.Warnf("auth resolve failed, path=%s, err=%v", routePath, err)
}
