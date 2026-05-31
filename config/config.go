package config

import (
	"fmt"
	"github.com/goodbye-jack/go-common/configsync"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/orm"
	templatefs "github.com/goodbye-jack/go-common/templates"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var configPaths = []string{".", "./config", "/opt"} // config配置读取顺序

type overrideSelection struct {
	ConfigFile   string
	ConfigEnv    string
	ExplicitFile bool
}

type configLayeringPolicy struct {
	Version    string `yaml:"version"`
	Validation struct {
		ForbiddenExactKeys []string `yaml:"forbidden_exact_keys"`
	} `yaml:"validation"`
}

// 这是测试值,
//var configPaths = []string{".", "./config", "/opt", "./example"}

func init() {
	inspection := prepareConfigBootstrap()
	configPaths = buildConfigPaths(inspection)

	globalViper := viper.New() // 1. 初始化全局Viper
	baseViper := readConfigWithSearchPaths("config", configPaths)
	if baseViper == nil {
		log.Fatalf("未找到基础配置文件！请检查%v 下是否存在 config.yaml", configPaths)
	}
	if err := mergeViper(globalViper, baseViper); err != nil {
		log.Fatalf("合并基础配置失败：%v", err)
	}
	log.Infof("读取基础配置：%s", baseViper.ConfigFileUsed())

	overrideViper, overrideLabel, overrideSelection := readConfigOverride(configPaths)
	if overrideViper != nil {
		if err := validateConfigOverride(overrideSelection, overrideViper); err != nil {
			log.Fatalf("覆盖配置校验失败：%v", err)
		}
		if err := mergeViper(globalViper, overrideViper); err != nil {
			log.Fatalf("合并覆盖配置失败：%v", err)
		}
		log.Infof("读取覆盖配置：%s", overrideLabel)
	}

	resolvedAppEnv, envSource := resolveRuntimeAppEnv(baseViper, overrideSelection)
	if resolvedAppEnv != "" {
		globalViper.Set("app.env", resolvedAppEnv)
		log.Infof("运行环境已解析：app.env=%s, source=%s", resolvedAppEnv, envSource)
	}

	if len(globalViper.AllKeys()) == 0 {
		log.Fatalf("未找到任何配置文件！请检查%v 下是否有 config.yaml 或覆盖配置文件", configPaths)
	}
	for _, key := range globalViper.AllKeys() {
		viper.Set(key, globalViper.Get(key))
	}
	if err := validateDeprecatedKeys(globalViper); err != nil {
		log.Fatalf("发现已废弃配置项，请先迁移后再启动：%v", err)
	}
	if appName := GetAppName(); appName != "" {
		log.LoadPrintProjectName(appName)
	}
	if err := orm.InitAllDB(globalViper); err != nil {
		log.Fatalf("数据库自动初始化失败：%v", err)
	}
}

func readConfigWithSearchPaths(configName string, searchPaths []string) *viper.Viper {
	reader := viper.New()
	reader.SetConfigName(configName)
	reader.SetConfigType("yaml")
	for _, path := range searchPaths {
		reader.AddConfigPath(path)
	}
	if err := reader.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil
		}
		log.Warnf("读取配置失败：name=%s, err=%v", configName, err)
		return nil
	}
	return reader
}

func readConfigOverride(searchPaths []string) (*viper.Viper, string, *overrideSelection) {
	configFile := strings.TrimSpace(os.Getenv("CONFIG_FILE"))
	configEnv := strings.TrimSpace(os.Getenv("CONFIG_ENV"))
	if configFile != "" {
		reader := viper.New()
		reader.SetConfigType("yaml")
		if filePath, ok := locateExplicitConfigFile(configFile, searchPaths); ok {
			reader.SetConfigFile(filePath)
			if err := reader.ReadInConfig(); err != nil {
				log.Fatalf("读取 CONFIG_FILE=%s 失败：%v", filePath, err)
			}
			if configEnv != "" {
				log.Infof("检测到 CONFIG_FILE 与 CONFIG_ENV 同时存在，已按优先级使用 CONFIG_FILE，忽略 CONFIG_ENV=%s", configEnv)
			}
			return reader, filePath, &overrideSelection{
				ConfigFile:   filePath,
				ConfigEnv:    configEnv,
				ExplicitFile: true,
			}
		}
		log.Fatalf("CONFIG_FILE 指定的配置文件不存在：%s", configFile)
	}
	if configEnv == "" {
		return nil, "", nil
	}
	envName := "config." + configEnv
	reader := readConfigWithSearchPaths(envName, searchPaths)
	if reader == nil {
		log.Fatalf("CONFIG_ENV=%s 对应的覆盖配置不存在，请检查 config.%s.yaml", configEnv, configEnv)
	}
	return reader, reader.ConfigFileUsed(), &overrideSelection{
		ConfigFile:   reader.ConfigFileUsed(),
		ConfigEnv:    configEnv,
		ExplicitFile: false,
	}
}

