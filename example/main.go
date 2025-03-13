package main

import (
	"fmt"
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/config"
	myHttp "github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/utils"
)

type World struct {
	Name string `json:"name"`
}

func recordOp(ctx context.Context, op myHttp.Operation) error {
	fmt.Println("%+v", op)
	return nil
}

func main() {
	addr := config.GetConfigString("addr")
	service_name := config.GetConfigString("service_name")

	server := myHttp.NewHTTPServer(service_name)
	server.StaticFs("/static")
	server.Route("/hello", []string{"GET"}, utils.RoleAdministrator, false, func(c *gin.Context) {
		world := World{
			Name: "China",
		}
		myHttp.JsonResponse(c, world, nil)
	})
	server.Route("/hello/error", []string{"GET"}, utils.RoleAdministrator, false, func(c *gin.Context) {
		world := World{
			Name: "China",
		}
		myHttp.JsonResponse(c, world, errors.New("error"))
	})
	server.SetOpRecordFn(recordOp)
	server.Prepare()
	server.Run(addr)
}
