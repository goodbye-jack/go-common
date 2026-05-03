package http

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/log"
)

const defaultCookieTokenName = "good_token"

// ResolveCookieTokenName 统一解析当前项目使用的 Cookie / Token 名称。
// go-common v1.3.3 起统一以 security.cookie.name 为准。
func ResolveCookieTokenName() string {
	tokenName := strings.TrimSpace(config.GetCookieTokenName())
	if tokenName != "" {
		return tokenName
	}
	log.Warnf("security.cookie.name 未配置，回退默认 token 名称：%s", defaultCookieTokenName)
	return defaultCookieTokenName
}

// GetTokenFromRequest 统一从请求中提取 token。
// 优先级：
// 1. Authorization: Bearer <token>
// 2. Cookie: <security.cookie.name>=<token>
func GetTokenFromRequest(c *gin.Context) (string, error) {
	if c == nil {
		return "", errors.New("request context is nil")
	}
	cred, err := ResolveCredentialFromRequest(c)
	if err != nil {
		return "", err
	}
	if cred == nil || strings.TrimSpace(cred.Token) == "" {
		return "", errors.New("missing token")
	}
	return strings.TrimSpace(cred.Token), nil
}
