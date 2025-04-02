package http

import (
	"context"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/rbac"
	"github.com/goodbye-jack/go-common/utils"
	"net/http"
	"time"
)

var (
	rbacClient *rbac.RbacClient = nil
)

type Operation struct {
	User       string                 `json:"user"`
	Time       time.Time              `json:"time"`
	Path       string                 `json:"path"`
	Method     string                 `json:"path"`
	ClientIP   string                 `json:"client_ip"`
	StatusCode int                    `json:"status_code"`
	Duration   int                    `json:"duration"`
	Body       map[string]interface{} `json:"body"`
	Tips       string                 `json:"tips"`
}

type OpRecordFn func(ctx context.Context, op Operation) error

type HTTPServer struct {
	service_name string
	routes       []*Route
	router       *gin.Engine
	opRecordFn   OpRecordFn
}

func init() {
	rbacClient = rbac.NewRbacClient(
		config.GetConfigString(utils.CasbinRedisAddrName),
	)

}

func NewHTTPServer(service_name string) *HTTPServer {
	routes := []*Route{
		NewRoute(service_name, "/ping", "健康检查", []string{"GET"}, "", false, func(c *gin.Context) {
			c.String(http.StatusOK, "Pong")
		}),
	}

	return &HTTPServer{
		service_name: service_name,
		routes:       routes,
		router:       gin.Default(),
	}
}

func (s *HTTPServer) GetRoutes() []*Route {
	return s.routes
}

func (s *HTTPServer) GetDefaultRoles() []string {
	return []string{
		utils.RoleAdministrator,
	}
}

func (s *HTTPServer) Route(path string, methods []string, role string, sso bool, fn gin.HandlerFunc) {
	if len(methods) == 0 {
		methods = append(methods, "GET")
	}
	s.routes = append(s.routes, NewRoute(s.service_name, path, "", methods, role, sso, fn))
}

// RouteCarryLog Carrying logs
func (s *HTTPServer) RouteCarryLog(path string, tips string, methods []string, role string, sso bool, fn gin.HandlerFunc) {
	if len(methods) == 0 {
		methods = append(methods, "GET")
	}
	s.routes = append(s.routes, NewRoute(s.service_name, path, tips, methods, role, sso, fn))
}

func (s *HTTPServer) SetOpRecordFn(fn OpRecordFn) {
	s.opRecordFn = fn
}

func (s *HTTPServer) Prepare() {
	policies := []rbac.Policy{}
	routeInfos := []*gin.RouteInfo{}
	for _, route := range s.routes {
		policies = append(policies, route.ToRbacPolicy()...)
		routeInfos = append(routeInfos, route.ToGinRoute()...)
	}
	rbacClient.AddActionPolicies(policies)

	recordOperationMiddleware := RecordOperationMiddleware(s.routes, s.opRecordFn)
	loginRequiredMiddleware := LoginRequiredMiddleware(s.routes)
	rbacMiddleware := RbacMiddleware(s.service_name)
	tenantMiddleware := TenantMiddleware()

	// 设置信任的代理IP（例如Nginx的IP）
	s.router.SetTrustedProxies([]string{"127.0.0.1", "192.168.0.0/24"})

	s.router.Use(loginRequiredMiddleware)
	s.router.Use(rbacMiddleware)
	s.router.Use(tenantMiddleware)
	s.router.Use(recordOperationMiddleware)

	for _, routeInfo := range routeInfos {
		s.router.Handle(routeInfo.Method, routeInfo.Path, routeInfo.HandlerFunc)
	}
}

func (s *HTTPServer) StaticFs(static_dir string) {
	s.router.Use(static.Serve("/static", static.LocalFile(static_dir, true)))
}

func (s *HTTPServer) Run(addr string) {
	log.Info("server %v is running", addr)
	s.router.Run(addr)
}
