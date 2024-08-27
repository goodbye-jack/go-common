package main

import (
	"errors"
	"github.com/gin-gonic/gin"
	myHttp "github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/goodbye-jack/go-common/config"
)

type World struct {
	Name string `json:"name"`
}

func main() {
	addr := config.GetConfigString("addr")
	service_name := config.GetConfigString("service_name")

	server := myHttp.NewHTTPServer(service_name)
	server.StaticFs(".")
	server.Route("/hello", []string{"GET"}, utils.RoleManager, false, func(c *gin.Context) {
		world := World {
			Name: "China",
		}
		myHttp.JsonResponse(c, world, nil)
	})
	server.Route("/hello/error", []string{"GET"}, utils.RoleManager, false, func(c *gin.Context) {
		world := World {
			Name: "China",
		}
		myHttp.JsonResponse(c, world, errors.New("error"))
	})
	server.Run(addr)
}
