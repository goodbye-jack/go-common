package http

import (
	"bytes"
	"context"
	"github.com/goodbye-jack/go-common/log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// -------------------------- 全局缓存（提速+避免重复扫描） --------------------------
var (
	cachedPkgPaths []string
	cachedRootDir  string
	cacheOnce      sync.Once
	rootDirOnce    sync.Once
	// 新增：存储自动扫描的路由组，避免重复注册
	discoveredGroups = make(map[string]bool)
	groupMu          sync.Mutex
)

// -------------------------- 核心：自动扫描+注册（零脚本） --------------------------
// InitAutoDiscoverRoutes 一键初始化（兼容原有逻辑+自动扫描，零脚本）
func (s *HTTPServer) InitAutoDiscoverRoutes() error {
	// 1. 自动扫描路由包（缓存结果，仅扫描一次）
	pkgPaths := autoDiscoverHandlerPackages()
	if len(pkgPaths) > 0 {
		// 2. 自动注册扫描到的路由组（复用原有RegisterGroupRouter）
		autoRegisterGroups(pkgPaths)
	}
	// 3. 复用原有路由组初始化逻辑（完全兼容）
	groupRegistryMu.Lock()
	entries := make([]GroupRouterEntry, len(groupRegistry))
	copy(entries, groupRegistry)
	groupRegistryMu.Unlock()
	// 按优先级排序（原有逻辑）
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority > entries[j].priority
	})
	registered := 0
	for _, entry := range entries {
		if !entry.enabled {
			log.Infof("路由组[%s]已被配置禁用", entry.groupName)
			continue
		}
		// 记录当前路由数量，用于统计该组新增的路由
		routesBefore := len(s.routes)
		// 原有前缀/中间件逻辑（完全兼容）
		originPrefix := s.globalPrefix
		s.globalPrefix = entry.prefix
		if s.globalPrefix != "" && s.globalPrefix[len(s.globalPrefix)-1] != '/' {
			s.globalPrefix += "/"
		}
		if len(entry.mws) > 0 {
			s.Use(entry.mws...)
		}
		// 关键：直接调用注册函数，路由会自动添加到 s.routes
		entry.register(s)
		// 恢复全局前缀
		s.globalPrefix = originPrefix
		registered++
		routesAdded := len(s.routes) - routesBefore
		log.Infof("已初始化路由组：%s，添加 %d 个路由", entry.groupName, routesAdded)
	}
	log.Infof("路由组自动初始化完成，共注册[%d]个路由组，总计 %d 个路由", registered, len(s.routes))
	return nil
}

// -------------------------- 内部扫描逻辑（零脚本核心） --------------------------
// autoDiscoverHandlerPackages 自动扫描（无临时文件、无脚本）
func autoDiscoverHandlerPackages() []string {
	cacheOnce.Do(func() {
		// 自动获取项目根目录（缓存）
		getRootDir()
		if cachedRootDir == "" {
			log.Warn("未找到项目根目录（go.mod），跳过路由包自动扫描")
			return
		}
		// 执行go list（简化命令，提速）
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// 核心：仅扫描包路径，无多余输出
		cmd := exec.CommandContext(ctx, "go", "list", "-f", "{{.ImportPath}}", "./internal/handler/...")
		cmd.Dir = cachedRootDir
		cmd.Env = append(os.Environ(), "GOGC=off") // 禁用GC提速

		var outBuf bytes.Buffer
		cmd.Stdout = &outBuf

		if err := cmd.Run(); err != nil {
			log.Warnf("扫描路由包失败（非致命，仅自动扫描失效）：%v", err)
			return
		}
		// 解析结果（过滤有效路由包）
		lines := strings.Fields(outBuf.String())
		for _, line := range lines {
			if strings.Contains(line, "/internal/handler/") && !strings.HasSuffix(line, "/...") {
				cachedPkgPaths = append(cachedPkgPaths, line)
			}
		}
		log.Infof("自动扫描到 %d 个路由包：%v", len(cachedPkgPaths), cachedPkgPaths)
	})
	return cachedPkgPaths
}

// autoRegisterGroups 自动注册路由组（复用原有逻辑，零脚本）
func autoRegisterGroups(pkgPaths []string) {
	groupMu.Lock()
	defer groupMu.Unlock()
	for _, pkg := range pkgPaths {
		groupName := filepath.Base(pkg)
		if discoveredGroups[groupName] {
			continue // 避免重复注册
		}
		// 复用原有RegisterGroupRouter（核心：触发路由包init()）
		RegisterGroupRouter(
			groupName,
			"/"+groupName, // 前缀默认/组名
			10,            // 默认优先级
			nil,           // 无专属中间件
			func(server *HTTPServer) {
				// 空函数：路由包init()中已自行注册，此处仅触发执行
			},
		)
		discoveredGroups[groupName] = true
	}
}

// getRootDir 自动获取项目根目录（缓存，仅执行一次）
func getRootDir() {
	rootDirOnce.Do(func() {
		wd, err := os.Getwd()
		if err != nil {
			log.Warnf("获取当前目录失败：%v", err)
			return
		}
		// 向上查找go.mod
		for {
			modPath := filepath.Join(wd, "go.mod")
			if _, err := os.Stat(modPath); err == nil {
				cachedRootDir = wd
				break
			}
			parent := filepath.Dir(wd)
			if parent == wd {
				break // 到达根目录
			}
			wd = parent
		}
	})
}

