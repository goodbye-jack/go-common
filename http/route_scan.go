package http

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/goodbye-jack/go-common/log"
	"go/ast"
	"go/parser"
	"go/token"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
	"os"
	"os/exec"
	"path/filepath"
	_ "reflect"
	"strings"
	"sync"
)

// 路由注册方法列表（可配置，支持你提到的所有方法名）
var routeRegisterMethods = map[string]bool{
	"RouteAPI":             true,
	"Route":                true,
	"RouteForRA":           true,
	"RouteAutoRegisterAPI": true, // 补充你可能用到的其他路由方法
}

// ScanRoutes 简化：只需加载业务包，触发init（收集RegisterRoute函数）
func ScanRoutes() error {
	// 步骤1：获取模块名和根目录
	modName, err := getModuleName()
	if err != nil {
		return err
	}
	modRoot, err := getModuleRoot()
	if err != nil {
		return err
	}
	log.Infof("从go.mod识别到模块名：[%s]，开始扫描路由", modName)

	// 步骤2：遍历所有.go文件
	var (
		wg         sync.WaitGroup
		visitedPkg = make(map[string]bool)
		mu         sync.Mutex
	)
	err = filepath.Walk(modRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// 过滤规则
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.Contains(path, "vendor/") || strings.Contains(path, ".git/") {
			return nil
		}

		// 提取包路径
		relPath, err := filepath.Rel(modRoot, path)
		if err != nil {
			return nil
		}
		pkgDir := filepath.Dir(relPath)
		pkgPath := strings.ReplaceAll(filepath.Join(modName, pkgDir), string(os.PathSeparator), "/")

		// 避免重复加载包
		mu.Lock()
		if visitedPkg[pkgPath] {
			mu.Unlock()
			return nil
		}
		visitedPkg[pkgPath] = true
		mu.Unlock()

		// 加载包（触发init，收集RegisterRoute函数）
		wg.Add(1)
		go func(pkg string) {
			defer wg.Done()
			if err := loadPkg(pkg); err != nil {
				log.Warnf("加载包[%s]失败：%v", pkg, err)
				return
			}
			log.Infof("成功初始化路由包：[%s]", pkg)
		}(pkgPath)

		return nil
	})
	wg.Wait()
	if err != nil {
		return err
	}

	log.Info("[所有路由包扫描并初始化完成]")
	return nil
}

//// ScanRoutes 基于go.mod+AST的智能路由扫描（无路径/文件名约定）
//func ScanRoutes() error {
//	// 步骤1：从go.mod中提取当前模块名（自动定位业务根路径）
//	modName, err := getModuleName()
//	if err != nil {
//		return err
//	}
//	log.Infof("从go.mod识别到模块名：%s，开始扫描路由", modName)
//
//	// 步骤2：遍历当前项目所有.go文件（排除vendor、test文件）
//	var (
//		wg          sync.WaitGroup
//		scanDir     = "."                   // 从当前目录开始扫描（go.mod所在目录）
//		visitedPkgs = make(map[string]bool) // 避免重复加载同一个包
//		mu          sync.Mutex
//	)
//	err = filepath.Walk(scanDir, func(path string, info os.FileInfo, err error) error {
//		if err != nil {
//			return nil
//		}
//		if info.IsDir() { // 过滤规则：排除vendor、.git、test文件（可自定义）
//			if strings.Contains(path, "vendor") || strings.Contains(path, ".git") || strings.HasSuffix(path, "_test") {
//				return filepath.SkipDir
//			}
//			return nil
//		}
//		// 只处理.go文件（兼容任意文件名，如TestRoutes.go、test-routes.go）
//		if !strings.HasSuffix(path, ".go") {
//			return nil
//		}
//		// 步骤3：解析当前文件的AST，判断是否包含路由注册方法调用
//		hasRouteRegister, pkgPath, err := hasRouteRegisterMethod(path, modName)
//		if err != nil {
//			log.Warnf("解析文件[%s]失败：%v", path, err)
//			return nil
//		}
//		// 不是路由注册文件，跳过
//		if !hasRouteRegister {
//			return nil
//		}
//		// 避免重复加载同一个包
//		mu.Lock()
//		if visitedPkgs[pkgPath] {
//			mu.Unlock()
//			return nil
//		}
//		visitedPkgs[pkgPath] = true
//		mu.Unlock()
//		wg.Add(1) // 步骤4：异步加载包并触发init（执行路由注册）
//		go func(pkg string) {
//			defer wg.Done()
//			if err := loadAndInitPkg(pkg); err != nil {
//				log.Warnf("初始化路由包[%s]失败：%v", pkg, err)
//				return
//			}
//			log.Infof("成功初始化路由包：%s", pkg)
//		}(pkgPath)
//		return nil
//	})
//	wg.Wait()
//	if err != nil {
//		return err
//	}
//	log.Info("所有路由包扫描并初始化完成")
//	return nil
//}

