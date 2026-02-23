package http

import (
	"context"
	"fmt"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/approval"
	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/rbac"
	"github.com/goodbye-jack/go-common/utils"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 全局路由组注册器（命名同步优化）
var (
	RbacClient     *rbac.RbacClient = nil
	GlobalServer   *HTTPServer      // 全局变量，首字母大写暴露 全局HTTPServer单例（核心：业务侧所有文件可直接访问）
	routeRegMu     sync.Mutex
	routeMetaCache = struct {
		sync.RWMutex
		metas []Route
		keys  map[string]bool // 预收集阶段去重（key: URL-Method）
	}{
		keys: make(map[string]bool),
	}
)

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
	globalPrefix     string          // 路由全局前缀 新增字段（增量，不影响原有逻辑）
	registeredKeys   map[string]bool // 已注册路由唯一键（URL-Method）
}

func init() {
	RbacClient = rbac.NewRbacClient(config.GetConfigString(utils.CasbinRedisAddrName))
}

// RegisterCollectedRoutes Server就绪后调用，批量注册预收集的路由
func (s *HTTPServer) RegisterCollectedRoutes() {
	routeMetaCache.RLock()
	defer routeMetaCache.RUnlock()
	if len(routeMetaCache.metas) == 0 {
		log.Info("无预收集的路由信息，跳过注册")
		return
	}
	log.Infof("开始注册预收集的路由，共 %d 条", len(routeMetaCache.metas))
	successCount := 0
	skipCount := 0
	for _, meta := range routeMetaCache.metas {
		// 遍历HTTP方法，逐个校验
		for _, method := range meta.Methods {
			if s.IsRouteRegistered(meta.Url, method) {
				log.Warnf("[批量注册] 重复路由，跳过：URL=%s, Method=%s", meta.Url, method)
				skipCount++
				continue
			}
			// 调用RouteAPI注册（内部已二次校验）
			s.RouteAPI(meta.Url, meta.Tips, []string{method}, meta.DefaultRoles, meta.Resource, meta.Action, meta.Sso, meta.BusinessApproval, meta.handlerFunc)
			successCount++
		}
	}
	log.Infof("[预收集路由注册完成] 成功注册：%d 条，跳过重复：%d 条", successCount, skipCount)
}

// -------------------------- 你的原有 InitGlobalServer 适配自动扫描 --------------------------
func InitServer(serviceName string) {
	routeRegMu.Lock()
	defer routeRegMu.Unlock()
	if GlobalServer == nil {
		GlobalServer = NewHTTPServer(serviceName)
		// 默认/ping路由
		GlobalServer.RouteAPI("/ping", "健康检查", []string{"GET"}, []string{utils.UserAnonymous}, "", "", false, false, func(c *gin.Context) {
			c.JSON(200, gin.H{"msg": "pong"})
		})
		GlobalServer.RegisterCollectedRoutes()
		log.Infof("GlobalServer初始化完成，服务名：%s，已注册路由总数：%d", serviceName, len(GlobalServer.routes))
	}
}

