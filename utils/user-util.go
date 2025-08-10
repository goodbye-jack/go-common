package utils

import "github.com/gin-gonic/gin"

// GetUser 从gin上下文获取用户ID
func GetUser(c *gin.Context) string {
	user := c.GetString("UserID")
	if user == "" {
		return UserAnonymous
	}
	return user
}

// SetUser 向gin上下文设置用户ID
func SetUser(c *gin.Context, user string) {
	c.Set("UserID", user)
}

// GetTenant 从请求头获取租户信息
func GetTenant(c *gin.Context) string {
	tenant := c.Request.Header.Get("Tenant")
	if tenant == "" {
		return TenantAnonymous
	}
	return tenant
}
