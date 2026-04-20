package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/rbac"
	"github.com/goodbye-jack/go-common/utils"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// 定义一个全局的拦截过滤器接口
type SsoHandler interface {
	Verify(c *gin.Context) bool
}

var ssoHandlers map[string]SsoHandler = map[string]SsoHandler{}
var ssoMu sync.Mutex

func Register(name string, handler SsoHandler) {
	ssoMu.Lock()
	if _, ok := ssoHandlers[name]; ok {
		log.Warnf("GlobalSsoHandler %s has already been registered and will be replaced", name)
	}
	ssoHandlers[name] = handler
	ssoMu.Unlock()
}

func RbacMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		routePath := c.FullPath()
		if routePath == "" {
			routePath = c.Request.URL.Path
		}
		if strings.HasPrefix(routePath, "/static/") || strings.HasPrefix(routePath, "/static/") {
			c.Next()
			return
		}
		if strings.HasSuffix(routePath, "login") {
			c.Next()
			return
		}
		user := GetUser(c)
		req := rbac.NewReq(
			user,
			serviceName,
			routePath,
			c.Request.Method,
		)
		ok, err := RbacClient.Enforce(req)
		if err != nil {
			log.Errorf("RbacMiddleware/Enforce(%v), %v", *req, err)
		}
		if !ok {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}

func LoginRequiredMiddleware(routes []*Route) gin.HandlerFunc {
	uniq2sso := map[string]bool{}
	for _, route := range routes {
		sso, nonsso := route.ToSso()
		for _, uniq := range sso {
			uniq2sso[uniq] = true
		}
		for _, uniq := range nonsso {
			uniq2sso[uniq] = false
		}
	}
	tokenName := config.GetConfigString(utils.ConfigNameToken)
	return func(c *gin.Context) {
		routePath := c.FullPath()
		if routePath == "" {
			routePath = c.Request.URL.Path
		}
		sso := uniq2sso[fmt.Sprintf("%s_%s", routePath, c.Request.Method)]
		token, err := c.Cookie(tokenName)
		if sso { // 需要登录的逻辑
			if err != nil || token == "" {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			// ====以下  用于与统一登录对接时，需要每次都回调统一登录token验证====
			ssoEnabled := config.GetConfigString(utils.SsoEnabledVerify)
			if sso && ssoEnabled == "true" {
				nowHandlerName := config.GetConfigString(utils.SsoVerifyHandlerName)
				for name, handler := range ssoHandlers {
					if name == nowHandlerName {
						if !handler.Verify(c) {
							c.AbortWithStatus(http.StatusUnauthorized)
							return
						}
						break
					}
				}
			}
			// ====以上  用于与统一登录对接时，需要每次都回调统一登录token验证====
			uid, errP := utils.ParseJWT(token)
			if errP != nil {
				log.Warnf("Token parse failed, err=%v", errP)
				if sso {
					c.AbortWithStatus(http.StatusUnauthorized)
					return
				}
			}
			log.Infof("LoginRequiredMiddleware(), uid=%s", uid)
			SetUser(c, uid)
			c.Next()
			//return
		} else { // 不需要登录
			c.Next()
		}
	}
}

func TenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Info("TenantMiddleware()")
		tenant := c.Request.Header.Get(utils.TenantHeaderName)

		ctx := context.WithValue(
			c.Request.Context(),
			utils.TenantContextName,
			tenant,
		)

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func RecordOperationMiddleware(routes []*Route, fn OpRecordFn) gin.HandlerFunc {
	return RecordRequestMiddleware(routes, fn, nil)
}

type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyLogWriter) Write(data []byte) (int, error) {
	if w.body != nil {
		_, _ = w.body.Write(data)
	}
	return w.ResponseWriter.Write(data)
}

func (w *bodyLogWriter) WriteString(s string) (int, error) {
	if w.body != nil {
		_, _ = w.body.WriteString(s)
	}
	return w.ResponseWriter.WriteString(s)
}

func RecordRequestMiddleware(routes []*Route, opFn OpRecordFn, accessFn AccessRecordFn) gin.HandlerFunc {
	return func(c *gin.Context) {
		if opFn == nil && accessFn == nil {
			c.Next()
			return
		}

		start := time.Now()
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = ioutil.ReadAll(c.Request.Body)
			c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		routePath := c.FullPath()
		if routePath == "" {
			routePath = c.Request.URL.Path
		}
		tips := findRouteTips(routes, routePath, c.Request.Method)
		writer := &bodyLogWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBuffer(nil),
		}
		c.Writer = writer
		c.Next()

		bodyMap := parseRequestBodyMap(c, bodyBytes)

		clientIP := c.ClientIP()
		userID := GetUser(c)
		legacyUser := resolveLegacyUser(userID, bodyMap)
		userName := resolveUserName(userID, bodyMap)

		op := Operation{
			User:            legacyUser,
			UserID:          userID,
			UserName:        userName,
			Time:            start,
			LogTime:         start,
			URL:             c.Request.URL.RequestURI(),
			Path:            c.Request.URL.Path,
			Method:          c.Request.Method,
			QueryString:     c.Request.URL.RawQuery,
			ClientType:      strings.TrimSpace(c.GetHeader("X-Client-Type")),
			StatusCode:      c.Writer.Status(),
			Duration:        int(time.Since(start).Milliseconds()),
			DurationMs:      int(time.Since(start).Milliseconds()),
			Body:            bodyMap,
			HeaderContent:   buildHeaderContent(c.Request.Header),
			RequestContent:  buildRequestContent(c, bodyBytes, bodyMap),
			ResponseContent: buildResponseContent(c.Writer.Header().Get("Content-Type"), writer.body.Bytes()),
			Tips:            tips,
			ClientIP:        clientIP,
			Tenant:          GetTenant(c),
			Authorization:   c.GetHeader("Authorization"),
		}
		ctx := context.WithoutCancel(c.Request.Context())
		if opFn != nil && tips != "" {
			go func() {
				_ = opFn(ctx, op)
			}()
		}
		if accessFn != nil {
			go func() {
				_ = accessFn(ctx, op)
			}()
		}
	}
}

