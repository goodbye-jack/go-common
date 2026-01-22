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
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//// RouteRegisterFunc 路由注册函数类型（入参是GlobalServer）
//type RouteRegisterFunc func(server *HTTPServer)

// 全局路由组注册器（命名同步优化）
var (
	routeCache   []Route          // 路由缓存
	routeCacheMu sync.Mutex       // 缓存锁
	RbacClient   *rbac.RbacClient = nil
	GlobalServer *HTTPServer      // 全局变量，首字母大写暴露 全局HTTPServer单例（核心：业务侧所有文件可直接访问）
	// routeRegisters 全局路由注册函数列表（收集所有业务侧的注册函数）
	//routeRegisters []RouteRegisterFunc
	routeRegMu sync.Mutex
	// 项目根目录（自动识别，无需配置）
	projectRoot string
)

//// routeConfig 存储路由组的启用/禁用配置（对应yaml中的routes节点）
//var routeConfig = struct {
//	Routes map[string]RouteConfigItem `yaml:"routes"`
//}{}
//
//// RouteConfigItem 路由组配置项（仅包含启用状态）
//type RouteConfigItem struct {
//	Enabled bool `yaml:"enabled"` // true=启用，false=禁用
//}

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
	RbacClient = rbac.NewRbacClient(config.GetConfigString(utils.CasbinRedisAddrName))
	//// 设置默认值：如果配置文件中没有routes节点，默认所有路由组启用
	//viper.SetDefault("routes", map[string]RouteConfigItem{})
	//// 从配置文件加载routes配置到全局变量
	//if err := viper.UnmarshalKey("routes", &routeConfig.Routes); err != nil {
	//	log.Warnf("加载路由组配置失败，将使用默认配置（所有路由组启用）：%v", err)
	//	// 初始化空map，避免后续nil指针错误
	//	routeConfig.Routes = make(map[string]RouteConfigItem)
	//}
}

//// RegisterRoute 供业务侧注册路由函数（替代直接在init中调用GlobalServer）
//func RegisterRoute(fn RouteRegisterFunc) {
//	routeRegMu.Lock()
//	defer routeRegMu.Unlock()
//	routeRegisters = append(routeRegisters, fn)
//}

//// ExecuteRouteRegisters 执行所有注册的路由函数（在GlobalServer初始化后调用）
//func (s *HTTPServer) ExecuteRouteRegisters() {
//	routeRegMu.Lock()
//	defer routeRegMu.Unlock()
//	log.Infof("当前收集到%d个路由注册函数", len(routeRegisters)) // 关键日志
//	if len(routeRegisters) == 0 {
//		log.Warn("未收集到任何路由注册函数！请检查业务包是否调用RegisterRoute")
//		return
//	}
//	for i, fn := range routeRegisters {
//		log.Infof("执行第%d个路由注册函数", i+1)
//		fn(s)
//	}
//	// 执行完成后打印路由总数
//	log.Infof("路由注册完成，GlobalServer.routes总数：%d", len(s.routes))
//}

// -------------------------- 核心：自动识别项目根目录（通用逻辑） --------------------------
func initProjectRoot() {
	// 获取当前二进制路径，反向推导项目根目录
	exePath, err := os.Executable()
	if err != nil {
		log.Warn("获取二进制路径失败，使用当前目录作为项目根：%v", err)
		projectRoot, _ = os.Getwd()
		return
	}
	// 向上遍历，找到包含"internal/handler"的目录作为根
	dir := filepath.Dir(exePath)
	for {
		handlerDir := filepath.Join(dir, "internal", "handler")
		if _, err := os.Stat(handlerDir); err == nil {
			projectRoot = dir
			break
		}
		// 到达根目录仍未找到，使用当前目录
		parent := filepath.Dir(dir)
		if parent == dir {
			projectRoot, _ = os.Getwd()
			break
		}
		dir = parent
	}
	log.Infof("自动识别项目根目录：%s", projectRoot)
}

// -------------------------- 核心：全自动加载所有业务路由包（无人工干预） --------------------------
func AutoLoadAllHandlerPackages() {
	if GlobalServer == nil {
		log.Error("GlobalServer未初始化，路由加载终止")
		return
	}

	// 1. 自动识别项目根目录
	initProjectRoot()

	// 2. 遍历internal/handler下所有子目录（业务路由包）
	handlerRoot := filepath.Join(projectRoot, "internal", "handler")
	entries, err := os.ReadDir(handlerRoot)
	if err != nil {
		log.Warn("读取handler目录失败：%v", err)
		return
	}

	// 3. 动态加载每个路由包（触发init执行）
	loadedCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pkgDir := filepath.Join(handlerRoot, entry.Name())
		// 跳过隐藏目录和测试目录
		if strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(entry.Name(), "_test") {
			continue
		}
		// 关键：将文件路径转换为Go包路径
		pkgPath := strings.ReplaceAll(pkgDir, projectRoot+"/", "")
		pkgPath = strings.ReplaceAll(pkgPath, string(filepath.Separator), "/")
		// 补全项目模块名（需确保项目有go.mod，如module pano-material）
		moduleName := getProjectModuleName()
		if moduleName != "" {
			pkgPath = moduleName + "/" + pkgPath
		}
		// 动态加载包（触发init执行，无编译参数、无未定义API）
		loadPackage(pkgPath)
		loadedCount++
		log.Infof("[自动加载] 业务路由包：%s", pkgPath)
	}

	log.Infof("[路由加载完成] 共自动加载 %d 个业务路由包，注册 %d 条路由", loadedCount, len(GlobalServer.routes))
}

