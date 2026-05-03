package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	goodutils "github.com/goodbye-jack/go-common/utils"
)

func TestResolveRequestValueReadsJSONBodyWithoutConsumingIt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"pano_id":"p-1001","user_id":"22"}`)
	req := httptest.NewRequest("POST", "/api/v1/media/findByPanoId", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req

	value := ResolveRequestValue(c, "pano_id")
	if value != "p-1001" {
		t.Fatalf("ResolveRequestValue() = %s", value)
	}

	var payload map[string]any
	if err := json.NewDecoder(c.Request.Body).Decode(&payload); err != nil {
		t.Fatalf("json decode after ResolveRequestValue() error = %v", err)
	}
	if payload["pano_id"] != "p-1001" {
		t.Fatalf("payload[pano_id] = %v", payload["pano_id"])
	}
}

func TestForbidRequestFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest("POST", "/api/v1/media/updateMusic", bytes.NewReader([]byte(`{"music_id":"m-1","user_id":"9"}`)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req

	err := ForbidRequestFields("user_id", "tenant_id").Check(c, &Principal{Type: PrincipalCustomer, UserID: 1})
	if err == nil {
		t.Fatal("ForbidRequestFields() expected error")
	}
}

func TestLoginRequiredMiddlewareGuardDeniesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handlerCalled := false
	policy := Internal(
		WithFailureMode(FailureModeForbidden),
		WithGuard(ForbidRequestFields("user_id")),
	)
	route := NewRouteWithPolicy("svc", "/guard-deny", "", []string{"POST"}, policy, func(c *gin.Context) {
		handlerCalled = true
		c.Status(http.StatusOK)
	})

	engine := gin.New()
	engine.Use(LoginRequiredMiddleware([]*Route{route}))
	engine.POST("/guard-deny", route.GetHandlersChain()...)

	token, err := goodutils.GenJWT("svc", 3600)
	if err != nil {
		t.Fatalf("GenJWT() error = %v", err)
	}
	req := httptest.NewRequest("POST", "/guard-deny", bytes.NewReader([]byte(`{"user_id":"9"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
	if handlerCalled {
		t.Fatal("handler should not be called when guard denies request")
	}
}

func TestLoginRequiredMiddlewareGuardReadsBodyWithoutBreakingHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	policy := Public(
		WithGuard(NewGuard("read-scene-id", func(c *gin.Context, _ *Principal) error {
			if ResolveRequestValue(c, "scene_id") != "scene-1" {
				t.Fatalf("ResolveRequestValue() mismatch")
			}
			return nil
		})),
	)
	route := NewRouteWithPolicy("svc", "/guard-body", "", []string{"POST"}, policy, func(c *gin.Context) {
		var payload struct {
			SceneID string `json:"scene_id"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			t.Fatalf("ShouldBindJSON() error = %v", err)
		}
		c.JSON(http.StatusOK, gin.H{"scene_id": payload.SceneID})
	})

	engine := gin.New()
	engine.Use(LoginRequiredMiddleware([]*Route{route}))
	engine.POST("/guard-body", route.GetHandlersChain()...)

	req := httptest.NewRequest("POST", "/guard-body", bytes.NewReader([]byte(`{"scene_id":"scene-1"}`)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json unmarshal response error = %v", err)
	}
	if response["scene_id"] != "scene-1" {
		t.Fatalf("response scene_id = %v", response["scene_id"])
	}
}
