package http

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubResolver struct {
	name       string
	supports   bool
	principal  *Principal
	err        error
	resolveHit int
}

func (s *stubResolver) Name() string { return s.name }

func (s *stubResolver) Supports(*Credential) bool { return s.supports }

func (s *stubResolver) Resolve(_ *gin.Context, _ *Credential) (*Principal, error) {
	s.resolveHit++
	return s.principal, s.err
}

func TestResolvePrincipalFromRequestPrefersLaterRegisteredResolver(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalExtractors := authRegistry.extractors
	originalResolvers := authRegistry.resolvers
	originalValidators := authRegistry.validators
	authRegistry.extractors = nil
	authRegistry.resolvers = nil
	authRegistry.validators = nil
	defer func() {
		authRegistry.extractors = originalExtractors
		authRegistry.resolvers = originalResolvers
		authRegistry.validators = originalValidators
	}()

	RegisterCredentialExtractor(staticCredentialExtractor{
		cred: &Credential{Token: "token-1", Source: "bearer", Scheme: "Bearer"},
	})

	legacy := &stubResolver{
		name:     "legacy",
		supports: true,
		principal: &Principal{
			Type:        PrincipalService,
			Subject:     "legacy-subject",
			TokenSource: "bearer",
		},
	}
	customer := &stubResolver{
		name:     "customer",
		supports: true,
		principal: &Principal{
			Type:        PrincipalCustomer,
			Subject:     "customer-subject",
			TokenSource: "bearer",
		},
	}
	RegisterPrincipalResolver(legacy)
	RegisterPrincipalResolver(customer)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/customer/me", nil)

	principal, err := ResolvePrincipalFromRequest(ctx)
	if err != nil {
		t.Fatalf("ResolvePrincipalFromRequest() error = %v", err)
	}
	if principal == nil {
		t.Fatal("ResolvePrincipalFromRequest() returned nil principal")
	}
	if principal.Type != PrincipalCustomer {
		t.Fatalf("principal.Type = %s, want %s", principal.Type, PrincipalCustomer)
	}
	if customer.resolveHit != 1 {
		t.Fatalf("customer.resolveHit = %d, want 1", customer.resolveHit)
	}
	if legacy.resolveHit != 0 {
		t.Fatalf("legacy.resolveHit = %d, want 0", legacy.resolveHit)
	}
}

type staticCredentialExtractor struct {
	cred *Credential
	err  error
}

func (s staticCredentialExtractor) Name() string { return "static" }

func (s staticCredentialExtractor) Extract(*gin.Context) (*Credential, error) {
	return s.cred, s.err
}

type sourceAwareResolver struct {
	resolveHit map[string]int
	principals map[string]*Principal
	errors     map[string]error
}

func (s *sourceAwareResolver) Name() string { return "source-aware" }

func (s *sourceAwareResolver) Supports(*Credential) bool { return true }

func (s *sourceAwareResolver) Resolve(_ *gin.Context, cred *Credential) (*Principal, error) {
	if s.resolveHit == nil {
		s.resolveHit = map[string]int{}
	}
	s.resolveHit[cred.Source]++
	if err := s.errors[cred.Source]; err != nil {
		return nil, err
	}
	return s.principals[cred.Source], nil
}

type stubTokenValidator struct {
	validateHit int
	err         error
}

func (s *stubTokenValidator) Name() string { return "stub-token-validator" }

func (s *stubTokenValidator) Validate(_ *gin.Context, _ *Credential, _ *Principal) error {
	s.validateHit++
	return s.err
}

