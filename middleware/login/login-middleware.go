package login

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/route"
	"github.com/goodbye-jack/go-common/utils"
	"net/http"
	"sync"
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

func LoginRequiredMiddleware(routes []*route.Route) gin.HandlerFunc {
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
			//goodhttp.SetUser(c, uid)
			utils.SetUser(c, uid)
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
				//goodhttp.SetUser(c, uid)
				utils.SetUser(c, uid)
			}
			c.Next()
		}
	}
}
