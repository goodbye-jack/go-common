package http

import (
	"context"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/approval"
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
	Method     string                 `json:"method"`
	ClientIP   string                 `json:"client_ip"`
	StatusCode int                    `json:"status_code"`
	Duration   int                    `json:"duration"`
	Body       map[string]interface{} `json:"body"`
	Tips       string                 `json:"tips"`
}

type OpRecordFn func(ctx context.Context, op Operation) error

type HTTPServer struct {
	service_name    string
	routes          []*Route
	router          *gin.Engine
	opRecordFn      OpRecordFn
	approvalHandler approval.ApprovalHandler
}

func init() {
	rbacClient = rbac.NewRbacClient(
		config.GetConfigString(utils.CasbinRedisAddrName),
	)

}

func NewHTTPServer(service_name string) *HTTPServer {
	routes := []*Route{
		NewRoute(service_name, "/ping", "健康检查", []string{"GET"}, "", false, false, func(c *gin.Context) {
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
	s.routes = append(s.routes, NewRoute(s.service_name, path, "", methods, role, sso, false, fn))
}

// RouteCarryLog Carrying logs
func (s *HTTPServer) RouteCarryLog(path string, tips string, methods []string, role string, sso bool, fn gin.HandlerFunc) {
	if len(methods) == 0 {
		methods = append(methods, "GET")
	}
	s.routes = append(s.routes, NewRoute(s.service_name, path, tips, methods, role, sso, false, fn))
}

func (s *HTTPServer) RouteAPI(path string, tips string, methods []string, role string, sso bool, business_approval bool, fn gin.HandlerFunc) {
	route := NewRoute(s.service_name, path, tips, methods, role, sso, business_approval, fn)
	// 添加业务审批中间件(如果需要)
	if business_approval && s.approvalHandler != nil {
		aConfig := approval.Config{
			BusinessApproval: business_approval,
			Handler:          s.approvalHandler,
		}
		route.AddMiddleware(approval.ApprovalMiddleware(aConfig))
	}
	//// 添加SSO中间件(如果需要)
	//if sso {
	//	route.AddMiddleware(s.ssoMiddleware)
	//}
	//if tips != "" && s.opRecordFn != nil {
	//	route.AddMiddleware(RecordOperationMiddleware(s.routes,s.opRecordFn))
	//}
	s.routes = append(s.routes, route)
}

// SetApprovalHandler 设置审批处理器
func (s *HTTPServer) SetApprovalHandler(handler approval.ApprovalHandler) {
	s.approvalHandler = handler
}

func (s *HTTPServer) SetOpRecordFn(fn OpRecordFn) {
	s.opRecordFn = fn
}

func (s *HTTPServer) Prepare() {
	//var policies []rbac.Policy
	//var routeInfos []*gin.RouteInfo
	//for _, route := range s.routes {
	//	policies = append(policies, route.ToRbacPolicy()...)
	//	routeInfos = append(routeInfos, route.ToGinRoute()...)
	//}
	//rbacClient.AddActionPolicies(policies)
	//
	//recordOperationMiddleware := RecordOperationMiddleware(s.routes, s.opRecordFn)
	//loginRequiredMiddleware := LoginRequiredMiddleware(s.routes)
	//rbacMiddleware := RbacMiddleware(s.service_name)
	//tenantMiddleware := TenantMiddleware()
	//
	//// 设置信任的代理IP（例如Nginx的IP）
	//s.router.SetTrustedProxies([]string{"127.0.0.1", "192.168.0.0/24"})
	//
	//s.router.Use(loginRequiredMiddleware)
	//s.router.Use(rbacMiddleware)
	//s.router.Use(tenantMiddleware)
	//s.router.Use(recordOperationMiddleware)
	//
	//for _, routeInfo := range routeInfos {
	//	s.router.Handle(routeInfo.Method, routeInfo.Path, routeInfo.HandlerFunc)
	//}
	var policies []rbac.Policy
	// 1. 收集所有路由的RBAC策略和路由信息
	for _, route := range s.routes {
		policies = append(policies, route.ToRbacPolicy()...)
	}
	// 2. 添加RBAC策略
	rbacClient.AddActionPolicies(policies)
	// 3. 设置全局中间件(注意顺序)
	s.router.SetTrustedProxies([]string{"127.0.0.1", "192.168.0.0/24"})
	// 4. 全局中间件(作用于所有路由)
	// 这里其实可以把必须检查的middleware放到这里注册,然后像操作记录还有是否登录的检查可以在每个RouteXXXX方法里实现,但是不想改了
	s.router.Use(
		LoginRequiredMiddleware(s.routes),                 // 登录检查
		RbacMiddleware(s.service_name),                    // RBAC鉴权
		TenantMiddleware(),                                // 租户隔离
		RecordOperationMiddleware(s.routes, s.opRecordFn), // 操作记录
	)
	// 5. 直接注册路由（不再使用routeInfos）
	for _, route := range s.routes {
		// 获取该路由的完整处理链（中间件+主处理函数）
		handlers := route.GetHandlersChain()
		// 为每个HTTP方法注册路由
		for _, method := range route.Methods {
			s.router.Handle(method, route.Url, handlers...)
		}
	}
}

func (s *HTTPServer) StaticFs(static_dir string) {
	s.router.Use(static.Serve("/static", static.LocalFile(static_dir, true)))
}

func (s *HTTPServer) Run(addr string) {
	log.Info("server %v is running", addr)
	s.router.Run(addr)
}