//package http
//
//import (
//	"bytes"
//	"context"
//	"github.com/goodbye-jack/go-common/log"
//	"os"
//	"os/exec"
//	"path/filepath"
//	"sort"
//	"strings"
//	"sync"
//	"time"
//)
//
//// -------------------------- 全局缓存（提速+避免重复扫描） --------------------------
//var (
//	cachedPkgPaths []string
//	cachedRootDir  string
//	cacheOnce      sync.Once
//	rootDirOnce    sync.Once
//	// 新增：存储自动扫描的路由组，避免重复注册
//	discoveredGroups = make(map[string]bool)
//	groupMu          sync.Mutex
//)
//
//// -------------------------- 核心：自动扫描+注册（零脚本） --------------------------
//// InitAutoDiscoverRoutes 一键初始化（兼容原有逻辑+自动扫描，零脚本）
//func (s *HTTPServer) InitAutoDiscoverRoutes() error {
//	// 1. 自动扫描路由包（缓存结果，仅扫描一次）
//	pkgPaths := autoDiscoverHandlerPackages()
//	if len(pkgPaths) > 0 {
//		// 2. 自动注册扫描到的路由组（复用原有RegisterGroupRouter）
//		autoRegisterGroups(pkgPaths)
//	}
//
//	// 3. 复用原有路由组初始化逻辑（完全兼容）
//	groupRegistryMu.Lock()
//	entries := make([]GroupRouterEntry, len(groupRegistry))
//	copy(entries, groupRegistry)
//	groupRegistryMu.Unlock()
//	// 按优先级排序（原有逻辑）
//	sort.Slice(entries, func(i, j int) bool {
//		return entries[i].priority > entries[j].priority
//	})
//	registered := 0
//	for _, entry := range entries {
//		if !entry.enabled {
//			log.Infof("路由组[%s]已被配置禁用", entry.groupName)
//			continue
//		}
//		// 原有前缀/中间件逻辑（完全兼容）
//		originPrefix := s.globalPrefix
//		s.globalPrefix = entry.prefix
//		if s.globalPrefix != "" && s.globalPrefix[len(s.globalPrefix)-1] != '/' {
//			s.globalPrefix += "/"
//		}
//		if len(entry.mws) > 0 {
//			s.Use(entry.mws...)
//		}
//		// 关键：直接触发路由包init()（无需脚本/临时文件）
//		// 原理：Go编译期会解析所有依赖，只要RegisterGroupRouter被调用，路由包init()就会执行
//		entry.register(s)
//		s.globalPrefix = originPrefix
//		registered++
//		log.Infof("已初始化路由组：%s", entry.groupName)
//	}
//	log.Infof("路由组自动初始化完成，共注册[%d]个路由组", registered)
//	return nil
//}
//
//// -------------------------- 内部扫描逻辑（零脚本核心） --------------------------
//// autoDiscoverHandlerPackages 自动扫描（无临时文件、无脚本）
//func autoDiscoverHandlerPackages() []string {
//	cacheOnce.Do(func() {
//		// 自动获取项目根目录（缓存）
//		getRootDir()
//		if cachedRootDir == "" {
//			log.Warn("未找到项目根目录（go.mod），跳过路由包自动扫描")
//			return
//		}
//		// 执行go list（简化命令，提速）
//		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
//		defer cancel()
//
//		// 核心：仅扫描包路径，无多余输出
//		cmd := exec.CommandContext(ctx, "go", "list", "-f", "{{.ImportPath}}", "./internal/handler/...")
//		cmd.Dir = cachedRootDir
//		cmd.Env = append(os.Environ(), "GOGC=off") // 禁用GC提速
//
//		var outBuf bytes.Buffer
//		cmd.Stdout = &outBuf
//
//		if err := cmd.Run(); err != nil {
//			log.Warnf("扫描路由包失败（非致命，仅自动扫描失效）：%v", err)
//			return
//		}
//		// 解析结果（过滤有效路由包）
//		lines := strings.Fields(outBuf.String())
//		for _, line := range lines {
//			if strings.Contains(line, "/internal/handler/") && !strings.HasSuffix(line, "/...") {
//				cachedPkgPaths = append(cachedPkgPaths, line)
//			}
//		}
//		log.Infof("自动扫描到 %d 个路由包：%v", len(cachedPkgPaths), cachedPkgPaths)
//	})
//	return cachedPkgPaths
//}
//
//// autoRegisterGroups 自动注册路由组（复用原有逻辑，零脚本）
//func autoRegisterGroups(pkgPaths []string) {
//	groupMu.Lock()
//	defer groupMu.Unlock()
//
//	for _, pkg := range pkgPaths {
//		groupName := filepath.Base(pkg)
//		if discoveredGroups[groupName] {
//			continue // 避免重复注册
//		}
//
//		// 复用原有RegisterGroupRouter（核心：触发路由包init()）
//		RegisterGroupRouter(
//			groupName,
//			"/"+groupName, // 前缀默认/组名
//			10,            // 默认优先级
//			nil,           // 无专属中间件
//			func(server *HTTPServer) {
//				// 空函数：路由包init()中已自行注册，此处仅触发执行
//			},
//		)
//
//		discoveredGroups[groupName] = true
//	}
//}
//
//// getRootDir 自动获取项目根目录（缓存，仅执行一次）
//func getRootDir() {
//	rootDirOnce.Do(func() {
//		wd, err := os.Getwd()
//		if err != nil {
//			log.Warnf("获取当前目录失败：%v", err)
//			return
//		}
//		// 向上查找go.mod
//		for {
//			modPath := filepath.Join(wd, "go.mod")
//			if _, err := os.Stat(modPath); err == nil {
//				cachedRootDir = wd
//				break
//			}
//
//			parent := filepath.Dir(wd)
//			if parent == wd {
//				break // 到达根目录
//			}
//			wd = parent
//		}
//	})
//}
