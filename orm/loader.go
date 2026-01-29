package orm

import (
	"context"
	"fmt"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/orm/dbconfig"
	"github.com/goodbye-jack/go-common/orm/mongodb"
	"github.com/goodbye-jack/go-common/orm/redis"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/spf13/viper"
	"strings"
)

// InitAllDB 总初始化入口（逻辑不变，仅替换Redis/Mongo的初始化函数）
func InitAllDB(v *viper.Viper) error {
	if !v.IsSet("databases") {
		log.Info("【数据库初始化】未配置databases节点，跳过所有数据库初始化")
		return nil
	}
	dbTypes := v.GetStringMap("databases")
	for dbType := range dbTypes {
		var err error
		switch dbType {
		// 关系型数据库
		case "mysql":
			err = initRelationalDB(v, utils.DBTypeMySQL)
		case "dm":
			err = initRelationalDB(v, utils.DBTypeDM)
		case "kingbase":
			err = initRelationalDB(v, utils.DBTypeKingBase)
		// NoSQL数据库（对齐关系型初始化逻辑）
		case "redis":
			err = initNoSQLDB(v, utils.DBTypeRedis)
		case "mongo":
			err = initNoSQLDB(v, utils.DBTypeMongo)
		default:
			log.Warnf("【数据库初始化】不支持的数据库类型：%s，跳过初始化", dbType)
			continue
		}
		if err != nil {
			return fmt.Errorf("【数据库初始化】%s初始化失败：%w", dbType, err)
		}
	}
	log.Info("【数据库初始化】所有配置的数据库初始化完成")
	return nil
}

// initRelationalDB 通用关系型数据库初始化（MySQL/DM/金仓等）
func initRelationalDB(v *viper.Viper, dbType utils.DBType) error {
	instanceKey := string(dbType)
	instances := v.GetStringMap(fmt.Sprintf("databases.%s", instanceKey))
	if len(instances) == 0 {
		log.Infof("【%s初始化】未配置任何%s实例，跳过", dbType, dbType)
		return nil
	}
	log.Infof("【%s初始化】开始初始化%d个实例：%v", dbType, len(instances), getInstanceNames(instances))
	for instanceName := range instances {
		// 1. 加载配置（逻辑不变）
		cfg, err := dbconfig.LoadDBConfig(v, fmt.Sprintf("%s.%s", instanceKey, instanceName))
		if err != nil {
			return fmt.Errorf("加载%s实例[%s]配置失败：%w", dbType, instanceName, err)
		}
		cfg.DBType = dbType
		// 2. 生成DSN（逻辑不变）
		dsn := cfg.GenDSN()
		if dsn == "" {
			return fmt.Errorf("%s实例[%s] DSN为空", dbType, instanceName)
		}
		// 3. 创建ORM实例（逻辑不变）
		slowTime := v.GetInt(fmt.Sprintf("databases.%s.%s.slow_time", instanceKey, instanceName))
		ormInstance := NewOrm(dsn, dbType, slowTime)
		if ormInstance == nil {
			return fmt.Errorf("%s实例[%s]创建ORM实例失败", dbType, instanceName)
		}
		// 4. 配置连接池（逻辑不变）
		sqlDB, err := ormInstance.DB()
		if err != nil {
			return fmt.Errorf("%s实例[%s]获取SQL DB失败：%w", dbType, instanceName, err)
		}
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConn)
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConn)
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifeTime)
		// 5. 测试连接（逻辑不变）
		if err := sqlDB.Ping(); err != nil {
			return fmt.Errorf("%s实例[%s] Ping失败：%w", dbType, instanceName, err)
		}
		// 6. 统一赋值：所有关系型数据库都存入RelationalMap，default实例赋值到DB
		RelationalMap[instanceName] = ormInstance
		if instanceName == "default" {
			DB = ormInstance // 无论MySQL/DM，default实例都赋值到全局DB
			log.Infof("【%s初始化】默认实例[default]已赋值到go-common全局orm.DB", dbType)
		}
		log.Infof("【%s初始化】实例[%s]初始化成功", dbType, instanceName)
	}
	return nil
}

