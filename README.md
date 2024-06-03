# go-common

go-common is a library for some common functions such as logging, configuration, orm, http client, http server, http routes, http rbac middleware etc.

# quick start

## installment

go get github.com/goodbye-jack/go-common


## config.yml

```
server_name: go-common
addr: ":8080"
```

## main.go

```
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
```

## go run main.go
