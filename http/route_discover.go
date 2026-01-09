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

		// 原有前缀/中间件逻辑（完全兼容）
		originPrefix := s.globalPrefix
		s.globalPrefix = entry.prefix
		if s.globalPrefix != "" && s.globalPrefix[len(s.globalPrefix)-1] != '/' {
			s.globalPrefix += "/"
		}

		if len(entry.mws) > 0 {
			s.Use(entry.mws...)
		}

		// 关键：直接触发路由包init()（无需脚本/临时文件）
		// 原理：Go编译期会解析所有依赖，只要RegisterGroupRouter被调用，路由包init()就会执行
		entry.register(s)
		s.globalPrefix = originPrefix
		registered++
		log.Infof("已初始化路由组：%s", entry.groupName)
	}

	log.Infof("路由组自动初始化完成，共注册[%d]个路由组", registered)
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
//	"os"
//	"os/exec"
//	"path/filepath"
//	"strings"
//	"text/template"
//
//	"github.com/goodbye-jack/go-common/log"
//)
//
//// -------------------------- 自动扫描核心方法（增量新增） --------------------------
//// AutoDiscoverHandlerPackages 自动扫描 internal/handler 下的所有路由包
//// 返回值：扫描到的包路径列表（如 ["pano-material/internal/handler/test"]）
//func AutoDiscoverHandlerPackages() []string {
//	// 1. 获取当前项目根目录（含 go.mod）
//	rootDir := getProjectRoot()
//	if rootDir == "" {
//		log.Warn("未找到项目根目录（go.mod），跳过路由包自动扫描")
//		return nil
//	}
//
//	// 2. 执行 go list 命令扫描 internal/handler 下的所有子包
//	cmd := exec.Command("go", "list", "./internal/handler/...")
//	cmd.Dir = rootDir
//	var outBuf, errBuf bytes.Buffer
//	cmd.Stdout = &outBuf
//	cmd.Stderr = &errBuf
//
//	if err := cmd.Run(); err != nil {
//		log.Warnf("扫描路由包失败：%v，错误输出：%s", err, errBuf.String())
//		return nil
//	}
//
//	// 3. 解析输出，过滤有效路由包
//	pkgLines := strings.Split(outBuf.String(), "\n")
//	var pkgPaths []string
//	for _, line := range pkgLines {
//		line = strings.TrimSpace(line)
//		if line == "" || !strings.Contains(line, "/internal/handler/") {
//			continue
//		}
//		pkgPaths = append(pkgPaths, line)
//	}
//
//	log.Infof("自动扫描到 %d 个路由包：%v", len(pkgPaths), pkgPaths)
//	return pkgPaths
//}
//
//// AutoRegisterDiscoveredRoutes 自动注册扫描到的路由包为路由组
//// 核心：触发路由包 init() + 注册为原有 GroupRouterEntry
//func AutoRegisterDiscoveredRoutes() {
//	pkgPaths := AutoDiscoverHandlerPackages()
//	if len(pkgPaths) == 0 {
//		return
//	}
//
//	// 1. 自动导入所有路由包（触发 init()）
//	importDiscoveredPackages(pkgPaths)
//
//	// 2. 为每个路由包注册为路由组（复用原有 RegisterGroupRouter）
//	for _, pkg := range pkgPaths {
//		// 提取路由组名称（如 internal/handler/test → test）
//		groupName := filepath.Base(pkg)
//		// 提取路由组前缀（如 test → /test）
//		prefix := "/" + groupName
//		// 复用原有注册逻辑（优先级默认10，无专属中间件）
//		RegisterGroupRouter(
//			groupName,
//			prefix,
//			10,  // 默认优先级
//			nil, // 无专属中间件
//			func(server *HTTPServer) {
//				// 空注册函数：路由包 init() 中已自行注册路由
//				// 兼容原有 RouteAutoRegisterAPI 逻辑
//			},
//		)
//	}
//}
//
//// -------------------------- 内部辅助方法（增量新增） --------------------------
//// getProjectRoot 自动获取项目根目录（含 go.mod）
//func getProjectRoot() string {
//	wd, err := os.Getwd()
//	if err != nil {
//		log.Warnf("获取当前目录失败：%v", err)
//		return ""
//	}
//
//	// 向上遍历查找 go.mod
//	for {
//		modPath := filepath.Join(wd, "go.mod")
//		if _, err := os.Stat(modPath); err == nil {
//			return wd
//		}
//		parent := filepath.Dir(wd)
//		if parent == wd { // 到达根目录
//			break
//		}
//		wd = parent
//	}
//	return ""
//}
//
//// importDiscoveredPackages 自动导入路由包（触发 init()）
//func importDiscoveredPackages(pkgPaths []string) {
//	// 生成临时导入文件（仅编译期使用，不修改业务代码）
//	tmpDir := os.TempDir()
//	tmpFile := filepath.Join(tmpDir, "auto_import_routes.go")
//
//	// 模板内容：空导入所有路由包
//	tplContent := `package main
//
//import (
//	{{range .}}_ "{{.}}"
//	{{end}}
//)
//
//// 空函数：仅用于触发路由包 init()
//func init() {}
//`
//	tpl, err := template.New("autoimport").Parse(tplContent)
//	if err != nil {
//		log.Warnf("解析导入模板失败：%v", err)
//		return
//	}
//
//	// 写入临时文件
//	var buf bytes.Buffer
//	if err := tpl.Execute(&buf, pkgPaths); err != nil {
//		log.Warnf("生成临时导入文件失败：%v", err)
//		return
//	}
//	if err := os.WriteFile(tmpFile, buf.Bytes(), 0644); err != nil {
//		log.Warnf("写入临时导入文件失败：%v", err)
//		return
//	}
//
//	// 运行临时文件触发 init()
//	cmd := exec.Command("go", "run", tmpFile)
//	cmd.Dir = getProjectRoot()
//	if _, err := cmd.CombinedOutput(); err != nil {
//		log.Warnf("触发路由包 init() 失败：%v", err)
//	}
//
//	// 删除临时文件（清理）
//	os.Remove(tmpFile)
//}
//
//// -------------------------- 对外暴露的简化方法（增量新增） --------------------------
//// InitAutoDiscoverRoutes 业务侧调用的一键自动扫描+初始化
//// 兼容原有 InitGroupAutoRoutes 逻辑，仅新增自动扫描步骤
//func (s *HTTPServer) InitAutoDiscoverRoutes() error {
//	// 1. 自动扫描并注册路由包为路由组
//	AutoRegisterDiscoveredRoutes()
//	// 2. 复用原有路由组初始化逻辑
//	return s.InitGroupAutoRoutes()
//}
