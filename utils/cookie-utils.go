package utils

import (
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/log"
)

func AddTokenCookie(c *gin.Context, token string, tokenExpired int) {
	tokenName := config.GetConfigString(ConfigNameToken)
	if tokenName == "" {
		log.Warn("!!!!!!!!!!!token name is empty!!!!!!!")
		tokenName = "good_token"
	}
	domainName := config.GetConfigString(ConfigNameDomain)
	log.Info("token name = %s", tokenName, domainName)

	c.SetCookie(tokenName, token, tokenExpired, "/", domainName, false, true)
}

func SetTokenCookie(c *gin.Context, token string, tokenExpired int, domain string, secure, httpOnly bool) {
	tokenName := config.GetConfigString(ConfigNameToken)
	if tokenName == "" {
		log.Warn("!!!!!!!!!!!token name is empty!!!!!!!")
		tokenName = "good_token"
	}
	log.Info("token name = %s", tokenName)
	c.SetCookie(tokenName, token, tokenExpired, "/", domain, secure, httpOnly)
}
