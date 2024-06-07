package main

import (
	"net/http"
	"github.com/gin-gonic/gin"
	myHttp "github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/goodbye-jack/go-common/config"
)

func main() {
	addr := config.GetConfigString("addr")
	service_name := config.GetConfigString("service_name")
	server := myHttp.NewHTTPServer(service_name)
	server.Route("/hello", []string{"GET"}, utils.RoleManager, false, func(c *gin.Context) {
		c.String(http.StatusOK, "Hello, World")
	})

	server.Run(addr)
}
