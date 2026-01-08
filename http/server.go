package http

import (
	"context"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/approval"
	//_ "github.com/goodbye-jack/go-common/autoimport"
	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/rbac"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/spf13/viper"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"net/http"
	"sort"
	"sync"
	"time"
)

// RouteRegisterFunc 业务侧只需实现这个函数类型（替代繁琐的接口）
type RouteRegisterFunc func(server *HTTPServer)

// GroupRouterEntry 路由组注册项（原simpleRouterEntry，命名更语义化）
type GroupRouterEntry struct {
	groupName string            // 路由组名称（配置化用）
	prefix    string            // 路由组前缀
	priority  int               // 注册优先级
	mws       []gin.HandlerFunc // 组专属中间件
	register  RouteRegisterFunc // 路由注册函数
	enabled   bool              // 是否启用（默认true）
}

// 全局路由组注册器（命名同步优化）
var (
	rbacClient      *rbac.RbacClient = nil
	groupRegistry   []GroupRouterEntry
	groupRegistryMu sync.Mutex
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
	rbacClient = rbac.NewRbacClient(
		config.GetConfigString(utils.CasbinRedisAddrName),
	)
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
	//// 添加SSO中间件(如果需要)
	//if sso {
	//	route.AddMiddleware(s.ssoMiddleware)
	//}
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

// RegisterGroupRouter 注册路由组（原RegisterSimpleRouter，命名更直观）
// 参数：路由组名称、前缀、优先级、中间件、注册函数
func RegisterGroupRouter(groupName, prefix string, priority int, mws []gin.HandlerFunc, register RouteRegisterFunc) {
	groupRegistryMu.Lock()
	defer groupRegistryMu.Unlock()
	enabled := true // 从配置读取是否启用（默认启用）
	if item, ok := routeConfig.Routes[groupName]; ok {
		enabled = item.Enabled
	}
	groupRegistry = append(groupRegistry, GroupRouterEntry{
		groupName: groupName,
		prefix:    prefix,
		priority:  priority,
		mws:       mws,
		register:  register,
		enabled:   enabled,
	})
	log.Infof("已注册路由组：%s（前缀：%s，优先级：%d）", groupName, prefix, priority)
}

// -------------------------- 简化版初始化（兼容原有逻辑） --------------------------
// InitGroupAutoRoutes 初始化所有路由组
func (s *HTTPServer) InitGroupAutoRoutes() error {
	groupRegistryMu.Lock()
	entries := make([]GroupRouterEntry, len(groupRegistry))
	copy(entries, groupRegistry)
	groupRegistryMu.Unlock()
	// 按优先级排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority > entries[j].priority
	})
	registered := 0 // 注册路由
	for _, entry := range entries {
		if !entry.enabled {
			log.Infof("路由组[%s]已被配置禁用", entry.groupName)
			continue
		}
		originPrefix := s.globalPrefix // 1. 设置当前组前缀
		s.globalPrefix = entry.prefix
		if s.globalPrefix != "" && s.globalPrefix[len(s.globalPrefix)-1] != '/' {
			s.globalPrefix += "/"
		}
		if len(entry.mws) > 0 { // 2. 添加组中间件
			s.Use(entry.mws...)
		}
		entry.register(s)             // 3. 执行业务侧的路由注册函数
		s.globalPrefix = originPrefix // 4. 恢复原有前缀
		registered++
		log.Infof("已初始化路由组：%s", entry.groupName)
	}
	log.Infof("路由组自动初始化完成，共注册%d个路由组", registered)
	return nil
}

// SetApprovalHandler 设置审批处理器
func (s *HTTPServer) SetApprovalHandler(handler approval.ApprovalHandler) {
	s.approvalHandler = handler
}

func (s *HTTPServer) SetOpRecordFn(fn OpRecordFn) {
	s.opRecordFn = fn
}

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
