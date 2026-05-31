package changeguard

import (
	"context"
	"strings"

	"github.com/goodbye-jack/go-common/log"
	commonsms "github.com/goodbye-jack/go-common/notify/sms"
)

type SMSConfigResolver interface {
	Resolve(ctx context.Context) (commonsms.Config, error)
}

type SMSNotifier struct {
	resolver SMSConfigResolver
}

func NewSMSNotifier(resolver SMSConfigResolver) *SMSNotifier {
	return &SMSNotifier{resolver: resolver}
}

func (n *SMSNotifier) Name() string {
	return "sms"
}

// Notify 把 changeguard 的通知消息转成 go-common 通用短信发送请求。
// 这里不关心业务短信验证码场景，只负责发送最终通知文本。
func (n *SMSNotifier) Notify(ctx context.Context, msg Message) error {
	if n == nil || n.resolver == nil {
		return nil
	}
	cfg, err := n.resolver.Resolve(ctx)
	if err != nil {
		return err
	}
	sender, err := commonsms.NewSender(cfg)
	if err != nil {
		return err
	}
	content := strings.TrimSpace(msg.Content)
	for _, recipient := range msg.Recipients {
		recipient = strings.TrimSpace(recipient)
		if recipient == "" {
			continue
		}
		smsMessage := commonsms.Message{
			Phone:    recipient,
			Content:  content,
			Template: "",
			Variables: map[string]string{
				"content": content,
			},
		}
		finalContent := sender.PreviewContent(smsMessage)
		log.Infof("changeguard sms notify sending, event_id=%s, recipient=%s, content=%s", msg.EventID, recipient, finalContent)
		result, err := sender.Send(ctx, smsMessage)
		if err != nil {
			log.Warnf("changeguard sms notify failed, event_id=%s, recipient=%s, content=%s, err=%v", msg.EventID, recipient, finalContent, err)
			return err
		}
		if result != nil {
			log.Infof("changeguard sms notify success, event_id=%s, recipient=%s, provider=%s, request_id=%s, response=%s", msg.EventID, recipient, result.Provider, result.ProviderRequestID, result.ProviderResponseJSON)
		} else {
			log.Infof("changeguard sms notify success, event_id=%s, recipient=%s, provider=unknown", msg.EventID, recipient)
		}
	}
	return nil
}
