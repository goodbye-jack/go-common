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
	"time"
)

func RbacMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/static/") {
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
		//Cookie not existed
		if err != nil || token == "" {
			log.Warn("Cookie/%s not existed, %v", tokenName, err)
			if sso {
				c.AbortWithStatus(http.StatusUnauthorized)
			}
			c.Next()
			return
		}
		//Token expired
		uid, err := utils.ParseJWT(token)
		if err != nil {
			log.Warn("Token(%s) expired, %v", token, err)
			if sso {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
		}
		log.Info("LoginRequiredMiddleware(), uid=%s", uid)
		SetUser(c, uid)

		c.Next()
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
			if route.Url == c.FullPath() {
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

		op := Operation{
			User:       GetUser(c),
			Time:       start,
			Path:       c.FullPath(),
			Method:     c.Request.Method,
			StatusCode: c.Writer.Status(),
			Duration:   int(time.Since(start).Milliseconds()),
			Body:       bodyMap,
			Tips:       tips,
		}
		log.Info("RecordOperaionMiddlware callback starting")

		// 异步记录日志
		go func() {
			_ = fn(c.Request.Context(), op)
		}()
	}
}