func NewHTTPServer(service_name string) *HTTPServer {
	routes := []*Route{
		NewRoute(service_name, "/ping", "健康检查", []string{"GET"}, utils.RoleIdle, "", "", false, false, func(c *gin.Context) {
			c.String(http.StatusOK, "Pong")
		}),
	}
	return &HTTPServer{
		service_name:     service_name,
		routes:           routes,
		router:           gin.Default(),
		extraMiddlewares: []gin.HandlerFunc{},
		registeredKeys:   make(map[string]bool), // 初始化已注册路由缓存
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

// IsRouteRegistered 校验路由是否已注册（URL+Method）
func (s *HTTPServer) IsRouteRegistered(url, method string) bool {
	if s.registeredKeys == nil {
		s.registeredKeys = make(map[string]bool)
	}
	key := fmt.Sprintf("%s-%s", strings.TrimSuffix(url, "/"), strings.ToUpper(method))
	return s.registeredKeys[key]
}

// MarkRouteRegistered 标记路由为已注册
func (s *HTTPServer) MarkRouteRegistered(url, method string) {
	if s.registeredKeys == nil {
		s.registeredKeys = make(map[string]bool)
	}
	key := fmt.Sprintf("%s-%s", strings.TrimSuffix(url, "/"), strings.ToUpper(method))
	s.registeredKeys[key] = true
}

// PreloadRouteAPI 业务侧init中调用，仅收集路由元信息，不注册（无任何依赖，可在init中安全调用）
func PreloadRouteAPI(url, tips string, methods []string, defaultRoles []string, resource string, action string, sso bool, businessApproval bool, handlerFunc gin.HandlerFunc) {
	routeMetaCache.Lock()
	defer routeMetaCache.Unlock()
	for _, method := range methods { // 遍历HTTP方法，生成唯一键去重
		key := fmt.Sprintf("%s-%s", strings.TrimSuffix(url, "/"), strings.ToUpper(method))
		if routeMetaCache.keys[key] {
			log.Warnf("[路由预收集] 重复路由，跳过：%s（URL:%s, Method:%s）", key, url, method)
			return
		}
		routeMetaCache.keys[key] = true
	}
	routeMetaCache.metas = append(routeMetaCache.metas, Route{ // 无重复则收集元信息
		Url:              url,
		Tips:             tips,
		Methods:          methods,
		DefaultRoles:     defaultRoles,
		Resource:         resource,
		Action:           action,
		Sso:              sso,
		BusinessApproval: businessApproval,
		handlerFunc:      handlerFunc,
	})
	log.Debugf("[路由预收集] 已收集路由：%s %s", methods, url)
}

func (s *HTTPServer) Route(path string, methods []string, role string, resource string, action string, sso bool, fn gin.HandlerFunc) {
	if len(methods) == 0 {
		methods = append(methods, "GET")
	}
	s.routes = append(s.routes, NewRoute(s.service_name, path, "", methods, role, resource, action, sso, false, fn))
}

// RouteForRA 鉴定专用router,携带日志记录,明确角色
func (s *HTTPServer) RouteForRA(path string, tips string, methods []string, roles []string, resource string, action string, sso bool, fn gin.HandlerFunc) {
	if len(methods) == 0 {
		methods = append(methods, "GET")
	}
	s.routes = append(s.routes, NewRouteForRA(s.service_name, path, tips, methods, roles, resource, action, sso, false, fn))
}

func (s *HTTPServer) RouteAPI(path string, tips string, methods []string, roles []string, resource string, action string, sso bool, business_approval bool, fn gin.HandlerFunc) {
	route := NewRouteCommon(s.service_name, path, tips, methods, roles, resource, action, sso, business_approval, fn)
	if business_approval && s.approvalHandler != nil { // 添加业务审批中间件(如果需要)
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
	//// 关键：注册到 gin 引擎（适配你的 Route 结构体字段）
	//for _, method := range methods {
	//	method = strings.ToUpper(method)
	//	if s.IsRouteRegistered(path, method) {
	//		log.Warnf("[路由注册] 重复路由，跳过：URL=%s, Method=%s", path, method)
	//		continue // 跳过当前方法，继续处理其他方法
	//	}
	//	// 使用你的 GetHandlersChain 获取完整处理链（中间件+主函数）
	//	s.router.Handle(strings.ToUpper(method), path, route.GetHandlersChain()...)
	//	// 标记为已注册
	//	s.MarkRouteRegistered(path, method)
	//}
	//log.Infof("[自动注册路由] url=%s, methods=%v", path, methods)
}

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

// SetApprovalHandler 设置审批处理器
func (s *HTTPServer) SetApprovalHandler(handler approval.ApprovalHandler) {
	s.approvalHandler = handler
}

func (s *HTTPServer) SetOpRecordFn(fn OpRecordFn) {
	s.opRecordFn = fn
}

func (s *HTTPServer) Prepare() {
	var policies []rbac.Policy
	for i, route := range s.routes { // 1. 收集所有路由的RBAC策略和路由信息
		log.Infof("路由%d：path=%s, methods=%v, roles=%v", i+1, route.Url, route.Methods, route.DefaultRoles)
		policies = append(policies, route.ToRbacPolicy()...)
	}
	_ = RbacClient.DeletePoliciesByService(s.service_name)               // 2. 清理旧策略
	RbacClient.AddActionPolicies(policies)                               // 3. 添加RBAC策略
	s.router.SetTrustedProxies([]string{"127.0.0.1", "192.168.0.0/24"}) // 3. 设置全局中间件(注意顺序)
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
		handlers := route.GetHandlersChain()   // 获取该路由的完整处理链（中间件+主处理函数）
		for _, method := range route.Methods { // 为每个HTTP方法注册路由
			method = strings.ToUpper(method)
			if s.IsRouteRegistered(route.Url, method) {
				log.Warnf("[路由注册] 重复路由，跳过：URL=%s, Method=%s", route.Url, method)
				continue // 跳过当前方法，继续处理其他方法
			}
			// 使用你的 GetHandlersChain 获取完整处理链（中间件+主函数）
			s.router.Handle(method, route.Url, handlers...)
			// 标记为已注册
			s.MarkRouteRegistered(route.Url, method)
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
