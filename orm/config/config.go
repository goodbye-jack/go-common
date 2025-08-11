package config

import (
	"fmt"
	gormLogger "gorm.io/gorm/logger"
	"time"
)

var defaultConfig = &Config{
	MaxOpenConn:     100,             // 最大连接数
	MaxIdleConn:     10,              // 最大空闲连接数
	ConnMaxLifeTime: 5 * time.Minute, // 连接最大存活时间
}

// Config DB configuration
type Config struct {
	DBName   string `json:"db_name"` // 数据库名称（要求唯一）
	DSN      string `json:"dsn"`     // 数据库连接串
	DBType   DBType `json:"db_type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`

	MaxOpenConn     int           `json:"max_open_conn"`      // 最大连接数
	MaxIdleConn     int           `json:"max_idle_conn"`      // 最大空闲连接数
	ConnMaxLifeTime time.Duration `json:"conn_max_life_time"` // 连接最大存活时间
	LogMode         LogMode       `json:"log_mode"`           // 日志级别
	SSLMode         string        `json:"ssl_mode"`
	TimeZone        string        `json:"time_zone"`
	Charset         string        `json:"charset"`
}

// GetLogMode _
func (c *Config) GetLogMode() gormLogger.LogLevel {
	switch c.LogMode {
	case LogModeInfo:
		return gormLogger.Info
	case LogModeWarn:
		return gormLogger.Warn
	case LogModeError:
		return gormLogger.Error
	case LogModeSilent:
		return gormLogger.Silent
	}
	return gormLogger.Error
}

func (c *Config) GenDSN() (dsn string) {
	if c.DSN != "" {
		return c.DSN
	}

	switch c.DBType {
	case DBTypeSQLite:
		//dsn = c.DSN
	default:
		dbName := c.DBName
		if dbName == "" {
			dbName = c.DBName
		}
		dsnTemplate, ok := DBDsnMap[c.DBType]
		if !ok {
			return
		}
		c.DSN = fmt.Sprintf(dsnTemplate, c.User, c.Password, c.Host, c.Port, dbName)
	}
	dsn = c.DSN
	return
}
