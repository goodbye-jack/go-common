package api

import commonhttp "github.com/goodbye-jack/go-common/http"

type Module interface {
	Register(server *commonhttp.HTTPServer)
}
