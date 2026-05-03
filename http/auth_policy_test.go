package http

import "testing"

func TestBearerOnlyOption(t *testing.T) {
	policy := Customer(BearerOnly())
	if len(policy.AllowedTokenSources) != 1 || policy.AllowedTokenSources[0] != TokenSourceBearer {
		t.Fatalf("AllowedTokenSources = %v, want [%s]", policy.AllowedTokenSources, TokenSourceBearer)
	}
}

func TestCookieOnlyOption(t *testing.T) {
	policy := Admin(CookieOnly())
	if len(policy.AllowedTokenSources) != 1 || policy.AllowedTokenSources[0] != TokenSourceCookie {
		t.Fatalf("AllowedTokenSources = %v, want [%s]", policy.AllowedTokenSources, TokenSourceCookie)
	}
}

func TestBearerOrCookieOption(t *testing.T) {
	policy := AnyUser(BearerOrCookie())
	if len(policy.AllowedTokenSources) != 2 {
		t.Fatalf("AllowedTokenSources length = %d, want 2", len(policy.AllowedTokenSources))
	}
	if policy.AllowedTokenSources[0] != TokenSourceBearer || policy.AllowedTokenSources[1] != TokenSourceCookie {
		t.Fatalf("AllowedTokenSources = %v, want [%s %s]", policy.AllowedTokenSources, TokenSourceBearer, TokenSourceCookie)
	}
}