// ---------------- NoSQL数据库通用初始化（Redis/Mongo 统一逻辑） ----------------
func initNoSQLDB(v *viper.Viper, dbType utils.DBType) error {
	instanceKey := string(dbType)
	instances := v.GetStringMap(fmt.Sprintf("databases.%s", instanceKey))
	if len(instances) == 0 {
		log.Infof("【%s初始化】未配置任何%s实例，跳过", dbType, dbType)
		return nil
	}
	log.Infof("【%s初始化】开始初始化%d个实例：%v", dbType, len(instances), getInstanceNames(instances))
	for instanceName := range instances {
		// 1. 加载配置
		cfg, err := dbconfig.LoadDBConfig(v, fmt.Sprintf("%s.%s", instanceKey, instanceName))
		if err != nil {
			return fmt.Errorf("加载%s实例[%s]配置失败：%w", dbType, instanceName, err)
		}
		cfg.DBType = dbType
		// 2. 生成DSN（复用你已有的GenDSN逻辑）
		dsn := cfg.GenDSN()
		if dsn == "" {
			return fmt.Errorf("%s实例[%s] DSN为空", dbType, instanceName)
		}
		// 3. 初始化超时时间（从配置读取，默认5秒）
		timeout := v.GetInt(fmt.Sprintf("databases.%s.%s.timeout", instanceKey, instanceName))
		if timeout <= 0 {
			timeout = 5
		}
		// 4. 创建NoSQL实例（根据类型调用你的NewRedis/NewMongo）
		var (
			redisInstance *redis.Redis
			mongoInstance *mongodb.Mongo
		)
		switch dbType {
		case utils.DBTypeRedis:
			redisInstance = redis.NewRedis(dsn, dbType, timeout, cfg)
			if redisInstance == nil {
				return fmt.Errorf("%s实例[%s]创建客户端失败", dbType, instanceName)
			}
		case utils.DBTypeMongo:
			mongoInstance = mongodb.NewMongo(dsn, dbType, timeout)
			if mongoInstance == nil {
				return fmt.Errorf("%s实例[%s]创建客户端失败", dbType, instanceName)
			}
		}
		// 5. 赋值到全局变量/映射（对齐关系型逻辑）
		switch dbType {
		case utils.DBTypeRedis:
			RedisMap[instanceName] = redisInstance
			if instanceName == "default" {
				Redis = redisInstance
				log.Infof("【Redis初始化】默认实例[default]已赋值到go-common全局orm.Redis")
			}
		case utils.DBTypeMongo:
			MongoMap[instanceName] = mongoInstance
			if instanceName == "default" {
				Mongo = mongoInstance
				log.Infof("【Mongo初始化】默认实例[default]已赋值到go-common全局orm.Mongo")
			}
		}
		log.Infof("【%s初始化】实例[%s]初始化成功", dbType, instanceName)
	}
	return nil
}

// ---------------- 工具函数 ----------------
// genMongoURI 拼接Mongo单点连接串（集群模式优先用DSN）
func genMongoURI(cfg *dbconfig.Config) string {
	var base string
	if cfg.User != "" && cfg.Password != "" {
		base = fmt.Sprintf("mongodb://%s:%s@%s:%d/", cfg.User, cfg.Password, cfg.Host, cfg.Port)
	} else {
		base = fmt.Sprintf("mongodb://%s:%d/", cfg.Host, cfg.Port)
	}
	var params []string
	if cfg.AuthDB != "" {
		params = append(params, fmt.Sprintf("authSource=%s", cfg.AuthDB))
	}
	if cfg.SSLMode != "" {
		params = append(params, fmt.Sprintf("ssl=%s", cfg.SSLMode))
	}

	if len(params) > 0 {
		return base + cfg.Database + "?" + strings.Join(params, "&")
	}
	return base + cfg.Database
}

// getInstanceNames 提取实例名列表（用于日志输出）
func getInstanceNames(instances map[string]interface{}) []string {
	names := make([]string, 0, len(instances))
	for name := range instances {
		names = append(names, name)
	}
	return names
}

// GetContext 获取通用上下文（兼容go-common的上下文逻辑）
func GetContext() context.Context {
	// 若go-common有全局上下文，可替换为config.GetContext()或其他全局上下文
	return context.Background()
}
