package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/utils"
	dysmsapi "github.com/alibabacloud-go/dysmsapi-20170525/v5/client"
	teautil "github.com/alibabacloud-go/tea-utils/v2/service"
)

const aliyunDefaultRegionID = "cn-hangzhou"

type aliyunSMSClient interface {
	SendSmsWithContext(ctx context.Context, request *dysmsapi.SendSmsRequest, runtime *teautil.RuntimeOptions) (*dysmsapi.SendSmsResponse, error)
}

type AliyunProvider struct {
	config  AliyunProviderConfig
	client  aliyunSMSClient
	runtime *teautil.RuntimeOptions
}

func NewAliyunProvider(cfg AliyunProviderConfig) (*AliyunProvider, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		cfg.Endpoint = DefaultConfig().Aliyun.Endpoint
	}
	client, err := dysmsapi.NewClient(&openapi.Config{
		AccessKeyId:     stringPtr(strings.TrimSpace(cfg.AccessKeyID)),
		AccessKeySecret: stringPtr(strings.TrimSpace(cfg.AccessKeySecret)),
		Endpoint:        stringPtr(strings.TrimSpace(cfg.Endpoint)),
		RegionId:        stringPtr(aliyunDefaultRegionID),
	})
	if err != nil {
		return nil, fmt.Errorf("初始化阿里云短信客户端失败")
	}
	return &AliyunProvider{
		config: cfg,
		client: client,
		runtime: &teautil.RuntimeOptions{
			ConnectTimeout: intPtr(int((5 * time.Second) / time.Millisecond)),
			ReadTimeout:    intPtr(int((8 * time.Second) / time.Millisecond)),
			MaxAttempts:    intPtr(1),
			Autoretry:      boolPtr(false),
		},
	}, nil
}

func (p *AliyunProvider) Name() string {
	return ProviderAliyun
}

// Send 调用阿里云短信发送接口。
// 这里默认约定业务侧在模板里使用 `${content}` 变量承载整段通知文本。
func (p *AliyunProvider) Send(ctx context.Context, req SendRequest) (*SendResult, error) {
	templateParamBytes, err := json.Marshal(map[string]string{
		"content": req.Content,
	})
	if err != nil {
		return nil, fmt.Errorf("构造阿里云短信模板参数失败")
	}
	resp, err := p.client.SendSmsWithContext(ctx, (&dysmsapi.SendSmsRequest{}).
		SetPhoneNumbers(strings.TrimSpace(req.Phone)).
		SetSignName(strings.TrimSpace(p.config.SignName)).
		SetTemplateCode(strings.TrimSpace(p.config.TemplateCode)).
		SetTemplateParam(string(templateParamBytes)), p.runtime)
	if err != nil {
		return nil, &ProviderError{
			Provider: ProviderAliyun,
			Message:  "阿里云短信请求失败",
		}
	}
	if resp == nil || resp.Body == nil {
		return nil, &ProviderError{
			Provider: ProviderAliyun,
			Message:  "阿里云短信返回为空",
		}
	}
	bodyJSONBytes, _ := json.Marshal(resp.Body)
	bodyCode := strings.TrimSpace(stringValue(resp.Body.Code))
	bodyMessage := strings.TrimSpace(stringValue(resp.Body.Message))
	if bodyCode != "OK" {
		return nil, &ProviderError{
			Provider: ProviderAliyun,
			Code:     bodyCode,
			Message:  chooseNonEmpty(bodyMessage, "阿里云短信发送失败"),
		}
	}
	return &SendResult{
		Provider:             ProviderAliyun,
		ProviderRequestID:    stringValue(resp.Body.RequestId),
		ProviderResponseJSON: string(bodyJSONBytes),
	}, nil
}

func stringPtr(v string) *string { return &v }
func intPtr(v int) *int          { return &v }
func boolPtr(v bool) *bool       { return &v }

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
