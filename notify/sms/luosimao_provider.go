package sms

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type LuosimaoProvider struct {
	config LuosimaoProviderConfig
	client *http.Client
}

type luosimaoResponse struct {
	Error   int    `json:"error"`
	Msg     string `json:"msg"`
	BatchID string `json:"batch_id"`
}

func NewLuosimaoProvider(cfg LuosimaoProviderConfig) *LuosimaoProvider {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		cfg.Endpoint = DefaultConfig().Luosimao.Endpoint
	}
	return &LuosimaoProvider{
		config: cfg,
		client: &http.Client{Timeout: 8 * time.Second},
	}
}

func (p *LuosimaoProvider) Name() string {
	return ProviderLuosimao
}

// Send 调用 Luosimao 单发文本短信接口。
// 这里直接发送最终渲染好的文本，不再耦合业务验证码模板。
func (p *LuosimaoProvider) Send(ctx context.Context, req SendRequest) (*SendResult, error) {
	form := url.Values{}
	form.Set("mobile", strings.TrimSpace(req.Phone))
	form.Set("message", strings.TrimSpace(req.Content))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("api:"+p.config.APIKey)))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{Provider: ProviderLuosimao, Message: "Luosimao 请求失败"}
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyText := strings.TrimSpace(string(bodyBytes))

	var payload luosimaoResponse
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return nil, &ProviderError{Provider: ProviderLuosimao, Message: "Luosimao 返回解析失败"}
	}
	if resp.StatusCode != http.StatusOK || payload.Error != 0 {
		return nil, &ProviderError{
			Provider: ProviderLuosimao,
			Code:     strings.TrimSpace(intToString(payload.Error)),
			Message:  chooseNonEmpty(payload.Msg, "Luosimao 短信发送失败"),
		}
	}
	return &SendResult{
		Provider:             ProviderLuosimao,
		ProviderRequestID:    payload.BatchID,
		ProviderResponseJSON: bodyText,
	}, nil
}
