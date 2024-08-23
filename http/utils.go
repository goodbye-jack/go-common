package http

import (
	"strings"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/utils"
)

func GetServiceName(c *gin.Context) string {
	log.Info("Header, Host(%s)", c.Request.Header.Get("Host"))
	log.Info("GetServiceName(%s)", c.Request.Host)
	parts := strings.Split(c.Request.Host, ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func GetUser(c *gin.Context) string {
	user := c.GetString("UserID")
	log.Info("GetUser(%s)", user)
	if user == "" {
		user = utils.UserAnonymous
	}
	return user
}

func SetUser(c *gin.Context, user string) {
	c.Set("UserID", user)
}

func GetTenant(c *gin.Context) string {
	tenant := c.Request.Header.Get("Tenant")
	if tenant == "" {
		return utils.TenantAnonymous
	}
	return tenant
}

func JsonResponse(c *gin.Context, data interface{}, err error) {
	statusCode := 200
	message := "success"

	if err != nil {
		data = nil
		message = whichError(err)
		statusCode = 500
	}

	c.JSON(statusCode, gin.H{
		"data":    data,
		"message": message,
	})
}
