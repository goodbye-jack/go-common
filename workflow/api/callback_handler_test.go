package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/workflow/assignment"
	"github.com/goodbye-jack/go-common/workflow/callback"
	"github.com/goodbye-jack/go-common/workflow/directory"
	"github.com/goodbye-jack/go-common/workflow/types"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type callbackClient struct {
	stubFlowableClient
	process func(context.Context, *types.FlowableCallbackPayload) error
}

func (client callbackClient) ProcessCallback(ctx context.Context, payload *types.FlowableCallbackPayload) error {
	return client.process(ctx, payload)
}

func signedCallbackRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/callback", strings.NewReader(body))
	request.Header.Set("X-Timestamp", "1234567890")
	request.Header.Set("X-Nonce", "test-nonce")
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write([]byte("1234567890\ntest-nonce\n" + body))
	request.Header.Set("X-Signature", hex.EncodeToString(mac.Sum(nil)))
	return request
}

func TestCallbackAcknowledgement(t *testing.T) {
	for _, test := range []struct {
		name                     string
		body                     string
		invalidSignature         bool
		engineError              error
		businessError            error
		withoutHook              bool
		status                   int
		wantEngine, wantBusiness int
	}{
		{name: "success", status: 200, wantEngine: 1, wantBusiness: 1},
		{name: "invalid signature", invalidSignature: true, status: 401},
		{name: "malformed JSON", body: "{", status: 400},
		{name: "engine failure", engineError: errors.New("engine unavailable"), status: 503, wantEngine: 1},
		{name: "business failure", businessError: errors.New("database unavailable"), status: 503, wantEngine: 1, wantBusiness: 1},
		{name: "business errors not classified as client errors", businessError: errors.New("business record not found invalid"), status: 503, wantEngine: 1, wantBusiness: 1},
		{name: "legacy no hook", withoutHook: true, status: 200, wantEngine: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			body := test.body
			if body == "" {
				body = `{"eventId":"event-1","eventType":"PROCESS_ENDED"}`
			}
			ctx.Request = signedCallbackRequest(body)
			if test.invalidSignature {
				ctx.Request.Header.Set("X-Signature", "invalid")
			}
			engineCalls, businessCalls := 0, 0
			module := &DefaultModule{callbackKey: "test-secret", flowable: callbackClient{process: func(context.Context, *types.FlowableCallbackPayload) error {
				engineCalls++
				return test.engineError
			}}}
			if !test.withoutHook {
				module.callbackHandler = func(context.Context, *types.FlowableCallbackPayload) error {
					businessCalls++
					if ctx.Writer.Written() || engineCalls != 1 {
						t.Error("business handler executed after acknowledgement or before engine")
					}
					return test.businessError
				}
			}
			module.handleCallback(ctx)
			if recorder.Code != test.status || engineCalls != test.wantEngine || businessCalls != test.wantBusiness {
				t.Fatalf("status=%d engine=%d business=%d", recorder.Code, engineCalls, businessCalls)
			}
			if test.businessError != nil && strings.Contains(recorder.Body.String(), test.businessError.Error()) {
				t.Error("internal error leaked")
			}
		})
	}
}

func TestCallbackHandlerRequiresSecret(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/callback", strings.NewReader(`{"eventId":"event-1","eventType":"PROCESS_ENDED"}`))
	calls := 0
	module := &DefaultModule{
		flowable:        callbackClient{process: func(context.Context, *types.FlowableCallbackPayload) error { calls++; return nil }},
		callbackHandler: func(context.Context, *types.FlowableCallbackPayload) error { calls++; return nil },
	}
	module.handleCallback(ctx)
	if recorder.Code != http.StatusServiceUnavailable || calls != 0 {
		t.Fatalf("status=%d calls=%d, want 503 and no processing", recorder.Code, calls)
	}
}

func TestSignedCallbackRetryWithTransactionalInbox(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "http-callback.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	if err := db.Exec("CREATE TABLE business_state (id INTEGER PRIMARY KEY, value INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO business_state VALUES (1, 0)").Error; err != nil {
		t.Fatal(err)
	}
	inbox, err := callback.NewInbox(db, "http-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := inbox.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	module := &DefaultModule{callbackKey: "test-secret", flowable: stubFlowableClient{}}
	module.callbackHandler = func(ctx context.Context, payload *types.FlowableCallbackPayload) error {
		_, err := inbox.Process(ctx, payload, func(tx *gorm.DB) error {
			attempts++
			if err := tx.Exec("UPDATE business_state SET value = value + 1 WHERE id = 1").Error; err != nil {
				return err
			}
			if attempts == 1 {
				return errors.New("injected failure after business write")
			}
			return nil
		})
		return err
	}
	for _, test := range []struct {
		invalidSignature        bool
		status, value, attempts int
	}{
		{true, 401, 0, 0},
		{false, 503, 0, 1},
		{false, 200, 1, 2},
		{false, 200, 1, 2},
	} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = signedCallbackRequest(`{"eventId":"stable-event","eventType":"PROCESS_ENDED"}`)
		if test.invalidSignature {
			ctx.Request.Header.Set("X-Signature", "wrong")
		}
		module.handleCallback(ctx)
		var value int
		if err := db.Raw("SELECT value FROM business_state WHERE id = 1").Scan(&value).Error; err != nil {
			t.Fatal(err)
		}
		if recorder.Code != test.status || value != test.value || attempts != test.attempts {
			t.Fatalf("status=%d value=%d attempts=%d; want %d %d %d", recorder.Code, value, attempts, test.status, test.value, test.attempts)
		}
	}
}

func TestCallbackHandlerForwardedFromConfig(t *testing.T) {
	settings := map[string]interface{}{
		"workflow.flowable.base_url": "http://127.0.0.1:1/flowable-rest/service",
		"workflow.flowable.username": "test", "workflow.flowable.password": "test",
	}
	for key, value := range settings {
		old := viper.Get(key)
		t.Cleanup(func() { viper.Set(key, old) })
		viper.Set(key, value)
	}
	called := false
	module, _, _, err := newModuleWithProviderFromConfig(RegisterOptions{
		DirectoryService: directory.NewNoopService(), AssignmentService: assignment.NewNoopService(),
		CallbackHandler: func(context.Context, *types.FlowableCallbackPayload) error { called = true; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if module.callbackHandler == nil {
		t.Fatal("config constructor dropped callback handler")
	}
	if err := module.callbackHandler(context.Background(), &types.FlowableCallbackPayload{}); err != nil || !called {
		t.Fatal("callback handler not forwarded")
	}
}
