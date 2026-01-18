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
	// 新增：标记路由组是否已经执行过
	groupExecutedMu sync.Mutex
	groupExecuted   = make(map[string]bool)

	globalRouteFuncs []RouteRegisterFunc
	routeFuncsMu     sync.Mutex
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

// SetApprovalHandler 设置审批处理器
func (s *HTTPServer) SetApprovalHandler(handler approval.ApprovalHandler) {
	s.approvalHandler = handler
}

func (s *HTTPServer) SetOpRecordFn(fn OpRecordFn) {
	s.opRecordFn = fn
}

// isRoutesInitialized 判断路由是否已经初始化
func (s *HTTPServer) isRoutesInitialized() bool {
	// 如果只有默认的 /ping 路由，认为没有初始化
	if len(s.routes) <= 1 {
		return false
	}

	// 检查是否有自定义路由（非 /ping）
	for _, route := range s.routes {
		if route.Url != "/ping" {
			return true
		}
	}

	return false
}

// AutoRegisterRoutes 让业务模块自动注册路由
func AutoRegisterRoutes(fn RouteRegisterFunc) {
	routeFuncsMu.Lock()
	defer routeFuncsMu.Unlock()
	globalRouteFuncs = append(globalRouteFuncs, fn)
	log.Debugf("路由函数已注册，总数: %d", len(globalRouteFuncs))
}

// ExecuteAllRouteGroups 执行所有已注册的路由函数
func ExecuteAllRouteGroups(server *HTTPServer) {
	routeFuncsMu.Lock() // 获取所有注册的函数
	funcs := make([]RouteRegisterFunc, len(globalRouteFuncs))
	copy(funcs, globalRouteFuncs)
	routeFuncsMu.Unlock()
	if len(funcs) == 0 {
		log.Info("没有需要执行的路由函数")
		return
	}
	log.Infof("开始执行 %d 个路由组", len(funcs))
	// 执行每个路由注册函数
	for i, fn := range funcs {
		log.Debugf("执行路由组[%d/%d]", i+1, len(funcs))
		fn(server) // ⬅️ 关键：这里会把路由添加到server.routes
	}
	log.Infof("路由执行完成，当前路由总数: %d", len(server.routes))
}

// executeAllRouteGroups 执行所有已注册的路由组
func executeAllRouteGroups(server *HTTPServer) {
	groupRegistryMu.Lock()
	entries := make([]GroupRouterEntry, len(groupRegistry))
	copy(entries, groupRegistry)
	groupRegistryMu.Unlock()

	if len(entries) == 0 {
		log.Info("没有注册的路由组需要执行")
		return
	}

	// 按优先级排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority > entries[j].priority
	})

	executed := 0
	for _, entry := range entries {
		if !entry.enabled {
			log.Infof("路由组[%s]已被配置禁用", entry.groupName)
			continue
		}

		// 检查是否已经执行过
		groupExecutedMu.Lock()
		alreadyExecuted := groupExecuted[entry.groupName]
		if !alreadyExecuted {
			groupExecuted[entry.groupName] = true
		}
		groupExecutedMu.Unlock()

		if alreadyExecuted {
			log.Debugf("路由组[%s]已经执行过，跳过", entry.groupName)
			continue
		}

		// 记录当前路由数量，用于统计该组新增的路由
		routesBefore := len(server.routes)

		// 原有前缀/中间件逻辑（完全兼容）
		originPrefix := server.globalPrefix
		server.globalPrefix = entry.prefix
		if server.globalPrefix != "" && server.globalPrefix[len(server.globalPrefix)-1] != '/' {
			server.globalPrefix += "/"
		}
		if len(entry.mws) > 0 {
			server.Use(entry.mws...)
		}

		// 关键：直接调用注册函数，路由会自动添加到 server.routes
		log.Infof("开始执行路由组：%s", entry.groupName)
		entry.register(server)

		// 恢复全局前缀
		server.globalPrefix = originPrefix

		executed++
		routesAdded := len(server.routes) - routesBefore
		log.Infof("已执行路由组：%s，添加 %d 个路由", entry.groupName, routesAdded)
	}

	log.Infof("路由组执行完成，共执行[%d]个路由组，总计 %d 个路由", executed, len(server.routes))
}

