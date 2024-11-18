package http

import (
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/utils"
)

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

func AddTokenCookie(c *gin.Context, token string, tokenExpired int) {
	tokenName := config.GetConfigString(utils.ConfigNameToken)
	if tokenName == "" {
		log.Warn("!!!!!!!!!!!token name is empty!!!!!!!")
		tokenName = "good-token"
	}
	log.Info("token name = %s", tokenName)

	c.SetCookie(tokenName, token, tokenExpired, "/", "", false, true)
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
