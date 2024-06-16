package http

import (
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/rbac"
	"github.com/goodbye-jack/go-common/utils"
	"net/http"
)

var (
	rbacClient *rbac.RbacClient = nil
)

type HTTPServer struct {
	service_name string
	routes       []*Route
	router       *gin.Engine
}

func init() {
	rbacClient = rbac.NewRbacClient(
                config.GetConfigString(utils.CasbinRedisAddrName),
        )

}

func NewHTTPServer(service_name string) *HTTPServer {
	routes := []*Route{
		NewRoute(service_name, "/ping", []string{"GET"}, "", false, func(c *gin.Context) {
			c.String(http.StatusOK, "Pong")
		}),
	}

	return &HTTPServer{
		service_name: service_name,
		routes:       routes,
		router:       gin.Default(),
	}
}

func (s *HTTPServer) Route(path string, methods []string, role string, sso bool, fn gin.HandlerFunc) {
	if len(methods) == 0 {
		methods = append(methods, "GET")
	}
	s.routes = append(s.routes, NewRoute(s.service_name, path, methods, role, sso, fn))
}

func (s *HTTPServer) prepare() {
	policies := []rbac.Policy{}
	routeInfos := []*gin.RouteInfo{}
	for _, route := range s.routes {
		policies = append(policies, route.ToRbacPolicy()...)
		routeInfos = append(routeInfos, route.ToGinRoute()...)
	}
	rbacClient.AddActionPolicies(policies)

	for _, routeInfo := range routeInfos {
		s.router.Handle(routeInfo.Method, routeInfo.Path, routeInfo.HandlerFunc)
	}

	loginRequiredMiddleware := LoginRequiredMiddleware(s.routes)
	rbacMiddleware := RbacMiddleware()
	tenantMiddleware := TenantMiddleware()

	s.router.Use(loginRequiredMiddleware)
	s.router.Use(rbacMiddleware)
	s.router.Use(tenantMiddleware)
}

func (s *HTTPServer) Run(addr string) {
	log.Infof("server %v is running", addr)
	s.prepare()
	s.router.Run(addr)
}
