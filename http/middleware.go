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
	"strings"
	"sync"
	"time"
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
		fmt.Printf("GlobalSsoHandler %s had registerd", name)
	}
	ssoHandlers[name] = handler
	ssoMu.Unlock()
}

func RbacMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/static/") || strings.HasPrefix(c.Request.URL.Path, "/static/") {
			c.Next()
			return
		}
		user := GetUser(c)
		req := rbac.NewReq(
			user,
			serviceName,
			c.Request.URL.Path,
			c.Request.Method,
		)
		ok, err := rbacClient.Enforce(req)
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
		sso := uniq2sso[fmt.Sprintf("%s_%s", c.Request.URL.Path, c.Request.Method)]
		token, err := c.Cookie(tokenName)
		if sso { // 需要登录的逻辑
			if err != nil || token == "" {
				c.AbortWithStatus(http.StatusUnauthorized)
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
				log.Warn("Token(%s) expired, %v", token, errP)
				if sso {
					c.AbortWithStatus(http.StatusUnauthorized)
					return
				}
			}
			log.Info("LoginRequiredMiddleware(), uid=%s", uid)
			SetUser(c, uid)
			c.Next()
			//return
		} else { // 不需要登录
			if err == nil && token != "" { //
				//Token expired
				uid, errP := utils.ParseJWT(token)
				if errP != nil {
					log.Warn("Token(%s) expired, %v", token, errP)
					if sso {
						c.AbortWithStatus(http.StatusUnauthorized)
						return
					}
				}
				log.Info("LoginRequiredMiddleware(), uid=%s", uid)
				SetUser(c, uid)
			}
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
	return func(c *gin.Context) {
		if fn == nil {
			log.Warn("RecorOperationMiddleware callback not set")
			c.Next()
			return
		}

		start := time.Now()
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = ioutil.ReadAll(c.Request.Body)
			c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		tips := ""
		for _, route := range routes {
			if route.Url == c.Request.URL.Path {
				for _, method := range route.Methods {
					if method == c.Request.Method {
						tips = route.Tips
					}
				}
				if tips != "" {
					break
				}
			}
		}

		c.Next()

		var bodyMap map[string]interface{}
		contentType := c.ContentType()
		if len(bodyBytes) > 0 {
			switch {
			case contentType == "application/json":
				json.Unmarshal(bodyBytes, &bodyMap)
			case contentType == "multipart/form-data" || contentType == "application/x-www-form-urlencoded":
				c.Request.ParseForm()
				bodyMap = make(map[string]interface{})
				for k, v := range c.Request.PostForm {
					if len(v) == 1 {
						bodyMap[k] = v[0]
					} else {
						bodyMap[k] = v
					}
				}
			}
		}

		clientIP := c.ClientIP()
		user := GetUser(c)
		// login supported
		if u, ok := bodyMap["phone"]; ok {
			if ustr, ok := u.(string); ok {
				user = ustr
			}
		}

		op := Operation{
			User:       user,
			Time:       start,
			Path:       c.Request.URL.Path,
			Method:     c.Request.Method,
			StatusCode: c.Writer.Status(),
			Duration:   int(time.Since(start).Milliseconds()),
			Body:       bodyMap,
			Tips:       tips,
			ClientIP:   clientIP,
		}
		log.Info("RecordOperaionMiddlware callback starting")

		// 异步记录日志
		go func() {
			_ = fn(context.Background(), op)
		}()
	}
}