func (s *HTTPServer) Prepare() {
	log.Info("开始准备服务器，当前路由数：%d", len(s.routes))
	// 第一步：执行所有已注册的路由组
	ExecuteAllRouteGroups(s)
	// 第二步：如果仍然没有自定义路由，尝试自动发现
	if !s.isRoutesInitialized() {
		log.Info("没有找到自定义路由，开始自动发现...")
		err := s.InitAutoDiscoverRoutes()
		if err != nil {
			log.Warnf("自动发现路由失败，将继续使用已有路由: %v", err)
		}
		// 自动发现后再次执行路由组
		ExecuteAllRouteGroups(s)
	}
	// 第三步：收集和注册所有路由
	var policies []rbac.Policy
	// 1. 收集所有路由的RBAC策略和路由信息
	for _, route := range s.routes {
		policies = append(policies, route.ToRbacPolicy()...)
	}
	// 2. 添加RBAC策略
	if len(policies) > 0 {
		rbacClient.AddActionPolicies(policies)
		log.Infof("已添加 %d 个RBAC策略", len(policies))
	} else {
		log.Warn("没有找到任何路由RBAC策略")
	}
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
	registeredRoutes := 0
	for _, route := range s.routes {
		// 获取该路由的完整处理链（中间件+主处理函数）
		handlers := route.GetHandlersChain()
		// 为每个HTTP方法注册路由
		for _, method := range route.Methods {
			s.router.Handle(method, route.Url, handlers...)
			registeredRoutes++
		}
	}
	log.Infof("服务器准备完成，共注册 %d 个路由，%d 个路由处理器", len(s.routes), registeredRoutes)
}

// Use 注册额外的全局中间件(将在 Prepare 时最先挂载)
func (s *HTTPServer) Use(middlewares ...gin.HandlerFunc) {
	if len(middlewares) == 0 {
		return
	}
	s.extraMiddlewares = append(s.extraMiddlewares, middlewares...)
}

func (s *HTTPServer) StaticFs(static_dir string) {
	s.router.Use(static.Serve("/static", static.LocalFile(static_dir, false)))
}

func (s *HTTPServer) Run(addr string) {
	log.Info("server %v is running", addr)
	s.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	s.router.Run(addr)
}