func TestResolvePrincipalFromRequestFallsBackToCredentialMatchingPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalExtractors := authRegistry.extractors
	originalResolvers := authRegistry.resolvers
	originalValidators := authRegistry.validators
	authRegistry.extractors = nil
	authRegistry.resolvers = nil
	authRegistry.validators = nil
	defer func() {
		authRegistry.extractors = originalExtractors
		authRegistry.resolvers = originalResolvers
		authRegistry.validators = originalValidators
	}()

	RegisterCredentialExtractor(staticCredentialExtractor{
		cred: &Credential{Token: "bearer-token", Source: "bearer", Scheme: "Bearer"},
	})
	RegisterCredentialExtractor(staticCredentialExtractor{
		cred: &Credential{Token: "cookie-token", Source: "cookie", Scheme: "Cookie"},
	})

	resolver := &sourceAwareResolver{
		principals: map[string]*Principal{
			"bearer": {
				Type:        PrincipalAdmin,
				Subject:     "general_tenant#admin",
				TokenSource: "bearer",
			},
			"cookie": {
				Type:        PrincipalCustomer,
				Subject:     "customer-subject",
				TokenSource: "cookie",
			},
		},
	}
	RegisterPrincipalResolver(resolver)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/customer/me", nil)
	ctx.Set(currentRouteContextKey, &Route{
		Url: "/customer/me",
		AuthPolicy: func() *AuthPolicy {
			policy := Customer(WithAllowedTokenSources("cookie"))
			return &policy
		}(),
	})

	principal, err := ResolvePrincipalFromRequest(ctx)
	if err != nil {
		t.Fatalf("ResolvePrincipalFromRequest() error = %v", err)
	}
	if principal == nil {
		t.Fatal("ResolvePrincipalFromRequest() returned nil principal")
	}
	if principal.Type != PrincipalCustomer {
		t.Fatalf("principal.Type = %s, want %s", principal.Type, PrincipalCustomer)
	}
	if principal.TokenSource != "cookie" {
		t.Fatalf("principal.TokenSource = %s, want %s", principal.TokenSource, "cookie")
	}
	if resolver.resolveHit["bearer"] != 0 {
		t.Fatalf("resolver.resolveHit[bearer] = %d, want 0", resolver.resolveHit["bearer"])
	}
	if resolver.resolveHit["cookie"] != 1 {
		t.Fatalf("resolver.resolveHit[cookie] = %d, want 1", resolver.resolveHit["cookie"])
	}
}

func TestResolvePrincipalFromRequestFallsBackAfterResolverError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalExtractors := authRegistry.extractors
	originalResolvers := authRegistry.resolvers
	originalValidators := authRegistry.validators
	authRegistry.extractors = nil
	authRegistry.resolvers = nil
	authRegistry.validators = nil
	defer func() {
		authRegistry.extractors = originalExtractors
		authRegistry.resolvers = originalResolvers
		authRegistry.validators = originalValidators
	}()

	RegisterCredentialExtractor(staticCredentialExtractor{
		cred: &Credential{Token: "bearer-token", Source: "bearer", Scheme: "Bearer"},
	})
	RegisterCredentialExtractor(staticCredentialExtractor{
		cred: &Credential{Token: "cookie-token", Source: "cookie", Scheme: "Cookie"},
	})

	resolver := &sourceAwareResolver{
		principals: map[string]*Principal{
			"cookie": {
				Type:        PrincipalCustomer,
				Subject:     "customer-subject",
				TokenSource: "cookie",
			},
		},
		errors: map[string]error{
			"bearer": errors.New("invalid bearer token"),
		},
	}
	RegisterPrincipalResolver(resolver)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/customer/me", nil)
	ctx.Set(currentRouteContextKey, &Route{
		Url: "/customer/me",
		AuthPolicy: func() *AuthPolicy {
			policy := Customer(WithAllowedTokenSources("bearer", "cookie"))
			return &policy
		}(),
	})

	principal, err := ResolvePrincipalFromRequest(ctx)
	if err != nil {
		t.Fatalf("ResolvePrincipalFromRequest() error = %v", err)
	}
	if principal == nil {
		t.Fatal("ResolvePrincipalFromRequest() returned nil principal")
	}
	if principal.TokenSource != "cookie" {
		t.Fatalf("principal.TokenSource = %s, want %s", principal.TokenSource, "cookie")
	}
	if resolver.resolveHit["bearer"] != 1 {
		t.Fatalf("resolver.resolveHit[bearer] = %d, want 1", resolver.resolveHit["bearer"])
	}
	if resolver.resolveHit["cookie"] != 1 {
		t.Fatalf("resolver.resolveHit[cookie] = %d, want 1", resolver.resolveHit["cookie"])
	}
}

