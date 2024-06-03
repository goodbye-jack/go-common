package main

import (
	"github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/config"
)

func main() {
	addr := config.GetConfigString("addr")
	service_name := config.GetConfigString("service_name")
	server := http.NewHTTPServer(service_name)
	server.Run(addr)
}
