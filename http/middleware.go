package http

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/rbac"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/goodbye-jack/go-common/config"
	"net/http"
)

func RbacMiddleware() gin.HandlerFunc {
	log.Info("RbacMiddleware")
	return func(c *gin.Context) {
		log.Info("RbacMiddleware()")
		user := GetUser(c)
		serviceName := GetServiceName(c)
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
	log.Info("token name is %s", tokenName)
	return func(c *gin.Context) {
		log.Info("LoginRequiredMiddleware()")
		sso := uniq2sso[fmt.Sprintf("%s_%s", c.Request.URL.Path, c.Request.Method)]
		if sso {
			token, err := c.Cookie(tokenName)
			//Cookie not existed
			if err != nil {
				log.Warn("Cookie/%s not existed, %v", tokenName, err)
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			//Token expired
			if _, err := utils.ParseJWT(token); err != nil {
				log.Warn("Token(%s) expired, %v", token, err)
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
		}

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
