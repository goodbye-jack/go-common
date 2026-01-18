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
	if len(pkgPaths) > 0 {
		// 2. 自动注册扫描到的路由组
		autoRegisterGroups(pkgPaths)

		// 3. 执行新发现的包中的路由组
		executeAllRouteGroups(s)
	}

	log.Infof("自动发现路由完成，总计 %d 个路由", len(s.routes))
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

		// 扫描常见的路由包目录
		scanPaths := []string{
			"./internal/handler/...",
			"./internal/handlers/...",
			"./internal/routes/...",
			"./internal/api/...",
			"./pkg/handler/...",
			"./app/handler/...",
		}

		for _, scanPath := range scanPaths {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)

			cmd := exec.CommandContext(ctx, "go", "list", "-f", "{{.ImportPath}}", scanPath)
			cmd.Dir = cachedRootDir
			cmd.Env = append(os.Environ(), "GOGC=off")

			var outBuf bytes.Buffer
			cmd.Stdout = &outBuf

			if err := cmd.Run(); err != nil {
				cancel()
				// 忽略不存在的路径
				continue
			}
			cancel()

			// 解析结果
			lines := strings.Fields(outBuf.String())
			for _, line := range lines {
				// 过滤掉通配符路径和vendor目录
				if !strings.HasSuffix(line, "/...") && !strings.Contains(line, "/vendor/") {
					cachedPkgPaths = append(cachedPkgPaths, line)
				}
			}
		}

		log.Infof("自动扫描到 %d 个路由包", len(cachedPkgPaths))
	})
	return cachedPkgPaths
}

// autoRegisterGroups 自动注册路由组
func autoRegisterGroups(pkgPaths []string) {
	for _, pkg := range pkgPaths {
		groupName := extractGroupName(pkg)
		if groupName == "" {
			continue
		}

		// 检查是否已经注册过
		groupExecutedMu.RLock()
		alreadyExecuted := groupExecuted[groupName]
		groupExecutedMu.RUnlock()

		if alreadyExecuted {
			continue
		}

		// 自动注册路由组（使用空函数，依赖业务代码中的init）
		RegisterGroupRouter(
			groupName,
			"/"+groupName,
			10,
			nil,
			func(server *HTTPServer) {
				log.Debugf("自动发现的路由组: %s (来自包: %s)", groupName, pkg)
				// 空函数，业务代码应该在自己的init中注册路由
			},
		)

		log.Debugf("自动注册路由组: %s (来自包: %s)", groupName, pkg)
	}
}

// extractGroupName 从包路径中提取组名
func extractGroupName(pkgPath string) string {
	groupName := filepath.Base(pkgPath)

	// 如果是版本号（如 v1, v2），则取上一级目录
	if strings.HasPrefix(groupName, "v") && len(groupName) <= 3 {
		parts := strings.Split(pkgPath, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2]
		}
	}

	return groupName
}

// getRootDir 自动获取项目根目录
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
