package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func TestGetTokenFromRequestPrefersBearer(t *testing.T) {
	t.Setenv("GIN_MODE", "test")
	gin.SetMode(gin.TestMode)
	viper.Set("security.cookie.name", "good_token")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("GET", "/token", nil)
	req.Header.Set("Authorization", "Bearer bearer-token")
	req.AddCookie(&http.Cookie{Name: "good_token", Value: "cookie-token"})
	ctx.Request = req

	token, err := GetTokenFromRequest(ctx)
	if err != nil {
		t.Fatalf("GetTokenFromRequest returned error: %v", err)
	}
	if token != "bearer-token" {
		t.Fatalf("unexpected token, got=%s want=%s", token, "bearer-token")
	}
}

func TestGetTokenFromRequestFallsBackToCookie(t *testing.T) {
	t.Setenv("GIN_MODE", "test")
	gin.SetMode(gin.TestMode)
	viper.Set("security.cookie.name", "good_token")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("GET", "/token", nil)
	req.AddCookie(&http.Cookie{Name: "good_token", Value: "cookie-token"})
	ctx.Request = req

	token, err := GetTokenFromRequest(ctx)
	if err != nil {
		t.Fatalf("GetTokenFromRequest returned error: %v", err)
	}
	if token != "cookie-token" {
		t.Fatalf("unexpected token, got=%s want=%s", token, "cookie-token")
	}
}

func TestResolveCookieTokenNameUsesConfiguredValue(t *testing.T) {
	viper.Set("security.cookie.name", "custom_token")
	if got := ResolveCookieTokenName(); got != "custom_token" {
		t.Fatalf("unexpected token name, got=%s want=%s", got, "custom_token")
	}
}

func TestGetTokenFromRequestRespectsRouteAllowedTokenSources(t *testing.T) {
	t.Setenv("GIN_MODE", "test")
	gin.SetMode(gin.TestMode)
	viper.Set("security.cookie.name", "good_token")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest("GET", "/token", nil)
	req.Header.Set("Authorization", "Bearer bearer-token")
	req.AddCookie(&http.Cookie{Name: "good_token", Value: "cookie-token"})
	ctx.Request = req
	ctx.Set(currentRouteContextKey, &Route{
		Url: "/token",
		AuthPolicy: func() *AuthPolicy {
			policy := Customer(WithAllowedTokenSources("cookie"))
			return &policy
		}(),
	})

	token, err := GetTokenFromRequest(ctx)
	if err != nil {
		t.Fatalf("GetTokenFromRequest returned error: %v", err)
	}
	if token != "cookie-token" {
		t.Fatalf("unexpected token, got=%s want=%s", token, "cookie-token")
	}
}
