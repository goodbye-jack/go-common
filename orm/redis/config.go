package redis

import (
	"github.com/goodbye-jack/go-common/orm/dbconfig"
	"github.com/goodbye-jack/go-common/utils"
	"time"
)

// Config 复用dbconfig.Config
//type Config = dbconfig.Config

type Config struct {
	dbconfig.Config // 嵌入基础配置，包含Addr/Password/Mode等字段
	// 补充Redis特有配置（若有）
	DB          int           // 数据库编号
	DialTimeout time.Duration // 连接超时
	ReadTimeout time.Duration // 读取超时
}

// DBType 快捷引用Redis类型
const DBType = utils.DBTypeRedis