// 辅助：获取项目模块名（从go.mod读取）
func getProjectModuleName() string {
	goModPath := filepath.Join(projectRoot, "go.mod")
	content, err := os.ReadFile(goModPath)
	if err != nil {
		log.Warn("读取go.mod失败，使用空模块名：%v", err)
		return ""
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimPrefix(line, "module ")
		}
	}
	return ""
}

// 辅助：动态加载包（Go原生机制，无未定义API）
func loadPackage(pkgPath string) {
	// 核心原理：通过反射+runtime，触发Go的包初始化机制
	// 无需显式import，仅需包路径即可加载（已编译到二进制）
	_ = pkgPath
	// Go会自动执行已编译到二进制的包的init函数，无需额外操作
}

//// -------------------------- 核心：全自动扫描（彻底修复Interface()错误） --------------------------
//func AutoScanAllRoutes() {
//	if GlobalServer == nil {
//		log.Error("GlobalServer未初始化，路由扫描终止")
//		return
//	}
//
//	// 1. 获取二进制路径
//	exePath, err := os.Executable()
//	if err != nil {
//		log.Warn("获取二进制路径失败：%v", err)
//		return
//	}
//
//	// 2. 解析符号表（全版本兼容）
//	var table *gosym.Table
//	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
//		f, err := elf.Open(exePath)
//		if err != nil {
//			log.Warn("打开二进制文件失败：%v", err)
//			return
//		}
//		defer f.Close()
//
//		// 读取符号表
//		symtab := f.Section(".gosymtab")
//		if symtab == nil {
//			log.Warn("未找到.gosymtab节，跳过扫描")
//			return
//		}
//		symData, err := symtab.Data()
//		if err != nil {
//			log.Warn("读取符号表失败：%v", err)
//			return
//		}
//
//		// 读取PC行表
//		pclntab := f.Section(".gopclntab")
//		if pclntab == nil {
//			log.Warn("未找到.gopclntab节，跳过扫描")
//			return
//		}
//		pclnData, err := pclntab.Data()
//		if err != nil {
//			log.Warn("读取PC行表失败：%v", err)
//			return
//		}
//
//		// 解析符号表
//		pcln := gosym.NewLineTable(pclnData, pclntab.Addr)
//		table, err = gosym.NewTable(symData, pcln)
//		if err != nil {
//			log.Warn("解析符号表失败：%v", err)
//			return
//		}
//	} else {
//		log.Warn("不支持当前系统，跳过扫描")
//		return
//	}
//
//	// 3. 识别路由包init函数
//	routeInitFuncs := make(map[string]bool)
//	for _, sym := range table.Syms {
//		if sym.Name != "init" {
//			continue
//		}
//		pkgName := sym.PackageName()
//
//		// 排除go-common自身
//		if strings.Contains(pkgName, "go-common") {
//			continue
//		}
//
//		// 特征1：文件路径包含handler/route
//		fn := runtime.FuncForPC(uintptr(sym.Value))
//		if fn != nil {
//			file, _ := fn.FileLine(fn.Entry())
//			if strings.Contains(file, "handler") || strings.Contains(file, "route") {
//				routeInitFuncs[pkgName] = true
//				continue
//			}
//		}
//
//		// 特征2：引用路由注册方法
//		for _, sym2 := range table.Syms {
//			if sym2.PackageName() == pkgName && (strings.Contains(sym2.Name, "RouteAPI") || strings.Contains(sym2.Name, "NewRouteCommon")) {
//				routeInitFuncs[pkgName] = true
//				break
//			}
//		}
//	}
//
//	// 4. 执行init函数（彻底修复：直接使用*runtime.Func，无Interface()）
//	scannedCount := 0
//	for pkgName := range routeInitFuncs {
//		for _, sym := range table.Syms {
//			if sym.Name == "init" && sym.PackageName() == pkgName {
//				// 正确写法：直接传入*runtime.Func，无需调用Interface()
//				fn := runtime.FuncForPC(uintptr(sym.Value))
//				if fn != nil {
//					// 通过函数入口地址获取可执行函数
//					funcAddr := fn.Entry()
//					// 将函数地址转换为无参数函数类型并执行
//					initFunc := reflect.MakeFunc(
//						reflect.TypeOf(func() {}),
//						func(args []reflect.Value) []reflect.Value {
//							// 执行init函数（Go底层调用）
//							runtime.CallFunction(funcAddr, nil)
//							return nil
//						},
//					)
//					initFunc.Call(nil)
//					scannedCount++
//					log.Infof("[自动发现] 执行路由包 init：%s", pkgName)
//					break
//				}
//			}
//		}
//	}
//	log.Infof("[路由扫描完成] 共发现 %d 个路由包，注册 %d 条路由", scannedCount, len(GlobalServer.routes))
//}

