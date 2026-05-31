package sms

import (
	"context"
	"fmt"
	"strings"
)

type Sender struct {
	config Config
}

type Message struct {
	Phone     string
	Content   string
	Template  string
	Variables map[string]string
}

func NewSender(cfg Config) (*Sender, error) {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Sender{config: cfg}, nil
}

// Send 发送一条文本短信。
// 对 Luosimao 这类文本通道直接发送渲染后内容；对阿里云则约定模板中使用 content 变量。
func (s *Sender) Send(ctx context.Context, msg Message) (*SendResult, error) {
	if s == nil {
		return nil, fmt.Errorf("短信发送器未初始化")
	}
	if !s.config.Enabled {
		return nil, nil
	}
	phone := strings.TrimSpace(msg.Phone)
	if phone == "" {
		return nil, fmt.Errorf("短信接收手机号不能为空")
	}
	content := s.renderContent(msg)
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("短信内容不能为空")
	}
	provider, err := NewProvider(s.config)
	if err != nil {
		return nil, err
	}
	return provider.Send(ctx, SendRequest{
		Phone:   phone,
		Content: content,
	})
}

// PreviewContent 返回最终将发给短信供应商的完整文本，便于外部做审计日志或调试输出。
func (s *Sender) PreviewContent(msg Message) string {
	if s == nil {
		return ""
	}
	return s.renderContent(msg)
}

// renderContent 统一把业务侧传入的 content/template/variables 渲染成最终短信文本。
// 这样 changeguard 或其他模块只需要关注“发什么内容”，不需要关注不同供应商的文本拼装差异。
func (s *Sender) renderContent(msg Message) string {
	template := chooseNonEmpty(msg.Template, s.config.DefaultTemplate)
	content := chooseNonEmpty(msg.Content, "")
	variables := map[string]string{
		"content": content,
		"sign":    chooseNonEmpty(s.config.DefaultSign, s.config.Luosimao.Sign, s.config.Aliyun.SignName),
	}
	for key, value := range msg.Variables {
		variables[key] = value
	}
	rendered := renderTemplate(template, variables)
	if s.config.Provider == ProviderLuosimao {
		sign := chooseNonEmpty(s.config.Luosimao.Sign, s.config.DefaultSign)
		if sign != "" && !strings.Contains(rendered, "【"+sign+"】") {
			rendered = strings.TrimSpace(rendered) + "【" + sign + "】"
		}
	}
	return rendered
}

// NewProvider 负责根据通用配置选择具体短信供应商实现。
// 这里不依赖任何业务包，确保 go-common 可以被多个后端项目直接复用。
func NewProvider(cfg Config) (Provider, error) {
	cfg.Normalize()
	switch cfg.Provider {
	case ProviderMock:
		return MockProvider{}, nil
	case ProviderLuosimao:
		if !cfg.Luosimao.Enabled {
			return nil, fmt.Errorf("Luosimao 短信供应商未启用")
		}
		return NewLuosimaoProvider(cfg.Luosimao), nil
	case ProviderAliyun:
		if !cfg.Aliyun.Enabled {
			return nil, fmt.Errorf("阿里云短信供应商未启用")
		}
		return NewAliyunProvider(cfg.Aliyun)
	default:
		return nil, fmt.Errorf("不支持的短信供应商")
	}
}
