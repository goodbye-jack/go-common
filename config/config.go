package config

import (
	"fmt"
	"github.com/goodbye-jack/go-common/configsync"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/orm"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"strings"
)

var configPaths = []string{".", "./config", "/opt"} // config配置读取顺序

// 这是测试值,
//var configPaths = []string{".", "./config", "/opt", "./example"}

func init() {
	inspection := prepareConfigBootstrap()
	configPaths = buildConfigPaths(inspection)

	globalViper := viper.New() // 1. 初始化全局Viper
	baseViper := viper.New()   // 2. 先读基础配置（可选）
	baseViper.SetConfigName("config")
	baseViper.SetConfigType("yaml")
	for _, path := range configPaths {
		baseViper.AddConfigPath(path)
	}
	if err := baseViper.ReadInConfig(); err == nil { // 合并基础配置到全局
		for _, key := range baseViper.AllKeys() {
			globalViper.Set(key, baseViper.Get(key))
		}
		log.Infof("读取基础配置：%s", baseViper.ConfigFileUsed())
	} else if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
		log.Warnf("读取基础配置失败：%v", err)
	}
	env := os.Getenv("CONFIG_ENV") // 3. 再读环境配置（可选，覆盖基础配置）
	if env != "" {
		envViper := viper.New()
		envViper.SetConfigName("config." + env)
		envViper.SetConfigType("yaml")
		for _, path := range configPaths {
			envViper.AddConfigPath(path)
		}
		if err := envViper.ReadInConfig(); err == nil { // 覆盖基础配置
			for _, key := range envViper.AllKeys() {
				globalViper.Set(key, envViper.Get(key))
			}
			log.Infof("读取%s环境配置：%s", env, envViper.ConfigFileUsed())
		} else if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.Warnf("读取环境配置失败：%v", err)
		}
	}
	if len(globalViper.AllKeys()) == 0 { // 4. 最终校验：如果全局Viper无任何配置，才报错
		log.Fatalf("未找到任何配置文件！请检查%v下是否有config.yml或config.%s.yml", configPaths, env)
	}
	for _, key := range globalViper.AllKeys() { // 5. 将全局Viper的配置同步到默认Viper
		viper.Set(key, globalViper.Get(key))
	}
	if globalViper.IsSet("service_name") { // 读取service_name的值并传给log.Init()
		serviceName := globalViper.GetString("service_name")
		log.LoadPrintProjectName(serviceName) // 把项目名传给日志初始化函数
	}
	// ========== 新增：自动初始化数据库 ==========
	if err := orm.InitAllDB(globalViper); err != nil {
		log.Fatalf("数据库自动初始化失败：%v", err)
	}
}

func prepareConfigBootstrap() *configsync.Inspection {
	currentDir, err := os.Getwd()
	if err != nil {
		log.Warnf("获取当前工作目录失败，跳过配置自动同步：%v", err)
		return nil
	}
	inspection, err := configsync.InspectProject(currentDir)
	if err != nil {
		log.Warnf("检查项目目录失败，跳过配置自动同步：%v", err)
		return nil
	}
	decision := shouldAutoSync(inspection)
	if !decision.allowed {
		if decision.logOnSkip && strings.TrimSpace(decision.reason) != "" {
			log.Infof("跳过go-common配置自动同步：%s", decision.reason)
		}
		return inspection
	}
	result, err := configsync.SyncProjectDefault(inspection.ProjectDir)
	if err != nil {
		log.Warnf("go-common配置自动同步失败：%v", err)
		return inspection
	}
	logAutoSyncResult(result, decision)
	return inspection
}

type autoSyncDecision struct {
	allowed   bool
	reason    string
	logOnSkip bool
	forced    bool
}