func TestResolveCredentialFromRequestRespectsRouteAllowedTokenSources(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalExtractors := authRegistry.extractors
	originalValidators := authRegistry.validators
	authRegistry.extractors = nil
	authRegistry.validators = nil
	defer func() {
		authRegistry.extractors = originalExtractors
		authRegistry.validators = originalValidators
	}()

	RegisterCredentialExtractor(staticCredentialExtractor{
		cred: &Credential{Token: "bearer-token", Source: "bearer", Scheme: "Bearer"},
	})
	RegisterCredentialExtractor(staticCredentialExtractor{
		cred: &Credential{Token: "cookie-token", Source: "cookie", Scheme: "Cookie"},
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/customer/me", nil)
	ctx.Set(currentRouteContextKey, &Route{
		Url: "/customer/me",
		AuthPolicy: func() *AuthPolicy {
			policy := Customer(WithAllowedTokenSources("cookie"))
			return &policy
		}(),
	})

	cred, err := ResolveCredentialFromRequest(ctx)
	if err != nil {
		t.Fatalf("ResolveCredentialFromRequest() error = %v", err)
	}
	if cred == nil {
		t.Fatal("ResolveCredentialFromRequest() returned nil credential")
	}
	if cred.Source != "cookie" {
		t.Fatalf("cred.Source = %s, want %s", cred.Source, "cookie")
	}
	if cred.Token != "cookie-token" {
		t.Fatalf("cred.Token = %s, want %s", cred.Token, "cookie-token")
	}
}

func TestResolvePrincipalFromTokenUsesRegisteredResolvers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalResolvers := authRegistry.resolvers
	originalValidators := authRegistry.validators
	authRegistry.resolvers = nil
	authRegistry.validators = nil
	defer func() {
		authRegistry.resolvers = originalResolvers
		authRegistry.validators = originalValidators
	}()

	resolver := &sourceAwareResolver{
		principals: map[string]*Principal{
			"bearer": {
				Type:        PrincipalCustomer,
				Subject:     "customer-subject",
				TokenSource: "bearer",
			},
		},
	}
	RegisterPrincipalResolver(resolver)

	principal, err := ResolvePrincipalFromToken("token-1", "bearer")
	if err != nil {
		t.Fatalf("ResolvePrincipalFromToken() error = %v", err)
	}
	if principal == nil {
		t.Fatal("ResolvePrincipalFromToken() returned nil principal")
	}
	if principal.Type != PrincipalCustomer {
		t.Fatalf("principal.Type = %s, want %s", principal.Type, PrincipalCustomer)
	}
	if resolver.resolveHit["bearer"] != 1 {
		t.Fatalf("resolver.resolveHit[bearer] = %d, want 1", resolver.resolveHit["bearer"])
	}
}

func TestResolvePrincipalFromRequestRunsRegisteredTokenValidators(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalExtractors := authRegistry.extractors
	originalResolvers := authRegistry.resolvers
	originalValidators := authRegistry.validators
	authRegistry.extractors = nil
	authRegistry.resolvers = nil
	authRegistry.validators = nil
	defer func() {
		authRegistry.extractors = originalExtractors
		authRegistry.resolvers = originalResolvers
		authRegistry.validators = originalValidators
	}()

	RegisterCredentialExtractor(staticCredentialExtractor{
		cred: &Credential{Token: "token-1", Source: "bearer", Scheme: "Bearer"},
	})
	RegisterPrincipalResolver(&sourceAwareResolver{
		principals: map[string]*Principal{
			"bearer": {
				Type:        PrincipalCustomer,
				Subject:     "customer-subject",
				TokenSource: "bearer",
				TokenID:     "token-1",
			},
		},
	})
	validator := &stubTokenValidator{err: errors.New("token revoked")}
	RegisterTokenValidator(validator)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/customer/me", nil)
	ctx.Set(currentRouteContextKey, &Route{
		Url: "/customer/me",
		AuthPolicy: func() *AuthPolicy {
			policy := Customer(WithAllowedTokenSources("bearer"))
			return &policy
		}(),
	})

	principal, err := ResolvePrincipalFromRequest(ctx)
	if err == nil {
		t.Fatal("expected validator error, got nil")
	}
	if principal != nil {
		t.Fatal("expected nil principal when validator denies token")
	}
	if validator.validateHit != 1 {
		t.Fatalf("validator.validateHit = %d, want 1", validator.validateHit)
	}
}