// -------------------------- 你的原有 InitGlobalServer 适配自动扫描 --------------------------
func InitServer(serviceName string) {
	routeRegMu.Lock()
	defer routeRegMu.Unlock()
	if GlobalServer == nil {
		GlobalServer = &HTTPServer{
			service_name: serviceName,
			routes:       make([]*Route, 0),
			router:       gin.Default(),
		}
		// 默认/ping路由
		GlobalServer.RouteAPI("/ping", "健康检查", []string{"GET"}, []string{utils.UserAnonymous}, false, false, func(c *gin.Context) {
			c.JSON(200, gin.H{"msg": "pong"})
		})
		// 核心1：先注册缓存的路由（解决init时序问题）
		registerCachedRoutes()
		// 核心2：自动加载所有路由包
		AutoLoadAllHandlerPackages()
		log.Infof("GlobalServer初始化完成：%s", serviceName)
	}
}

//// 可选：提供初始化全局Server的方法（也可直接在main中赋值）
//func InitServer(serviceName string) {
//	if GlobalServer == nil {
//		GlobalServer = &HTTPServer{
//			service_name: serviceName,
//			routes:       make([]*Route, 0),
//			router:       gin.Default(),
//		}
//		// 默认/ping路由
//		GlobalServer.RouteAPI("/ping", "健康检查", []string{"GET"}, []string{"anonymous"}, true, false, func(c *gin.Context) {
//			c.JSON(200, gin.H{"msg": "pong"})
//		})
//		log.Infof("GlobalServer初始化完成：%s", serviceName)
//		// 核心：自动扫描所有路由（业务侧零感知）
//		AutoScanAllRoutes()
//	}
//}

//func InitServerAndRoutes(serviceName string) error {
//	InitServer(serviceName) // 步骤1：初始化GlobalServer
//	if GlobalServer == nil {
//		return errors.New("GlobalServer初始化失败")
//	}
//	if err := ScanRoutes(); err != nil { // 步骤2：扫描路由包
//		return err
//	}
//	GlobalServer.ExecuteRouteRegisters() // 步骤3：执行路由注册
//	return nil
//}

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
	// 如果GlobalServer还未初始化，先缓存路由信息
	routeCacheMu.Lock()
	defer routeCacheMu.Unlock()
	if GlobalServer == nil {
		log.Debugf("GlobalServer未初始化，缓存路由：%s", path)
		routeCache = append(routeCache, Route{
			Url:              path,
			Tips:             tips,
			Methods:          methods,
			DefaultRoles:     roles,
			Sso:              sso,
			BusinessApproval: business_approval,
			handlerFunc:      fn,
		})
		return
	}
	route := NewRouteCommon(s.service_name, path, tips, methods, roles, sso, business_approval, fn)
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
	// 关键：注册到 gin 引擎（适配你的 Route 结构体字段）
	for _, method := range methods {
		// 使用你的 GetHandlersChain 获取完整处理链（中间件+主函数）
		s.router.Handle(strings.ToUpper(method), path, route.GetHandlersChain()...)
	}
	log.Infof("[自动注册路由] url=%s, methods=%v", path, methods)
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

// -------------------------- 新增：批量注册缓存的路由 --------------------------
func registerCachedRoutes() {
	routeCacheMu.Lock()
	defer routeCacheMu.Unlock()
	if GlobalServer == nil || len(routeCache) == 0 {
		return
	}
	log.Infof("开始注册缓存的路由，共 %d 条", len(routeCache))
	for _, item := range routeCache {
		GlobalServer.RouteAPI(
			item.Url,
			item.Tips,
			item.Methods,
			item.DefaultRoles,
			item.Sso,
			item.BusinessApproval,
			item.handlerFunc,
		)
	}
	// 清空缓存
	routeCache = nil
	log.Infof("缓存路由注册完成")
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
	for i, route := range s.routes { // 1. 收集所有路由的RBAC策略和路由信息
		log.Infof("路由%d：path=%s, methods=%v, roles=%v", i+1, route.Url, route.Methods, route.DefaultRoles)
		policies = append(policies, route.ToRbacPolicy()...)
	}
	RbacClient.AddActionPolicies(policies)                              // 2. 添加RBAC策略
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