func locateExplicitConfigFile(configFile string, searchPaths []string) (string, bool) {
	if filepath.IsAbs(configFile) {
		if stat, err := os.Stat(configFile); err == nil && !stat.IsDir() {
			return configFile, true
		}
		return "", false
	}
	if stat, err := os.Stat(configFile); err == nil && !stat.IsDir() {
		absPath, err := filepath.Abs(configFile)
		if err == nil {
			return absPath, true
		}
		return configFile, true
	}
	for _, base := range searchPaths {
		candidate := filepath.Join(base, configFile)
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			absPath, err := filepath.Abs(candidate)
			if err == nil {
				return absPath, true
			}
			return candidate, true
		}
	}
	return "", false
}

func mergeViper(dst, src *viper.Viper) error {
	if dst == nil || src == nil {
		return nil
	}
	for _, key := range src.AllKeys() {
		dst.Set(key, src.Get(key))
	}
	return nil
}

func validateConfigOverride(selection *overrideSelection, overrideViper *viper.Viper) error {
	if selection == nil || overrideViper == nil {
		return nil
	}
	policy, err := loadConfigLayeringPolicy()
	if err != nil {
		log.Warnf("读取配置分层规则失败，跳过覆盖配置校验：%v", err)
		return nil
	}
	forbidden := make(map[string]struct{}, len(policy.Validation.ForbiddenExactKeys))
	for _, key := range policy.Validation.ForbiddenExactKeys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		forbidden[trimmed] = struct{}{}
	}
	var invalidKeys []string
	for _, key := range overrideViper.AllKeys() {
		if _, exists := forbidden[key]; exists {
			invalidKeys = append(invalidKeys, key)
		}
	}
	if len(invalidKeys) > 0 {
		return fmt.Errorf("覆盖文件 %s 包含禁止项：%s；请按 go-common 配置分层规范处理覆盖文件", selection.ConfigFile, strings.Join(invalidKeys, ", "))
	}
	log.Infof("覆盖配置校验通过：file=%s, override_key_count=%d", selection.ConfigFile, len(overrideViper.AllKeys()))
	return nil
}

