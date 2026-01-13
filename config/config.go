package config

import (
	"github.com/goodbye-jack/go-common/log"
	"github.com/spf13/viper"
	"os"
)

var configPaths = []string{".", "./config", "/opt"} // config配置读取顺序

// 这是测试值,
//var configPaths = []string{".", "./config", "/opt", "./example"}

func init() {
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
			log.Infof("读取并覆盖环境配置：%s", envViper.ConfigFileUsed())
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
		log.Init(serviceName) // 把项目名传给日志初始化函数
	}
}

// 保留原有读取方法
func GetConfigString(name string) string { return viper.GetString(name) }
func GetConfigInt(name string) int       { return viper.GetInt(name) }
