package sms

import (
	"context"
	"testing"
)

func TestNewSenderAndSendWithMockProvider(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Provider = ProviderMock
	cfg.DefaultTemplate = "【关键资源变更】{{content}}"

	sender, err := NewSender(cfg)
	if err != nil {
		t.Fatalf("expected sender init success, got err=%v", err)
	}

	result, err := sender.Send(context.Background(), Message{
		Phone:   "13800138000",
		Content: "支付配置已变更",
	})
	if err != nil {
		t.Fatalf("expected mock send success, got err=%v", err)
	}
	if result == nil || result.Provider != ProviderMock {
		t.Fatalf("expected mock provider result, got %#v", result)
	}
}

func TestRenderContentAppendsLuosimaoSign(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Provider = ProviderLuosimao
	cfg.DefaultTemplate = "【关键资源变更】{{content}}"
	cfg.Luosimao.Sign = "盘龙科技"
	cfg.Normalize()

	sender := &Sender{config: cfg}

	rendered := sender.renderContent(Message{
		Content: "短信配置已更新",
	})
	expected := "【关键资源变更】短信配置已更新【盘龙科技】"
	if rendered != expected {
		t.Fatalf("expected rendered content %q, got %q", expected, rendered)
	}
}