func loadConfigLayeringPolicy() (*configLayeringPolicy, error) {
	data, err := templatefs.FS.ReadFile(fmt.Sprintf("releases/%s/config.layering.yaml", configsync.CurrentVersion))
	if err != nil {
		return nil, err
	}
	var policy configLayeringPolicy
	if err := yaml.Unmarshal(data, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

func resolveRuntimeAppEnv(baseViper *viper.Viper, selection *overrideSelection) (string, string) {
	baseValue := ""
	if baseViper != nil {
		baseValue = strings.TrimSpace(baseViper.GetString("app.env"))
	}
	if selection == nil {
		if baseValue != "" {
			return baseValue, "config.yaml"
		}
		return "local", "default"
	}
	if selection.ExplicitFile {
		if inferred := inferEnvNameFromConfigFile(selection.ConfigFile); inferred != "" {
			return inferred, "CONFIG_FILE"
		}
		return "file", "CONFIG_FILE"
	}
	if env := strings.TrimSpace(selection.ConfigEnv); env != "" {
		return env, "CONFIG_ENV"
	}
	if baseValue != "" {
		return baseValue, "config.yaml"
	}
	return "local", "default"
}

func inferEnvNameFromConfigFile(filePath string) string {
	baseName := strings.ToLower(filepath.Base(filePath))
	for _, ext := range []string{".yaml", ".yml"} {
		if !strings.HasSuffix(baseName, ext) {
			continue
		}
		raw := strings.TrimSuffix(baseName, ext)
		if !strings.HasPrefix(raw, "config.") {
			continue
		}
		envName := strings.TrimPrefix(raw, "config.")
		switch envName {
		case "dev", "test", "prod", "local":
			return envName
		}
	}
	return ""
}

var deprecatedConfigKeys = map[string]string{
	"service_name":                 "app.name",
	"addr":                         "server.addr",
	"uc":                           "server.uc_addr",
	"http.gin_mode":                "server.http.gin_mode",
	"cookie_token":                 "security.cookie.name",
	"cookie_token_expired_seconds": "security.cookie.expired_seconds",
	"base_path":                    "storage.local.base_path",
	"relative_path_prefix":         "storage.local.relative_path_prefix",
	"production":                   "storage.local.cleanup_after_upload",
	"redis_addr":                   "databases.redis.default.host + port",
	"redis_password":               "databases.redis.default.password",
}

func validateDeprecatedKeys(v *viper.Viper) error {
	if v == nil {
		return nil
	}
	var hits []string
	for oldKey, newKey := range deprecatedConfigKeys {
		if !v.IsSet(oldKey) {
			continue
		}
		hits = append(hits, fmt.Sprintf("%s -> %s", oldKey, newKey))
	}
	if len(hits) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(hits, "; "))
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
			"go-common配置自动同步完成：project=%s, initialized=%t, missing=%d, deprecated=%d, rules=%s",
			result.ProjectDir,
			result.Initialized,
			len(result.MissingKeys),
			len(result.DeprecatedKeys),
			result.RulesPath,
		)
		return
	}
	switch {
	case result.Initialized:
		log.Infof("go-common已初始化真实配置：config=%s", result.ConfigPath)
	case result.PreviousVersion != "" && result.PreviousVersion != result.TargetVersion:
		log.Infof(
			"go-common检测到配置模板升级：from=%s, to=%s, missing=%d, deprecated=%d, rules=%s",
			result.PreviousVersion,
			result.TargetVersion,
			len(result.MissingKeys),
			len(result.DeprecatedKeys),
			result.RulesPath,
		)
	case len(result.MissingKeys) > 0 || len(result.DeprecatedKeys) > 0:
		log.Infof(
			"go-common检测到配置待处理项：missing=%d, deprecated=%d, file=%s",
			len(result.MissingKeys),
			len(result.DeprecatedKeys),
			result.RulesPath,
		)
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
func GetConfigStringSlice(name string) []string {
	values := viper.GetStringSlice(name)
	if len(values) == 0 {
		raw := strings.TrimSpace(viper.GetString(name))
		if raw == "" {
			return nil
		}
		values = strings.Split(raw, ",")
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func GetConfigDuration(name string, fallback time.Duration) time.Duration {
	raw := viper.Get(name)
	if raw == nil {
		return fallback
	}
	switch value := raw.(type) {
	case time.Duration:
		return value
	case int:
		return time.Duration(value) * time.Second
	case int64:
		return time.Duration(value) * time.Second
	case float64:
		return time.Duration(value) * time.Second
	case string:
		if strings.TrimSpace(value) == "" {
			return fallback
		}
		if dur, err := time.ParseDuration(value); err == nil {
			return dur
		}
		if seconds, err := strconv.Atoi(value); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return fallback
}

func GetAppName() string { return GetConfigString("app.name") }

func GetAppEnv() string { return GetConfigString("app.env") }

func GetServerAddr() string {
	if addr := strings.TrimSpace(GetConfigString("server.addr")); addr != "" {
		return addr
	}
	host := strings.TrimSpace(GetConfigString("server.host"))
	port := GetConfigInt("server.port")
	if host == "" && port == 0 {
		return ""
	}
	if host == "" {
		return fmt.Sprintf(":%d", port)
	}
	if port == 0 {
		return host
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func GetServerUCAddr() string { return GetConfigString("server.uc_addr") }

func GetCookieTokenName() string { return GetConfigString("security.cookie.name") }

func GetCookieTokenExpiredSeconds() int { return GetConfigInt("security.cookie.expired_seconds") }

func GetLocalBasePath() string { return GetConfigString("storage.local.base_path") }

func GetLocalRelativePathPrefix() string {
	return GetConfigString("storage.local.relative_path_prefix")
}

func GetLocalCleanupAfterUpload() bool { return GetConfigBool("storage.local.cleanup_after_upload") }