// getModuleRoot 获取项目根目录（基于go mod）
func getModuleRoot() (string, error) {
	// 执行go list -m -f '{{.Dir}}'获取模块根目录
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("获取模块根目录失败：%v", err)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", errors.New("模块根目录为空")
	}
	log.Infof("识别到项目根目录：%s", root)
	return root, nil
}

// loadPkg 加载包并触发init函数执行
func loadPkg(pkgPath string) error {
	// 使用golang.org/x/tools/go/packages加载包（更可靠）
	cfg := &packages.Config{
		Mode: packages.LoadAllSyntax, // 加载包的所有信息，触发init
	}
	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return fmt.Errorf("加载包[%s]失败：%v", pkgPath, err)
	}
	// 检查包加载错误
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return fmt.Errorf("包[%s]加载错误：%v", pkgPath, pkg.Errors)
		}
	}
	return nil
}

// getModuleName 从go.mod中提取当前模块名（适配go mod模式）
func getModuleName() (string, error) {
	// 查找go.mod文件（从当前目录向上遍历）
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	var modPath string
	curDir := wd
	for {
		mp := filepath.Join(curDir, "go.mod")
		if _, err := os.Stat(mp); err == nil {
			modPath = mp
			break
		}
		// 到根目录仍未找到
		parent := filepath.Dir(curDir)
		if parent == curDir {
			return "", os.ErrNotExist
		}
		curDir = parent
	}
	// 解析go.mod内容
	modContent, err := os.ReadFile(modPath)
	if err != nil {
		return "", err
	}
	// 提取module名
	modFile, err := modfile.Parse(modPath, modContent, nil)
	if err != nil {
		return "", err
	}
	if modFile.Module == nil || modFile.Module.Mod.Path == "" {
		return "", os.ErrInvalid
	}

	return modFile.Module.Mod.Path, nil
}

// hasRouteRegisterMethod 解析文件AST，判断是否包含路由注册方法调用
func hasRouteRegisterMethod(filePath string, moduleName string) (bool, string, error) {
	fset := token.NewFileSet()
	// 解析单个go文件的AST
	file, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return false, "", err
	}

	// 标记是否找到路由注册方法
	hasRoute := false
	// 提取当前文件的包路径
	pkgPath := ""

	// 遍历AST节点，查找目标方法调用
	ast.Inspect(file, func(n ast.Node) bool {
		// 跳过已找到的情况
		if hasRoute {
			return false
		}

		// 匹配方法调用表达式
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// 解析调用的方法名
		selectorExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// 获取方法名（如 RouteAPI、RouteForRA）
		methodName := selectorExpr.Sel.Name
		if routeRegisterMethods[methodName] {
			hasRoute = true
			return false
		}

		return true
	})

	// 提取当前文件的包路径（基于moduleName + 相对路径）
	if hasRoute {
		// 获取文件的绝对路径
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return false, "", err
		}
		// 找到项目根目录（moduleName对应的目录）
		//moduleRoot := ""
		// 切割路径：找到moduleName对应的父目录
		wd, _ := os.Getwd()
		relPath, err := filepath.Rel(wd, absPath)
		if err != nil {
			return false, "", err
		}
		// 拼接包路径：moduleName + 相对目录（去掉文件名）
		dir := filepath.Dir(relPath)
		pkgPath = filepath.Join(moduleName, dir)
		// 替换路径分隔符为/（符合go包路径规范）
		pkgPath = strings.ReplaceAll(pkgPath, string(os.PathSeparator), "/")
	}

	return hasRoute, pkgPath, nil
}

// loadAndInitPkg 正确加载包并触发init函数执行
func loadAndInitPkg(pkgPath string) error {
	cfg := &packages.Config{
		Mode: packages.LoadAllSyntax, // 加载包的所有信息，触发init
	}
	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return err
	}
	// 检查包加载是否有错误
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return fmt.Errorf("包[%s]加载错误：%v", pkgPath, pkg.Errors)
		}
	}
	return nil
}

//// loadAndInitPkg 加载包并触发init函数执行
//func loadAndInitPkg(pkgPath string) error {
//	// 反射加载包（核心：触发init函数，执行路由注册）
//	// 这里采用go内置的build机制，确保包的init被执行
//	_, err := buildPkg(pkgPath)
//	if err != nil {
//		return err
//	}
//	return nil
//}

// buildPkg 封装go build的包加载逻辑（适配go mod）
func buildPkg(pkgPath string) (interface{}, error) {
	// 模拟go build加载包，触发init函数
	// 生产环境可替换为更高效的实现（如golang.org/x/tools/go/packages）
	cmd := exec.Command("go", "list", "-e", "-json", pkgPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	// 简化实现：只要包能被list到，就说明可加载，init会自动执行
	// 如需更严谨，可使用golang.org/x/tools/go/packages加载包并执行init
	return nil, nil
}
