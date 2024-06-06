package http

import (
	"fmt"
	"context"
	"net/http"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/rbac"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/goodbye-jack/go-common/log"
)

func RbacMiddleware() gin.HandlerFunc {
	rbacClient := rbac.NewRbacClient()

	return func(c *gin.Context) {
		user := GetUser(c) 
		tenant := GetTenant(c) 
		req := rbac.NewReq(
			tenant,
			c.Request.Host,
			user,
			c.Request.URL.Path,
			c.Request.Method,
		)
		ok, err := rbacClient.Enforce(req)
		if err != nil {
			log.Errorf("RbacMiddleware, %v", err)
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
	return func(c *gin.Context) {
		sso := uniq2sso[fmt.Sprintf("%s_%s", c.Request.URL.Path, c.Request.Method)]
		u := GetUser(c)
		if u == utils.UserAnonymous && sso {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Next()
	}
}

func TenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
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