//package http
//
//import (
//	"context"
//	"github.com/gin-contrib/static"
//	"github.com/gin-gonic/gin"
//	"github.com/goodbye-jack/go-common/approval"
//	//_ "github.com/goodbye-jack/go-common/autoimport"
//	"github.com/goodbye-jack/go-common/config"
//	"github.com/goodbye-jack/go-common/log"
//	"github.com/goodbye-jack/go-common/rbac"
//	"github.com/goodbye-jack/go-common/utils"
//	"github.com/spf13/viper"
//	swaggerFiles "github.com/swaggo/files"
//	ginSwagger "github.com/swaggo/gin-swagger"
//	"net/http"
//	"sync"
//	"time"
//)
//
//// RouteRegisterFunc 业务侧只需实现这个函数类型（替代繁琐的接口）
//type RouteRegisterFunc func(server *HTTPServer)
//
//// GroupRouterEntry 路由组注册项（原simpleRouterEntry，命名更语义化）
//type GroupRouterEntry struct {
//	groupName string            // 路由组名称（配置化用）
//	prefix    string            // 路由组前缀
//	priority  int               // 注册优先级
//	mws       []gin.HandlerFunc // 组专属中间件
//	register  RouteRegisterFunc // 路由注册函数
//	enabled   bool              // 是否启用（默认true）
//}
//
//// 全局路由组注册器（命名同步优化）
//var (
//	rbacClient      *rbac.RbacClient = nil
//	groupRegistry   []GroupRouterEntry
//	groupRegistryMu sync.Mutex
//)
//
//// routeConfig 存储路由组的启用/禁用配置（对应yaml中的routes节点）
//var routeConfig = struct {
//	Routes map[string]RouteConfigItem `yaml:"routes"`
//}{}
//
//// RouteConfigItem 路由组配置项（仅包含启用状态）
//type RouteConfigItem struct {
//	Enabled bool `yaml:"enabled"` // true=启用，false=禁用
//}
//
//type Operation struct {
//	User          string                 `json:"user"`
//	Time          time.Time              `json:"time"`
//	Path          string                 `json:"path"`
//	Method        string                 `json:"method"`
//	ClientIP      string                 `json:"client_ip"`
//	StatusCode    int                    `json:"status_code"`
//	Duration      int                    `json:"duration"`
//	Body          map[string]interface{} `json:"body"`
//	Tips          string                 `json:"tips"`
//	Authorization string                 `json:"authorization"` // 门户使用的token,在单独项目里拿不到，通过这里获取
//}
//
//type OpRecordFn func(ctx context.Context, op Operation) error
//
//type HTTPServer struct {
//	service_name     string
//	routes           []*Route
//	router           *gin.Engine
//	opRecordFn       OpRecordFn
//	approvalHandler  approval.ApprovalHandler
//	extraMiddlewares []gin.HandlerFunc
//	// 新增字段（增量，不影响原有逻辑）
//	globalPrefix string // 路由全局前缀
//}
//
//func init() {
//	rbacClient = rbac.NewRbacClient(
//		config.GetConfigString(utils.CasbinRedisAddrName),
//	)
//	// 设置默认值：如果配置文件中没有routes节点，默认所有路由组启用
//	viper.SetDefault("routes", map[string]RouteConfigItem{})
//	// 从配置文件加载routes配置到全局变量
//	if err := viper.UnmarshalKey("routes", &routeConfig.Routes); err != nil {
//		log.Warnf("加载路由组配置失败，将使用默认配置（所有路由组启用）：%v", err)
//		// 初始化空map，避免后续nil指针错误
//		routeConfig.Routes = make(map[string]RouteConfigItem)
//	}
//}
//
//func NewHTTPServer(service_name string) *HTTPServer {
//	routes := []*Route{
//		NewRoute(service_name, "/ping", "健康检查", []string{"GET"}, utils.RoleIdle, false, false, func(c *gin.Context) {
//			c.String(http.StatusOK, "Pong")
//		}),
//	}
//	return &HTTPServer{
//		service_name:     service_name,
//		routes:           routes,
//		router:           gin.Default(),
//		extraMiddlewares: []gin.HandlerFunc{},
//	}
//}
//
//func (s *HTTPServer) GetRoutes() []*Route {
//	return s.routes
//}
//
//func (s *HTTPServer) GetDefaultRoles() []string {
//	return []string{
//		utils.RoleAdministrator,
//	}
//}
//
//func (s *HTTPServer) Route(path string, methods []string, role string, sso bool, fn gin.HandlerFunc) {
//	if len(methods) == 0 {
//		methods = append(methods, "GET")
//	}
//	s.routes = append(s.routes, NewRoute(s.service_name, path, "", methods, role, sso, false, fn))
//}
//
//// RouteForRA 鉴定专用router,携带日志记录,明确角色
//func (s *HTTPServer) RouteForRA(path string, tips string, methods []string, roles []string, sso bool, fn gin.HandlerFunc) {
//	if len(methods) == 0 {
//		methods = append(methods, "GET")
//	}
//	s.routes = append(s.routes, NewRouteForRA(s.service_name, path, tips, methods, roles, sso, false, fn))
//}
//
//func (s *HTTPServer) RouteAPI(path string, tips string, methods []string, roles []string, sso bool, business_approval bool, fn gin.HandlerFunc) {
//	route := NewRouteCommon(s.service_name, path, tips, methods, roles, sso, business_approval, fn)
//	// 添加业务审批中间件(如果需要)
//	if business_approval && s.approvalHandler != nil {
//		aConfig := approval.Config{
//			BusinessApproval: business_approval,
//			Handler:          s.approvalHandler,
//		}
//		route.AddMiddleware(approval.ApprovalMiddleware(aConfig))
//	}
//	if tips != "" && s.opRecordFn != nil {
//		route.AddMiddleware(RecordOperationMiddleware(s.routes, s.opRecordFn))
//	}
//	s.routes = append(s.routes, route)
//}
//
//// RouteAutoRegisterAPI 业务侧调用的简化版API（自动拼接前缀、内置优先级）
//// 参数：原有RouteAPI参数 + 组前缀（可选）
//func (s *HTTPServer) RouteAutoRegisterAPI(path string, tips string, methods []string, roles []string,
//	sso bool, businessApproval bool, fn gin.HandlerFunc, prefix ...string, // 可选前缀，不传则用全局前缀
//) {
//	// 拼接前缀（优先级：传入前缀 > 全局前缀）
//	finalPrefix := s.globalPrefix
//	if len(prefix) > 0 && prefix[0] != "" {
//		finalPrefix = prefix[0]
//	}
//	// 处理前缀格式（确保以/结尾）
//	if finalPrefix != "" && finalPrefix[len(finalPrefix)-1] != '/' {
//		finalPrefix += "/"
//	}
//	// 最终路径 = 前缀 + 原始路径
//	finalPath := finalPrefix + path
//	// 复用原有RouteAPI逻辑
//	s.RouteAPI(finalPath, tips, methods, roles, sso, businessApproval, fn)
//}
//
//// RegisterGroupRouter 注册路由组（原RegisterSimpleRouter，命名更直观）
//// 参数：路由组名称、前缀、优先级、中间件、注册函数
//func RegisterGroupRouter(groupName, prefix string, priority int, mws []gin.HandlerFunc, register RouteRegisterFunc) {
//	groupRegistryMu.Lock()
//	defer groupRegistryMu.Unlock()
//	enabled := true // 从配置读取是否启用（默认启用）
//	if item, ok := routeConfig.Routes[groupName]; ok {
//		enabled = item.Enabled
//	}
//	groupRegistry = append(groupRegistry, GroupRouterEntry{
//		groupName: groupName,
//		prefix:    prefix,
//		priority:  priority,
//		mws:       mws,
//		register:  register,
//		enabled:   enabled,
//	})
//	log.Infof("已注册路由组：%s（前缀：%s，优先级：%d）", groupName, prefix, priority)
//}
//
//// SetApprovalHandler 设置审批处理器
//func (s *HTTPServer) SetApprovalHandler(handler approval.ApprovalHandler) {
//	s.approvalHandler = handler
//}
//
//func (s *HTTPServer) SetOpRecordFn(fn OpRecordFn) {
//	s.opRecordFn = fn
//}
//
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
//
//func (s *HTTPServer) Prepare() {
//	// 新增：自动发现和注册路由（如果还没初始化）
//	err := s.InitAutoDiscoverRoutes()
//	if err != nil {
//		log.Warnf("自动发现路由失败，将继续使用已有路由: %v", err)
//	}
//	var policies []rbac.Policy
//	// 1. 收集所有路由的RBAC策略和路由信息
//	for _, route := range s.routes {
//		policies = append(policies, route.ToRbacPolicy()...)
//	}
//	// 2. 添加RBAC策略
//	rbacClient.AddActionPolicies(policies)
//	// 3. 设置全局中间件(注意顺序)
//	s.router.SetTrustedProxies([]string{"127.0.0.1", "192.168.0.0/24"})
//	// 4. 全局中间件(作用于所有路由)
//	// 先注册用户自定义额外中间件，再注册内置中间件，确保用户安全中间件可最早生效
//	if len(s.extraMiddlewares) > 0 {
//		s.router.Use(s.extraMiddlewares...)
//	}
//	s.router.Use(
//		LoginRequiredMiddleware(s.routes),                 // 登录检查
//		RbacMiddleware(s.service_name),                    // RBAC鉴权
//		TenantMiddleware(),                                // 租户隔离
//		RecordOperationMiddleware(s.routes, s.opRecordFn), // 操作记录
//	)
//	// 5. 直接注册路由（不再使用routeInfos）
//	for _, route := range s.routes {
//		// 获取该路由的完整处理链（中间件+主处理函数）
//		handlers := route.GetHandlersChain()
//		// 为每个HTTP方法注册路由
//		for _, method := range route.Methods {
//			s.router.Handle(method, route.Url, handlers...)
//		}
//	}
//}
//
//// Use 注册额外的全局中间件(将在 Prepare 时最先挂载)
//func (s *HTTPServer) Use(middlewares ...gin.HandlerFunc) {
//	if len(middlewares) == 0 {
//		return
//	}
//	s.extraMiddlewares = append(s.extraMiddlewares, middlewares...)
//}
//
//func (s *HTTPServer) StaticFs(static_dir string) {
//	//s.router.Use(static.Serve("/static", static.LocalFile(static_dir, true)))
//	// 禁用/static访问,必须要有参数才能访问
//	s.router.Use(static.Serve("/static", static.LocalFile(static_dir, false)))
//}
//
//func (s *HTTPServer) Run(addr string) {
//	log.Info("server %v is running", addr)
//	s.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
//	s.router.Run(addr)
//}