func shouldAutoSync(inspection *configsync.Inspection) autoSyncDecision {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("GO_COMMON_CONFIG_SYNC")))
	switch mode {
	case "", "auto":
	case "off", "false", "0", "disable", "disabled":
		return autoSyncDecision{reason: "GO_COMMON_CONFIG_SYNC=off", logOnSkip: true}
	case "force", "on", "true", "1", "enable", "enabled":
		if inspection == nil {
			return autoSyncDecision{reason: "未识别到项目目录"}
		}
		if inspection.IsGoCommonModule {
			return autoSyncDecision{reason: "当前模块为go-common自身"}
		}
		if !inspection.HasGoMod {
			return autoSyncDecision{reason: "未找到go.mod"}
		}
		return autoSyncDecision{allowed: true, forced: true}
	default:
		log.Warnf("未知的GO_COMMON_CONFIG_SYNC模式：%s，将按auto处理", mode)
	}
	if inspection == nil {
		return autoSyncDecision{reason: "未识别到项目目录"}
	}
	if !inspection.HasGoMod {
		return autoSyncDecision{reason: "未找到go.mod"}
	}
	if inspection.IsGoCommonModule {
		return autoSyncDecision{reason: "当前模块为go-common自身"}
	}
	if !inspection.HasGoCommonDependency {
		return autoSyncDecision{reason: "当前项目未检测到go-common依赖"}
	}
	if blocked, reason := blockedByProductionLikeEnv(); blocked {
		return autoSyncDecision{reason: reason}
	}
	return autoSyncDecision{allowed: true}
}

func blockedByProductionLikeEnv() (bool, string) {
	for _, envKey := range []string{"GO_ENV", "APP_ENV", "RUN_ENV", "CONFIG_ENV"} {
		envValue := strings.ToLower(strings.TrimSpace(os.Getenv(envKey)))
		switch envValue {
		case "prod", "production":
			return true, fmt.Sprintf("%s=%s", envKey, envValue)
		}
	}
	return false, ""
}

func logAutoSyncResult(result *configsync.Result, decision autoSyncDecision) {
	if result == nil {
		return
	}
	if decision.forced {
		log.Infof(
			"go-common配置自动同步完成：project=%s, initialized=%t, missing=%d, latest=%s",
			result.ProjectDir,
			result.Initialized,
			len(result.MissingKeys),
			result.LatestPath,
		)
		return
	}
	switch {
	case result.Initialized:
		log.Infof("go-common已初始化真实配置：config=%s", result.ConfigPath)
	case result.PreviousVersion != "" && result.PreviousVersion != result.TargetVersion:
		log.Infof(
			"go-common检测到配置模板升级：from=%s, to=%s, missing=%d",
			result.PreviousVersion,
			result.TargetVersion,
			len(result.MissingKeys),
		)
	case len(result.MissingKeys) > 0:
		log.Infof("go-common检测到配置缺失项：missing=%d, file=%s", len(result.MissingKeys), result.MissingPath)
	}
}

func buildConfigPaths(inspection *configsync.Inspection) []string {
	candidatePaths := []string{".", "./config"}
	if inspection != nil && strings.TrimSpace(inspection.ProjectDir) != "" {
		projectDir := filepath.Clean(inspection.ProjectDir)
		candidatePaths = append(candidatePaths, projectDir, filepath.Join(projectDir, "config"))
	}
	candidatePaths = append(candidatePaths, "/opt")

	uniquePaths := make([]string, 0, len(candidatePaths))
	seen := make(map[string]struct{})
	for _, path := range candidatePaths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		key := trimmed
		if strings.HasPrefix(trimmed, ".") {
			key = trimmed
		} else {
			key = filepath.Clean(trimmed)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		uniquePaths = append(uniquePaths, trimmed)
	}
	return uniquePaths
}

// 保留原有读取方法
func GetConfigString(name string) string { return viper.GetString(name) }
func GetConfigInt(name string) int       { return viper.GetInt(name) }
func GetConfigBool(name string) bool     { return viper.GetBool(name) }
