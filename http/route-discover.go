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

// -------------------------- 全局缓存 --------------------------
var (
	cachedPkgPaths []string
	cachedRootDir  string
	cacheOnce      sync.Once
	rootDirOnce    sync.Once
	// 新增：存储自动扫描的路由组，避免重复注册
	discoveredGroups = make(map[string]bool)
	groupMu          sync.Mutex
)

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
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// 扫描所有包含路由的包
		cmd := exec.CommandContext(ctx, "go", "list", "-f", "{{.ImportPath}}", "./...")
		cmd.Dir = cachedRootDir
		cmd.Env = append(os.Environ(), "GOGC=off") // 禁用GC提速

		var outBuf bytes.Buffer
		cmd.Stdout = &outBuf

		if err := cmd.Run(); err != nil {
			log.Warnf("扫描项目包失败：%v", err)
			return
		}

		// 解析结果，过滤出可能的handler包
		lines := strings.Fields(outBuf.String())
		for _, line := range lines {
			// 过滤掉vendor目录和测试包
			if strings.Contains(line, "/vendor/") ||
				strings.Contains(line, "_test") ||
				strings.HasSuffix(line, ".test") {
				continue
			}

			// 检查包是否包含路由相关代码
			if isPossibleRoutePackage(line) {
				cachedPkgPaths = append(cachedPkgPaths, line)
			}
		}

		log.Infof("自动扫描到 %d 个可能的路由包：%v", len(cachedPkgPaths), cachedPkgPaths)
	})
	return cachedPkgPaths
}

// isPossibleRoutePackage 检查包是否可能是路由包
func isPossibleRoutePackage(pkgPath string) bool {
	// 常见路由包命名模式
	routePatterns := []string{
		"handler",
		"handlers",
		"handle",
		"routes",
		"api",
		"controller",
	}

	pkgName := filepath.Base(pkgPath)
	for _, pattern := range routePatterns {
		if strings.Contains(strings.ToLower(pkgName), pattern) {
			return true
		}
	}

	// 检查路径中是否包含常见模式
	for _, pattern := range routePatterns {
		if strings.Contains(strings.ToLower(pkgPath), "/"+pattern+"/") {
			return true
		}
	}

	return false
}

// autoRegisterGroups 自动注册路由组
func autoRegisterGroups(pkgPaths []string) {
	groupMu.Lock()
	defer groupMu.Unlock()

	for _, pkg := range pkgPaths {
		groupName := extractGroupName(pkg)
		if groupName == "" {
			continue
		}

		if discoveredGroups[groupName] {
			continue // 避免重复注册
		}

		// 注册一个空的路由组，等待业务代码中的 init() 填充
		RegisterGroupRouter(
			groupName,
			"/"+groupName,
			10,
			nil,
			func(server *HTTPServer) {
				log.Debugf("执行空的路由组函数: %s (包路径: %s)", groupName, pkg)
			},
		)

		discoveredGroups[groupName] = true
		log.Debugf("自动注册路由组: %s (来自包: %s)", groupName, pkg)
	}
}

// extractGroupName 从包路径中提取组名
func extractGroupName(pkgPath string) string {
	// 使用包名作为组名
	groupName := filepath.Base(pkgPath)

	// 如果是版本化的包，如 "v1", "v2"，取上一级目录
	if strings.HasPrefix(groupName, "v") && len(groupName) <= 3 {
		parts := strings.Split(pkgPath, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2]
		}
	}

	return groupName
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
