package tenant

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/utils"
)

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
