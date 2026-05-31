package changeguard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	goodhttp "github.com/goodbye-jack/go-common/http"
)

const cachedRequestBodyKey = "__changeguard_cached_request_body"

func GetCachedRequestBody(c *gin.Context) ([]byte, error) {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return nil, nil
	}
	if raw, ok := c.Get(cachedRequestBodyKey); ok {
		if body, ok := raw.([]byte); ok {
			return body, nil
		}
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}
	c.Set(cachedRequestBodyKey, body)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func GetCachedJSONMap(c *gin.Context) (map[string]any, error) {
	body, err := GetCachedRequestBody(c)
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		return map[string]any{}, err
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return map[string]any{}, nil
	}
	return result, nil
}

func ResolvePrincipal(c *gin.Context) Principal {
	if c == nil {
		return Principal{}
	}
	if principal, ok := goodhttp.GetPrincipal(c); ok && principal != nil {
		return Principal{
			UserID:      firstString(uintToString(principal.UserID), principal.Subject, c.GetString("UserID"), c.GetString("user_id"), c.GetString("userId")),
			UserName:    firstString(principal.DisplayName, principal.Subject, c.GetString("user_name"), c.GetString("userName")),
			UserAccount: firstString(principal.Subject, c.GetString("user"), c.GetString("user_account")),
			TenantCode:  firstString(principal.TenantCode, c.GetString("tenant_code"), c.GetString("tenantCode")),
			ClientIP:    strings.TrimSpace(c.ClientIP()),
			ClientType:  strings.TrimSpace(c.GetHeader("User-Agent")),
		}
	}
	return Principal{
		UserID:      firstString(c.GetString("UserID"), c.GetString("user_id"), c.GetString("userId")),
		UserName:    firstString(c.GetString("user_name"), c.GetString("userName")),
		UserAccount: firstString(c.GetString("UserID"), c.GetString("user"), c.GetString("user_account")),
		TenantCode:  firstString(c.GetString("tenant_code"), c.GetString("tenantCode")),
		ClientIP:    strings.TrimSpace(c.ClientIP()),
		ClientType:  strings.TrimSpace(c.GetHeader("User-Agent")),
	}
}

func firstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uintToString(value uint) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprint(value)
}
