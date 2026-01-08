package autoimport

import (
	"bufio"
	"fmt"
	"github.com/goodbye-jack/go-common/generator"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// 全局锁：防止多goroutine重复生成
var genLock sync.Once

// 从go.mod解析module名（通用逻辑）
func getModuleName() (string, error) {
	// 获取项目根目录（从当前工作目录向上找go.mod）
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// 向上遍历找go.mod
	for {
		goModPath := filepath.Join(wd, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			// 找到go.mod，解析module名
			file, err := os.Open(goModPath)
			if err != nil {
				return "", err
			}
			defer file.Close()

			moduleRegex := regexp.MustCompile(`^module\s+(\S+)$`)
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if match := moduleRegex.FindStringSubmatch(line); match != nil {
					return match[1], nil
				}
			}
			return "", fmt.Errorf("go.mod中未找到module声明")
		}
		// 到根目录仍未找到
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", fmt.Errorf("未找到go.mod文件，请在项目根目录执行")
		}
		wd = parent
	}
}

// 编译期自动生成路由导入文件（init触发，仅执行一次）
func init() {
	genLock.Do(func() {
		// 1. 动态解析业务项目module名
		moduleName, err := getModuleName()
		if err != nil {
			fmt.Printf("[autoimport] 解析module名失败（非致命，路由可能未加载）：%v\n", err)
			return
		}
		// 2. 调用核心生成函数（通用默认配置，可通过环境变量自定义）
		matchPatterns := os.Getenv("ROUTE_MATCH_PATTERNS")
		if matchPatterns == "" {
			matchPatterns = "internal/handler/*,*Route,*router"
		}
		outputDir := os.Getenv("ROUTE_OUTPUT_DIR")
		if outputDir == "" {
			outputDir = "routes"
		}
		ignoreDirs := os.Getenv("ROUTE_IGNORE_DIRS")
		if ignoreDirs == "" {
			ignoreDirs = "vendor,.git,testdata,docs"
		}
		// 3. 生成路由导入文件
		err = generator.AutoGenRouteImport(moduleName, "", outputDir, matchPatterns, ignoreDirs)
		if err != nil {
			fmt.Printf("[autoimport] 生成路由文件失败（非致命，路由可能未加载）：%v\n", err)
			return
		}

		fmt.Printf("[autoimport] ✅ 自动生成路由导入文件完成：%s/auto_import.go\n", outputDir)
	})
}