func findRouteTips(routes []*Route, routePath string, method string) string {
	for _, route := range routes {
		if route.Url != routePath {
			continue
		}
		for _, routeMethod := range route.Methods {
			if strings.EqualFold(routeMethod, method) {
				return route.Tips
			}
		}
	}
	return ""
}

func parseRequestBodyMap(c *gin.Context, bodyBytes []byte) map[string]interface{} {
	if len(bodyBytes) == 0 {
		return nil
	}
	contentType := strings.ToLower(c.ContentType())
	bodyMap := map[string]interface{}{}
	switch {
	case strings.HasPrefix(contentType, "application/json"):
		if err := json.Unmarshal(bodyBytes, &bodyMap); err == nil {
			return bodyMap
		}
	case strings.HasPrefix(contentType, "multipart/form-data"), strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
		_ = c.Request.ParseForm()
		for k, v := range c.Request.PostForm {
			if len(v) == 1 {
				bodyMap[k] = v[0]
			} else {
				bodyMap[k] = v
			}
		}
		if len(bodyMap) > 0 {
			return bodyMap
		}
	}
	return nil
}

func resolveUserName(userID string, bodyMap map[string]interface{}) string {
	for _, key := range []string{"user_name", "username", "userName", "phone", "mobile"} {
		if value, ok := bodyMap[key]; ok {
			if strValue, ok := value.(string); ok && strings.TrimSpace(strValue) != "" {
				return strings.TrimSpace(strValue)
			}
		}
	}
	userName := strings.TrimSpace(userID)
	if userName == "" || userName == utils.UserAnonymous {
		return userName
	}
	if idx := strings.LastIndex(userName, "#"); idx >= 0 && idx < len(userName)-1 {
		return userName[idx+1:]
	}
	return userName
}

func resolveLegacyUser(userID string, bodyMap map[string]interface{}) string {
	if value, ok := bodyMap["phone"]; ok {
		if strValue, ok := value.(string); ok && strings.TrimSpace(strValue) != "" {
			return strings.TrimSpace(strValue)
		}
	}
	return userID
}

func buildHeaderContent(header http.Header) string {
	if len(header) == 0 {
		return ""
	}
	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	payload := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		values := header.Values(key)
		maskedValues := make([]string, len(values))
		for idx, value := range values {
			maskedValues[idx] = maskSensitiveHeaderValue(key, value)
		}
		if len(maskedValues) == 1 {
			payload[key] = maskedValues[0]
		} else {
			payload[key] = maskedValues
		}
	}
	return marshalJSONString(payload)
}

func buildRequestContent(c *gin.Context, bodyBytes []byte, bodyMap map[string]interface{}) string {
	if len(bodyBytes) > 0 {
		contentType := strings.ToLower(c.ContentType())
		switch {
		case strings.HasPrefix(contentType, "application/json"):
			var payload interface{}
			if err := json.Unmarshal(bodyBytes, &payload); err == nil {
				return marshalJSONString(sanitizePayload(payload))
			}
		case strings.HasPrefix(contentType, "multipart/form-data"), strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
			if bodyMap != nil {
				return marshalJSONString(sanitizePayload(bodyMap))
			}
		default:
			if text, ok := sanitizeRawText(string(bodyBytes)); ok {
				return text
			}
		}
	}
	if c.Request.URL.RawQuery != "" {
		return c.Request.URL.RawQuery
	}
	return ""
}

func buildResponseContent(contentType string, bodyBytes []byte) string {
	if len(bodyBytes) == 0 {
		return ""
	}
	lowerContentType := strings.ToLower(contentType)
	switch {
	case strings.HasPrefix(lowerContentType, "application/json"):
		var payload interface{}
		if err := json.Unmarshal(bodyBytes, &payload); err == nil {
			return marshalJSONString(payload)
		}
	case strings.HasPrefix(lowerContentType, "text/"), strings.Contains(lowerContentType, "xml"), strings.Contains(lowerContentType, "javascript"), lowerContentType == "":
		if utf8.Valid(bodyBytes) {
			return string(bodyBytes)
		}
	default:
		return "[binary response omitted]"
	}
	if utf8.Valid(bodyBytes) {
		return string(bodyBytes)
	}
	return "[binary response omitted]"
}

func maskSensitiveHeaderValue(key string, value string) string {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	switch lowerKey {
	case "authorization", "cookie", "set-cookie":
		if strings.TrimSpace(value) == "" {
			return value
		}
		return "***"
	default:
		return value
	}
}

func sanitizePayload(value interface{}) interface{} {
	switch typedValue := value.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(typedValue))
		for key, item := range typedValue {
			if isSensitiveField(key) {
				result[key] = "***"
				continue
			}
			result[key] = sanitizePayload(item)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(typedValue))
		for idx, item := range typedValue {
			result[idx] = sanitizePayload(item)
		}
		return result
	default:
		return typedValue
	}
}

func isSensitiveField(key string) bool {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(lowerKey, "password") || strings.Contains(lowerKey, "pwd") || strings.Contains(lowerKey, "token") || strings.Contains(lowerKey, "secret")
}

func sanitizeRawText(text string) (string, bool) {
	if !utf8.ValidString(text) {
		return "", false
	}
	return strings.TrimSpace(text), true
}

func marshalJSONString(value interface{}) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}
