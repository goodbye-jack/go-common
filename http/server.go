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
	"github.com/spf13/viper"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"net/http"
	"time"
)

// 全局路由组注册器（命名同步优化）
var (
	rbacClient *rbac.RbacClient = nil
)

// routeConfig 存储路由组的启用/禁用配置（对应yaml中的routes节点）
var routeConfig = struct {
	Routes map[string]RouteConfigItem `yaml:"routes"`
}{}

// RouteConfigItem 路由组配置项（仅包含启用状态）
type RouteConfigItem struct {
	Enabled bool `yaml:"enabled"` // true=启用，false=禁用
}

type Operation struct {
	User          string                 `json:"user"`
	Time          time.Time              `json:"time"`
	Path          string                 `json:"path"`
	Method        string                 `json:"method"`
	ClientIP      string                 `json:"client_ip"`
	StatusCode    int                    `json:"status_code"`
	Duration      int                    `json:"duration"`
	Body          map[string]interface{} `json:"body"`
	Tips          string                 `json:"tips"`
	Authorization string                 `json:"authorization"` // 门户使用的token,在单独项目里拿不到，通过这里获取
}

type OpRecordFn func(ctx context.Context, op Operation) error

type HTTPServer struct {
	service_name     string
	routes           []*Route
	router           *gin.Engine
	opRecordFn       OpRecordFn
	approvalHandler  approval.ApprovalHandler
	extraMiddlewares []gin.HandlerFunc
	// 新增字段（增量，不影响原有逻辑）
	globalPrefix string // 路由全局前缀
}

func init() {
	rbacClient = rbac.NewRbacClient(config.GetConfigString(utils.CasbinRedisAddrName))
	// 设置默认值：如果配置文件中没有routes节点，默认所有路由组启用
	viper.SetDefault("routes", map[string]RouteConfigItem{})
	// 从配置文件加载routes配置到全局变量
	if err := viper.UnmarshalKey("routes", &routeConfig.Routes); err != nil {
		log.Warnf("加载路由组配置失败，将使用默认配置（所有路由组启用）：%v", err)
		// 初始化空map，避免后续nil指针错误
		routeConfig.Routes = make(map[string]RouteConfigItem)
	}
}

func NewHTTPServer(service_name string) *HTTPServer {
	routes := []*Route{
		NewRoute(service_name, "/ping", "健康检查", []string{"GET"}, utils.RoleIdle, false, false, func(c *gin.Context) {
			c.String(http.StatusOK, "Pong")
		}),
	}
	return &HTTPServer{
		service_name:     service_name,
		routes:           routes,
		router:           gin.Default(),
		extraMiddlewares: []gin.HandlerFunc{},
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

// RouteForRA 鉴定专用router,携带日志记录,明确角色
func (s *HTTPServer) RouteForRA(path string, tips string, methods []string, roles []string, sso bool, fn gin.HandlerFunc) {
	if len(methods) == 0 {
		methods = append(methods, "GET")
	}
	s.routes = append(s.routes, NewRouteForRA(s.service_name, path, tips, methods, roles, sso, false, fn))
}

func (s *HTTPServer) RouteAPI(path string, tips string, methods []string, roles []string, sso bool, business_approval bool, fn gin.HandlerFunc) {
	route := NewRouteCommon(s.service_name, path, tips, methods, roles, sso, business_approval, fn)
	// 添加业务审批中间件(如果需要)
	if business_approval && s.approvalHandler != nil {
		aConfig := approval.Config{
			BusinessApproval: business_approval,
			Handler:          s.approvalHandler,
		}
		route.AddMiddleware(approval.ApprovalMiddleware(aConfig))
	}
	if tips != "" && s.opRecordFn != nil {
		route.AddMiddleware(RecordOperationMiddleware(s.routes, s.opRecordFn))
	}
	s.routes = append(s.routes, route)
}

// RouteAutoRegisterAPI 业务侧调用的简化版API（自动拼接前缀、内置优先级）
// 参数：原有RouteAPI参数 + 组前缀（可选）
func (s *HTTPServer) RouteAutoRegisterAPI(path string, tips string, methods []string, roles []string,
	sso bool, businessApproval bool, fn gin.HandlerFunc, prefix ...string, // 可选前缀，不传则用全局前缀
) {
	// 拼接前缀（优先级：传入前缀 > 全局前缀）
	finalPrefix := s.globalPrefix
	if len(prefix) > 0 && prefix[0] != "" {
		finalPrefix = prefix[0]
	}
	// 处理前缀格式（确保以/结尾）
	if finalPrefix != "" && finalPrefix[len(finalPrefix)-1] != '/' {
		finalPrefix += "/"
	}
	// 最终路径 = 前缀 + 原始路径
	finalPath := finalPrefix + path
	// 复用原有RouteAPI逻辑
	s.RouteAPI(finalPath, tips, methods, roles, sso, businessApproval, fn)
}

// SetApprovalHandler 设置审批处理器
func (s *HTTPServer) SetApprovalHandler(handler approval.ApprovalHandler) {
	s.approvalHandler = handler
}

func (s *HTTPServer) SetOpRecordFn(fn OpRecordFn) {
	s.opRecordFn = fn
}

//// isRoutesInitialized 判断路由是否已经初始化
//func (s *HTTPServer) isRoutesInitialized() bool {
//	// 如果只有默认的 /ping 路由，认为没有初始化
//	if len(s.routes) <= 1 {
//		return false
//	}
//	for _, route := range s.routes { // 检查是否有自定义路由（非 /ping）
//		if route.Url != "/ping" {
//			return true
//		}
//	}
//	return false
//}

func (s *HTTPServer) Prepare() {
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
	// 先注册用户自定义额外中间件，再注册内置中间件，确保用户安全中间件可最早生效
	if len(s.extraMiddlewares) > 0 {
		s.router.Use(s.extraMiddlewares...)
	}
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

// Use 注册额外的全局中间件(将在 Prepare 时最先挂载)
func (s *HTTPServer) Use(middlewares ...gin.HandlerFunc) {
	if len(middlewares) == 0 {
		return
	}
	s.extraMiddlewares = append(s.extraMiddlewares, middlewares...)
}

func (s *HTTPServer) StaticFs(static_dir string) {
	//s.router.Use(static.Serve("/static", static.LocalFile(static_dir, true)))
	// 禁用/static访问,必须要有参数才能访问
	s.router.Use(static.Serve("/static", static.LocalFile(static_dir, false)))
}

func (s *HTTPServer) Run(addr string) {
	log.Info("server %v is running", addr)
	s.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	s.router.Run(addr)
}
