package config

import (
	"github.com/goodbye-jack/go-common/log"
	"github.com/spf13/viper"
)

func init() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/opt")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Fatalf("config.yaml not found in /opt/ or current dir")
		}

		log.Fatalf("ReadInConfig error, %v", err)
	}
}

func GetConfigString(name string) string {
	return viper.GetString(name)
}

func GetConfigInt(name string) int {
	return viper.GetInt(name)
}
