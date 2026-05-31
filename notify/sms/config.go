package sms

import (
	"fmt"
	"strings"
)

const (
	ProviderMock     = "mock"
	ProviderLuosimao = "luosimao"
	ProviderAliyun   = "aliyun"
)

// Config 是 go-common 侧的通用短信发送配置。
// 它只描述“如何把一条文本短信发出去”，不承载业务验证码场景语义。
type Config struct {
	Enabled         bool                   `json:"enabled"`
	Provider        string                 `json:"provider"`
	DefaultSign     string                 `json:"default_sign"`
	DefaultTemplate string                 `json:"default_template"`
	Mock            MockProviderConfig     `json:"mock"`
	Luosimao        LuosimaoProviderConfig `json:"luosimao"`
	Aliyun          AliyunProviderConfig   `json:"aliyun"`
}

type MockProviderConfig struct {
	Enabled bool `json:"enabled"`
}

type LuosimaoProviderConfig struct {
	Enabled  bool   `json:"enabled"`
	APIKey   string `json:"api_key"`
	Sign     string `json:"sign"`
	Endpoint string `json:"endpoint"`
}

type AliyunProviderConfig struct {
	Enabled         bool   `json:"enabled"`
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	Endpoint        string `json:"endpoint"`
	SignName        string `json:"sign_name"`
	TemplateCode    string `json:"template_code"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:         false,
		Provider:        ProviderMock,
		DefaultTemplate: "【通知】{{content}}",
		Mock: MockProviderConfig{
			Enabled: true,
		},
		Luosimao: LuosimaoProviderConfig{
			Enabled:  false,
			Endpoint: "https://sms-api.luosimao.com/v1/send.json",
		},
		Aliyun: AliyunProviderConfig{
			Enabled:  false,
			Endpoint: "dysmsapi.aliyuncs.com",
		},
	}
}

// Normalize 补齐默认值，保证发送器可以稳定读取配置。
// 这里也会把“当前选中的 provider”同步到对应 enabled 状态，避免配置语义分裂。
func (c *Config) Normalize() {
	if c == nil {
		return
	}
	defaults := DefaultConfig()
	c.Provider = normalizeProvider(c.Provider)
	if c.Provider == "" {
		c.Provider = defaults.Provider
	}
	if strings.TrimSpace(c.DefaultTemplate) == "" {
		c.DefaultTemplate = defaults.DefaultTemplate
	}
	if strings.TrimSpace(c.Luosimao.Endpoint) == "" {
		c.Luosimao.Endpoint = defaults.Luosimao.Endpoint
	}
	if strings.TrimSpace(c.Aliyun.Endpoint) == "" {
		c.Aliyun.Endpoint = defaults.Aliyun.Endpoint
	}
	switch c.Provider {
	case ProviderMock:
		c.Mock.Enabled = true
		c.Luosimao.Enabled = false
		c.Aliyun.Enabled = false
	case ProviderLuosimao:
		c.Mock.Enabled = false
		c.Luosimao.Enabled = true
		c.Aliyun.Enabled = false
	case ProviderAliyun:
		c.Mock.Enabled = false
		c.Luosimao.Enabled = false
		c.Aliyun.Enabled = true
	}
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	switch c.Provider {
	case ProviderMock:
		return nil
	case ProviderLuosimao:
		if !c.Luosimao.Enabled {
			return fmt.Errorf("Luosimao 短信供应商未启用")
		}
		if strings.TrimSpace(c.Luosimao.APIKey) == "" {
			return fmt.Errorf("Luosimao API Key 不能为空")
		}
		if strings.TrimSpace(c.Luosimao.Sign) == "" {
			return fmt.Errorf("Luosimao 短信签名不能为空")
		}
		return nil
	case ProviderAliyun:
		if !c.Aliyun.Enabled {
			return fmt.Errorf("阿里云短信供应商未启用")
		}
		if strings.TrimSpace(c.Aliyun.AccessKeyID) == "" {
			return fmt.Errorf("阿里云 AccessKey ID 不能为空")
		}
		if strings.TrimSpace(c.Aliyun.AccessKeySecret) == "" {
			return fmt.Errorf("阿里云 AccessKey Secret 不能为空")
		}
		if strings.TrimSpace(c.Aliyun.SignName) == "" {
			return fmt.Errorf("阿里云短信签名不能为空")
		}
		if strings.TrimSpace(c.Aliyun.TemplateCode) == "" {
			return fmt.Errorf("阿里云短信模板编码不能为空")
		}
		return nil
	default:
		return fmt.Errorf("不支持的短信供应商")
	}
}

func normalizeProvider(provider string) string {
	switch strings.TrimSpace(strings.ToLower(provider)) {
	case ProviderMock:
		return ProviderMock
	case ProviderLuosimao:
		return ProviderLuosimao
	case ProviderAliyun:
		return ProviderAliyun
	default:
		return ""
	}
}
