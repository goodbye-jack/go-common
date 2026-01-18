package http

import (
	"bytes"
	"context"
	"github.com/goodbye-jack/go-common/log"
	"os"
	"os/exec"
	"path/filepath"
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
)

// InitAutoDiscoverRoutes 一键初始化（兼容原有逻辑+自动扫描，零脚本）
func (s *HTTPServer) InitAutoDiscoverRoutes() error {
	// 1. 自动扫描路由包（缓存结果，仅扫描一次）
	pkgPaths := autoDiscoverHandlerPackages()
	if len(pkgPaths) == 0 {
		log.Info("没有扫描到路由包")
		return nil
	}

	log.Infof("扫描到 %d 个可能的路由包，开始自动注册...", len(pkgPaths))

	// 2. 自动注册扫描到的路由组
	for _, pkg := range pkgPaths {
		groupName := extractGroupName(pkg)
		if groupName == "" {
			continue
		}

		// 检查是否已经注册过
		groupRegistryMu.Lock()
		alreadyRegistered := false
		for _, entry := range groupRegistry {
			if entry.groupName == groupName {
				alreadyRegistered = true
				break
			}
		}
		groupRegistryMu.Unlock()

		if alreadyRegistered {
			log.Debugf("路由组 %s 已经注册过，跳过", groupName)
			continue
		}

		// 注册路由组
		RegisterGroupRouter(
			groupName,
			"/"+groupName,
			10,
			nil,
			func(server *HTTPServer) {
				// 这个函数会被 executeAllRouteGroups 调用
				log.Debugf("执行自动发现的路由组: %s (包: %s)", groupName, pkg)
			},
		)

		log.Infof("自动注册路由组: %s (来自包: %s)", groupName, pkg)
	}

	return nil
}

// autoDiscoverHandlerPackages 自动扫描
func autoDiscoverHandlerPackages() []string {
	cacheOnce.Do(func() {
		// 自动获取项目根目录（缓存）
		getRootDir()
		if cachedRootDir == "" {
			log.Warn("未找到项目根目录（go.mod），跳过路由包自动扫描")
			return
		}

		log.Infof("开始扫描路由包，项目根目录: %s", cachedRootDir)

		// 方法1：扫描所有包，然后过滤
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "go", "list", "-f", "{{.ImportPath}}", "./...")
		cmd.Dir = cachedRootDir
		cmd.Env = append(os.Environ(), "GOGC=off")

		var outBuf bytes.Buffer
		cmd.Stdout = &outBuf

		if err := cmd.Run(); err != nil {
			log.Warnf("扫描项目包失败：%v", err)
			return
		}

		// 解析结果
		allPackages := strings.Fields(outBuf.String())
		log.Infof("找到 %d 个包，开始过滤...", len(allPackages))

		for _, pkg := range allPackages {
			// 过滤掉vendor目录和测试包
			if strings.Contains(pkg, "/vendor/") ||
				strings.Contains(pkg, "_test") ||
				strings.HasSuffix(pkg, ".test") {
				continue
			}

			// 更宽松的过滤规则
			if isPossibleRoutePackage(pkg) {
				cachedPkgPaths = append(cachedPkgPaths, pkg)
				log.Debugf("发现可能的路由包: %s", pkg)
			}
		}

		log.Infof("自动扫描完成，找到 %d 个可能的路由包", len(cachedPkgPaths))
	})
	return cachedPkgPaths
}

// isPossibleRoutePackage 检查包是否可能是路由包（更宽松的规则）
func isPossibleRoutePackage(pkgPath string) bool {
	// 你的项目结构中的路由文件命名
	routePatterns := []string{
		"handler",  // 标准命名
		"handlers", // 标准命名
		"route",    // 标准命名
		"routes",   // 标准命名
		"api",      // 标准命名
		"pano",     // 你的项目特有
		"mf",       // 你的项目特有
		"test",     // 你的项目特有
	}

	// 获取包名
	pkgName := filepath.Base(pkgPath)

	// 检查包名是否包含关键词
	pkgNameLower := strings.ToLower(pkgName)
	for _, pattern := range routePatterns {
		if strings.Contains(pkgNameLower, pattern) {
			return true
		}
	}

	// 检查完整路径是否包含关键词
	pkgPathLower := strings.ToLower(pkgPath)
	for _, pattern := range routePatterns {
		if strings.Contains(pkgPathLower, "/"+pattern) {
			return true
		}
	}

	// 检查文件扩展名（如果文件以 -route.go 或 -handler.go 结尾）
	if strings.HasSuffix(pkgPathLower, "-route") || strings.HasSuffix(pkgPathLower, "-handler") {
		return true
	}

	return false
}

// extractGroupName 从包路径中提取组名
func extractGroupName(pkgPath string) string {
	// 获取包名
	groupName := filepath.Base(pkgPath)

	// 清理常见的后缀
	commonSuffixes := []string{
		"-handler", "-handlers", "-route", "-routes",
		"handler", "handlers", "route", "routes",
		"-api", "api",
	}

	for _, suffix := range commonSuffixes {
		if strings.HasSuffix(groupName, suffix) {
			groupName = strings.TrimSuffix(groupName, suffix)
			break
		}
	}

	// 清理连字符和空格
	groupName = strings.Trim(groupName, "-_ ")

	// 如果清理后为空，使用原始包名
	if groupName == "" {
		return filepath.Base(pkgPath)
	}

	return groupName
}

// getRootDir 自动获取项目根目录
func getRootDir() {
	rootDirOnce.Do(func() {
		// 方法1：从当前工作目录向上查找
		wd, err := os.Getwd()
		if err != nil {
			log.Warnf("获取当前目录失败：%v", err)
			return
		}

		startDir := wd
		for {
			modPath := filepath.Join(wd, "go.mod")
			if _, err := os.Stat(modPath); err == nil {
				cachedRootDir = wd
				log.Infof("找到项目根目录: %s (从 %s 开始查找)", cachedRootDir, startDir)
				return
			}

			parent := filepath.Dir(wd)
			if parent == wd {
				break
			}
			wd = parent
		}

		// 方法2：从环境变量获取
		if envRoot := os.Getenv("GO_PROJECT_ROOT"); envRoot != "" {
			if _, err := os.Stat(filepath.Join(envRoot, "go.mod")); err == nil {
				cachedRootDir = envRoot
				log.Infof("使用环境变量指定的项目根目录: %s", cachedRootDir)
				return
			}
		}

		log.Warn("未找到项目根目录（go.mod）")
	})
}
