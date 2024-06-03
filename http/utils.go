package http

import (
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/utils"
)

func GetUser(c *gin.Context) string {
	user := c.Request.Header.Get("User")
	if user == "" {
		user = utils.UserAnonymous
	}
	return user
}

func GetTenant(c *gin.Context) string {
	tenant := c.Request.Header.Get("Tenant")
	if tenant == "" {
		return utils.TenantAnonymous
	}
	return tenant
}
