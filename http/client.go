package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/utils"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPClient struct {
	client         *http.Client
	tenant         string
	service_name   string
	service_domain string
}

var uniq2client map[string]*HTTPClient = map[string]*HTTPClient{}

func genUniq(tenant, service_name string) string {
	if tenant == utils.TenantAnonymous {
		return service_name
	}
	return fmt.Sprintf("%s_%s", tenant, service_name)
}

func NewHTTPClient(tenant, service_name string) *HTTPClient {
	log.Info("u2c, %v", uniq2client)
	uniq := genUniq(tenant, service_name)
	if client, ok := uniq2client[uniq]; ok {
		return client
	}
	service_domain := config.GetConfigString(service_name)
	service_domain, _ = strings.CutSuffix(service_domain, "/")
	if !strings.HasPrefix(service_domain, "http://") && !strings.HasPrefix(service_domain, "https://") {
		service_domain = fmt.Sprintf("http://%s", service_domain)
	}
	transport := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
	}
	uniq2client[uniq] = &HTTPClient{
		tenant:         tenant,
		service_name:   service_name,
		service_domain: service_domain,
		client: &http.Client{
			Transport: transport,
		},
	}
	log.Infof("the uniq %s create newHTTPClient %+v", uniq, uniq2client[uniq])
	return uniq2client[uniq]
}

func (c *HTTPClient) genAbsUrl(url string) string {
	// 如果 url 已经是完整 URL（包含 http:// 或 https://），直接返回
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}
	// 否则拼接基础路径（如果有）
	return fmt.Sprintf("%s%s", c.service_domain, url)
}

func (c *HTTPClient) do(ctx context.Context, method, url string, data []byte, headers map[string]string) ([]byte, error) {
	if c == nil {
		return nil, errors.New("HTTPClient is nil")
	}
	absUrl := c.genAbsUrl(url)
	log.Info("HTTPClient.do absUrl=%s", absUrl)
	//Body
	buf := bytes.NewBuffer(data)
	req, err := http.NewRequestWithContext(ctx, method, absUrl, buf)
	if err != nil {
		log.Error("do(%s, %s) error, %v", method, absUrl, err)
		return nil, err
	}
	//Header
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		log.Error("do/http.Do() error, %v", err)
		return nil, err
	}
	//Response
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("do/io.ReadAll() error, %v", err)
		return nil, err
	}
	log.Info("%s(%s) status code %d", method, absUrl, resp.StatusCode)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New(
			fmt.Sprintf("%s(%s) statusCode=%d, %s", method, absUrl, resp.StatusCode, string(body)),
		)
	}
	log.Info("%s(url, %s): %s", method, absUrl, string(data), string(body))
	return body, nil
}

func (c *HTTPClient) Get(ctx context.Context, url string, data []byte, headers map[string]string) ([]byte, error) {
	return c.do(ctx, "GET", url, data, headers)
}

func (c *HTTPClient) Post(ctx context.Context, url string, data []byte, headers map[string]string) ([]byte, error) {
	return c.do(ctx, "POST", url, data, headers)
}

func (c *HTTPClient) Put(ctx context.Context, url string, data []byte, headers map[string]string) ([]byte, error) {
	return c.do(ctx, "PUT", url, data, headers)
}

func (c *HTTPClient) Delete(ctx context.Context, url string, data []byte, headers map[string]string) ([]byte, error) {
	return c.do(ctx, "DELETE", url, data, headers)
}
