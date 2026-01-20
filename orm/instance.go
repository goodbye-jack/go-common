package orm

import (
	"github.com/goodbye-jack/go-common/orm/mongodb"
	"github.com/goodbye-jack/go-common/orm/redis"
)

// ========== 全局默认实例（业务唯一调用入口） ==========
var (
	DB    *Orm           // 关系型数据库（MySQL/DM/金仓等）
	Redis *redis.Redis   // Redis客户端（封装后的类型）
	Mongo *mongodb.Mongo // Mongo客户端（封装后的类型）
)

// ========== 多实例映射（特殊场景用） ==========
var (
	RelationalMap = make(map[string]*Orm)           // 关系型多实例
	RedisMap      = make(map[string]*redis.Redis)   // Redis多实例
	MongoMap      = make(map[string]*mongodb.Mongo) // Mongo多实例
)

// ========== 通用调用方法 ==========
func GetDB(instanceName string) *Orm {
	return RelationalMap[instanceName]
}

func GetRedis(instanceName string) *redis.Redis {
	return RedisMap[instanceName]
}

func GetMongo(instanceName string) *mongodb.Mongo {
	return MongoMap[instanceName]
}
