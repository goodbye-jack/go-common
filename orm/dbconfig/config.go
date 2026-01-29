package dbconfig

import (
	"errors"
	"fmt"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/spf13/viper"
	gormLogger "gorm.io/gorm/logger"
	"strings"
	"time"
)

var defaultConfig = &Config{
	// 关系型
	MaxOpenConn:     100,             // 最大连接数
	MaxIdleConn:     10,              // 最大空闲连接数
	ConnMaxLifeTime: 5 * time.Minute, // 连接最大存活时间
	// 非关系型默认配置（新增）
	MaxPoolSize:    50,              // Mongo/Redis最大连接池
	MinPoolSize:    10,              // Mongo最小连接池
	ConnectTimeout: 5 * time.Second, // 连接超时
	DBIndex:        0,               // Redis数据库索引
	ReadTimeout:    3 * time.Second, // Redis读超时
	WriteTimeout:   3 * time.Second, // Redis写超时
}

// Config DB configuration
type Config struct {
	// 基础通用字段
	DBName   string        `json:"db_name" yaml:"db_name"`
	DBType   utils.DBType  `json:"db_type" yaml:"db_type"`
	Mode     utils.DBMode  `json:"mode" yaml:"mode"`     // 新增：单点/集群标识
	Schema   string        `json:"schema" yaml:"schema"` // DM模式（适配DM的schema参数）
	Host     string        `json:"host" yaml:"host"`
	Port     int           `json:"port" yaml:"port"`
	User     string        `json:"user" yaml:"user"`
	Password string        `json:"password" yaml:"password"`
	Database string        `json:"database" yaml:"database"`
	DSN      string        `json:"dsn" yaml:"dsn"`
	LogMode  utils.LogMode `json:"log_mode" yaml:"log_mode"`
	SSLMode  string        `json:"ssl_mode" yaml:"ssl_mode"`
	SSL      bool          `json:"ssl" yaml:"ssl"` // Mongo专用SSL开关（true/false）
	TimeZone string        `json:"time_zone" yaml:"time_zone"`
	Charset  string        `json:"charset" yaml:"charset"`
	// 关系型数据库专属字段
	MaxOpenConn     int           `json:"max_open_conn" yaml:"max_open_conn"`
	MaxIdleConn     int           `json:"max_idle_conn" yaml:"max_idle_conn"`
	ConnMaxLifeTime time.Duration `json:"conn_max_life_time" yaml:"conn_max_life_time"`
	// 非关系型数据库专属字段
	MaxPoolSize    int           `json:"max_pool_size" yaml:"max_pool_size"`
	MinPoolSize    int           `json:"min_pool_size" yaml:"min_pool_size"`
	ConnectTimeout time.Duration `json:"connect_timeout" yaml:"connect_timeout"`
	DBIndex        int           `json:"db_index" yaml:"db_index"` // Redis DB索引
	ReadTimeout    time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout   time.Duration `json:"write_timeout" yaml:"write_timeout"`
	AuthDB         string        `json:"auth_db" yaml:"auth_db"` // Mongo认证库
}

// GetLogMode _
func (c *Config) GetLogMode() gormLogger.LogLevel {
	switch c.LogMode {
	case utils.LogModeInfo:
		return gormLogger.Info
	case utils.LogModeWarn:
		return gormLogger.Warn
	case utils.LogModeError:
		return gormLogger.Error
	case utils.LogModeSilent:
		return gormLogger.Silent
	}
	return gormLogger.Error
}

// GenDSN 生成DSN（复用DBDsnMap模板 + 默认值常量，逻辑简洁）
func (c *Config) GenDSN() string {
	if c.DSN != "" { // 1. 自定义DSN优先级最高
		return c.DSN
	}
	switch c.DBType { // 2. 按数据库类型分层处理（复用DBDsnMap模板）
	case utils.DBTypeMySQL, utils.DBTypePostgres, utils.DBTypeSqlserver, utils.DBTypeOracle, utils.DBTypeDM, utils.DBTypeSQLite:
		return c.genRelationalDSN() // 关系型数据库
	case utils.DBTypeMongo, utils.DBTypeMongoDB:
		return c.genMongoDSN() // Mongo（兼容MongoDB别名）
	case utils.DBTypeRedis:
		return c.genRedisDSN() // Redis
	default:
		return fmt.Sprintf("unsupported db type: %s", c.DBType)
	}
}

// genRelationalDSN 生成关系型数据库DSN（复用DBDsnMap模板）
func (c *Config) genRelationalDSN() string {
	// 获取对应类型的DSN模板
	template, ok := utils.DBDsnMap[c.DBType]
	if !ok {
		return fmt.Sprintf("no dsn template for db type: %s", c.DBType)
	}
	// 按模板参数拼接（适配不同数据库的参数顺序）
	switch c.DBType {
	case utils.DBTypeSQLite:
		return fmt.Sprintf(template, c.Database) // SQLite仅需数据库路径
	case utils.DBTypeDM:
		// DM模板：dm://%s:%s@%s:%d?schema=%s → user/pass/host/port/schema
		schema := c.Schema
		if schema == "" {
			schema = c.Database // 未指定schema时用数据库名
		}
		return fmt.Sprintf(template, c.User, c.Password, c.Host, c.Port, schema)
	case utils.DBTypeOracle:
		// Oracle模板：%s/%s@%s:%d/%s → user/pass/host/port/dbname
		return fmt.Sprintf(template, c.User, c.Password, c.Host, c.Port, c.Database)
	case utils.DBTypeMySQL:
		// MySQL模板：%s:%s@tcp(%s:%d)/%s?parseTime=True&loc=Local → user/pass/host/port/dbname
		return fmt.Sprintf(template, c.User, c.Password, c.Host, c.Port, c.Database)
	case utils.DBTypePostgres:
		// Postgres模板：user=%s password=%s host=%s port=%d dbname=%s ... → user/pass/host/port/dbname
		return fmt.Sprintf(template, c.User, c.Password, c.Host, c.Port, c.Database)
	case utils.DBTypeSqlserver:
		// SQLServer模板：user id=%s;password=%s;server=%s;port=%d;database=%s ... → user/pass/host/port/dbname
		return fmt.Sprintf(template, c.User, c.Password, c.Host, c.Port, c.Database)
	default:
		return ""
	}
}

// genMongoDSN 生成Mongo DSN（最终版：仅用SSL布尔字段）
func (c *Config) genMongoDSN() string {
	// 1. 获取Mongo的DSN模板
	template, ok := utils.DBDsnMap[c.DBType]
	if !ok {
		template = utils.DBDsnMap[utils.DBTypeMongo] // 兼容DBTypeMongoDB别名
	}
	// 2. 填充默认值（复用你定义的常量）
	maxPool := c.MaxPoolSize
	if maxPool <= 0 {
		maxPool = utils.DefaultMongoMaxPoolSize
	}
	minPool := c.MinPoolSize
	if minPool <= 0 {
		minPool = utils.DefaultMongoMinPoolSize
	}
	connectTimeoutMS := int(c.ConnectTimeout.Milliseconds())
	if connectTimeoutMS <= 0 {
		connectTimeoutMS = int(utils.DefaultMongoConnectTimeout.Milliseconds())
	}
	// 3. 区分有/无密码场景拼接DSN
	var dsn string
	if c.User == "" && c.Password == "" {
		// 无密码：mongodb://host:port/database?xxx
		dsn = fmt.Sprintf("mongodb://%s:%d/%s?maxPoolSize=%d&minPoolSize=%d&connectTimeoutMS=%d",
			c.Host, c.Port, c.Database, maxPool, minPool, connectTimeoutMS,
		)
	} else {
		// 有密码：复用原有模板
		dsn = fmt.Sprintf(template,
			c.User, c.Password, c.Host, c.Port, c.Database,
			maxPool, minPool, connectTimeoutMS,
		)
	}

	// 4. 扩展参数（仅处理Mongo专用的SSL和AuthDB）
	params := []string{}
	// AuthDB（认证库）
	if c.AuthDB != "" {
		params = append(params, fmt.Sprintf("authSource=%s", c.AuthDB))
	}
	// SSL（布尔值，仅Mongo用）：配置了才拼接，空则不拼
	params = append(params, fmt.Sprintf("ssl=%t", c.SSL)) // 直接转布尔字符串（true/false）
	// 拼接扩展参数
	if len(params) > 0 {
		dsn += "&" + strings.Join(params, "&")
	}
	// 5. 集群模式适配（Host为逗号分隔的节点列表）
	if c.Mode == utils.DBModeCluster && strings.Contains(c.Host, ",") {
		if c.User == "" && c.Password == "" {
			dsn = fmt.Sprintf("mongodb://%s/%s?maxPoolSize=%d&minPoolSize=%d&connectTimeoutMS=%d",
				c.Host, c.Database, maxPool, minPool, connectTimeoutMS,
			)
		} else {
			dsn = strings.Replace(dsn, fmt.Sprintf("@%s:%d", c.Host, c.Port), "@"+c.Host, 1)
		}
		// 集群模式补充扩展参数
		if len(params) > 0 {
			dsn += "&" + strings.Join(params, "&")
		}
	}
	return dsn
}

// genRedisDSN 生成Redis DSN（修复：区分有无密码 + 集群适配）
func (c *Config) genRedisDSN() string {
	// 1. 填充默认值
	dbIndex := utils.DefaultRedisDBIndex
	if c.Database != "" {
		fmt.Sscanf(c.Database, "%d", &dbIndex)
	}
	dialTimeout := int(c.ConnectTimeout.Seconds())
	if dialTimeout <= 0 {
		dialTimeout = int(utils.DefaultRedisConnectTimeout.Seconds())
	}
	readTimeout := int(c.ReadTimeout.Seconds())
	if readTimeout <= 0 {
		readTimeout = int(utils.DefaultRedisReadTimeout.Seconds())
	}
	writeTimeout := int(c.WriteTimeout.Seconds())
	if writeTimeout <= 0 {
		writeTimeout = int(utils.DefaultRedisWriteTimeout.Seconds())
	}
	// 2. 区分有无密码（核心修复）
	var authPart string
	if c.Password != "" {
		// 有密码：:password（Redis User通常为空，无需拼接）
		authPart = ":" + c.Password
	}
	// 无密码：空字符串（避免多余的:@）
	// 3. 单机模式DSN
	dsn := fmt.Sprintf(utils.DBDsnMap[utils.DBTypeRedis],
		c.User, authPart, c.Host, c.Port, dbIndex,
		dialTimeout, readTimeout, writeTimeout,
	)
	// 4. 集群模式适配（修复密码拼接）
	if c.Mode == utils.DBModeCluster && strings.Contains(c.Host, ",") {
		if c.Password != "" {
			dsn = fmt.Sprintf("redis://:%s@%s?db=%d&dial_timeout=%ds&read_timeout=%ds&write_timeout=%ds",
				c.Password, c.Host, dbIndex, dialTimeout, readTimeout, writeTimeout,
			)
		} else {
			dsn = fmt.Sprintf("redis://%s?db=%d&dial_timeout=%ds&read_timeout=%ds&write_timeout=%ds",
				c.Host, dbIndex, dialTimeout, readTimeout, writeTimeout,
			)
		}
	}
	return dsn
}

//// genRedisDSN 生成Redis DSN（复用DBDsnMap模板 + 默认值常量）
//func (c *Config) genRedisDSN() string {
//	template := DBDsnMap[DBTypeRedis] // 复用Redis模板
//
//	// 填充默认值（未配置时用常量）
//	dbIndex := DefaultRedisDBIndex
//	if c.Database != "" {
//		// 兼容字符串转数字（Redis DB索引是int）
//		fmt.Sscanf(c.Database, "%d", &dbIndex)
//	}
//	dialTimeout := int(c.ConnectTimeout.Seconds())
//	if dialTimeout <= 0 {
//		dialTimeout = int(DefaultRedisConnectTimeout.Seconds())
//	}
//	readTimeout := int(c.ReadTimeout.Seconds())
//	if readTimeout <= 0 {
//		readTimeout = int(DefaultRedisReadTimeout.Seconds())
//	}
//	writeTimeout := int(c.WriteTimeout.Seconds())
//	if writeTimeout <= 0 {
//		writeTimeout = int(DefaultRedisWriteTimeout.Seconds())
//	}
//
//	// 基础DSN拼接（模板：redis://%s:%s@%s:%d/%d?dial_timeout=%ds&read_timeout=%ds&write_timeout=%ds）
//	dsn := fmt.Sprintf(template,
//		c.User, c.Password, c.Host, c.Port, dbIndex,
//		dialTimeout, readTimeout, writeTimeout,
//	)
//	// 集群/哨兵模式适配
//	if c.Mode == DBModeCluster && strings.Contains(c.Host, ",") {
//		// 集群模式：替换为redis集群DSN格式
//		dsn = fmt.Sprintf("redis://%s?db=%d&dial_timeout=%ds&read_timeout=%ds&write_timeout=%ds",
//			c.Host, dbIndex, dialTimeout, readTimeout, writeTimeout,
//		)
//		if c.Password != "" {
//			dsn = fmt.Sprintf("redis://:%s@%s?db=%d&dial_timeout=%ds&read_timeout=%ds&write_timeout=%ds",
//				c.Password, c.Host, dbIndex, dialTimeout, readTimeout, writeTimeout,
//			)
//		}
//	}
//	return dsn
//}

// 定义各类型数据库的必填字段规则
var requiredFields = map[utils.DBType][]string{
	// 关系型
	utils.DBTypeMySQL: {
		"user", "password", "host", "port", "database", // 单点必填
		// 集群模式下DSN必填，无需host/port
	},
	utils.DBTypeDM: {
		"user", "password", "host", "port", "database",
	},
	// 非关系型
	utils.DBTypeMongo: {
		"user", "password", "database", // 通用必填
		// 单点需host/port，集群需DSN
	},
	utils.DBTypeRedis: {
		"password", "database", // 通用必填
		// 单点需host/port，集群需DSN
	},
}

// 移除原有对 "github.com/goodbye-jack/go-common/config" 的导入

// LoadDBConfig 重构：接收viper实例 + dbKey，不再直接调用config.GetConfigXXX
// 参数说明：
//
//	v: viper实例（由外部传入，避免直接依赖config模块）
//	dbKey: 格式如 "mysql.master"
func LoadDBConfig(v *viper.Viper, dbKey string) (*Config, error) {
	// 解析dbKey：如"mysql.master" → type=mysql, instance=master
	parts := splitDBKey(dbKey)
	if len(parts) != 2 {
		return nil, errors.New("dbKey格式错误，应为${dbType}.${instanceName}（如mysql.master）")
	}
	dbType := utils.DBType(parts[0])
	instanceName := parts[1]
	// 构造配置路径：databases.${dbType}.${instanceName}.xxx
	prefix := fmt.Sprintf("databases.%s.%s", dbType, instanceName)
	cfg := &Config{
		// 基础默认值（通用）
		DBType:   dbType,
		DBName:   instanceName,
		LogMode:  utils.LogModeError,
		SSLMode:  "disable",
		TimeZone: "Asia/Shanghai",
		Charset:  "utf8mb4",
	}
	// 1. 按类型设置默认值
	setDefaultValuesByType(cfg)
	// 2. 读取配置字段（从传入的viper实例读取，不再调用config.GetConfigXXX）
	readConfigFields(v, prefix, cfg)
	// 3. 校验必填字段
	if err := ValidateRequiredFields(cfg); err != nil {
		return nil, fmt.Errorf("必填字段校验失败：%w", err)
	}
	return cfg, nil
}

// 重构readConfigFields：接收viper实例作为参数
func readConfigFields(v *viper.Viper, prefix string, cfg *Config) {
	// 基础字段（全部改为从v.GetXXX读取）
	if v.IsSet(prefix + ".dsn") {
		cfg.DSN = v.GetString(prefix + ".dsn")
	}
	if v.IsSet(prefix + ".mode") {
		cfg.Mode = utils.DBMode(v.GetString(prefix + ".mode"))
	}
	if v.IsSet(prefix + ".host") {
		cfg.Host = v.GetString(prefix + ".host")
	}
	if v.IsSet(prefix + ".port") {
		cfg.Port = v.GetInt(prefix + ".port")
	}
	if v.IsSet(prefix + ".user") {
		cfg.User = v.GetString(prefix + ".user")
	}
	if v.IsSet(prefix + ".password") {
		cfg.Password = v.GetString(prefix + ".password")
	}
	if v.IsSet(prefix + ".database") {
		cfg.Database = v.GetString(prefix + ".database")
	}
	if v.IsSet(prefix + ".log_mode") {
		cfg.LogMode = utils.LogMode(v.GetString(prefix + ".log_mode"))
	}
	if v.IsSet(prefix + ".ssl_mode") {
		cfg.SSLMode = v.GetString(prefix + ".ssl_mode")
	}
	if v.IsSet(prefix + ".time_zone") {
		cfg.TimeZone = v.GetString(prefix + ".time_zone")
	}
	if v.IsSet(prefix + ".charset") {
		cfg.Charset = v.GetString(prefix + ".charset")
	}

	// 关系型专属字段
	if v.IsSet(prefix + ".max_open_conn") {
		cfg.MaxOpenConn = v.GetInt(prefix + ".max_open_conn")
	}
	if v.IsSet(prefix + ".max_idle_conn") {
		cfg.MaxIdleConn = v.GetInt(prefix + ".max_idle_conn")
	}
	if v.IsSet(prefix + ".conn_max_life_time") {
		dur, err := time.ParseDuration(v.GetString(prefix + ".conn_max_life_time"))
		if err == nil {
			cfg.ConnMaxLifeTime = dur
		}
	}

	// 非关系型专属字段
	if v.IsSet(prefix + ".max_pool_size") {
		cfg.MaxPoolSize = v.GetInt(prefix + ".max_pool_size")
	}
	if v.IsSet(prefix + ".min_pool_size") {
		cfg.MinPoolSize = v.GetInt(prefix + ".min_pool_size")
	}
	if v.IsSet(prefix + ".connect_timeout") {
		dur, err := time.ParseDuration(v.GetString(prefix + ".connect_timeout"))
		if err == nil {
			cfg.ConnectTimeout = dur
		}
	}
	if v.IsSet(prefix + ".db_index") {
		cfg.DBIndex = v.GetInt(prefix + ".db_index")
	}
	if v.IsSet(prefix + ".read_timeout") {
		dur, err := time.ParseDuration(v.GetString(prefix + ".read_timeout"))
		if err == nil {
			cfg.ReadTimeout = dur
		}
	}
	if v.IsSet(prefix + ".write_timeout") {
		dur, err := time.ParseDuration(v.GetString(prefix + ".write_timeout"))
		if err == nil {
			cfg.WriteTimeout = dur
		}
	}
	if v.IsSet(prefix + ".auth_db") {
		cfg.AuthDB = v.GetString(prefix + ".auth_db")
	}
}

// splitDBKey 拆分dbKey为类型+实例名
func splitDBKey(dbKey string) []string {
	sep := -1
	for i, c := range dbKey {
		if c == '.' {
			sep = i
			break
		}
	}
	if sep == -1 {
		return nil
	}
	return []string{dbKey[:sep], dbKey[sep+1:]}
}

// setDefaultValuesByType 按数据库类型设置默认值
func setDefaultValuesByType(cfg *Config) {
	switch cfg.DBType {
	// 关系型：MySQL/达梦等
	case utils.DBTypeMySQL, utils.DBTypeDM, utils.DBTypePostgres:
		cfg.MaxOpenConn = utils.DefaultMySQLMaxOpenConn
		cfg.MaxIdleConn = utils.DefaultMySQLMaxIdleConn
		cfg.ConnMaxLifeTime = utils.DefaultMySQLConnMaxLifeTime
	// 非关系型：Mongo
	case utils.DBTypeMongo:
		cfg.MaxPoolSize = utils.DefaultMongoMaxPoolSize
		cfg.MinPoolSize = utils.DefaultMongoMinPoolSize
		cfg.ConnectTimeout = utils.DefaultMongoConnectTimeout
	// 非关系型：Redis
	case utils.DBTypeRedis:
		cfg.MaxPoolSize = utils.DefaultRedisMaxPoolSize
		cfg.MinPoolSize = utils.DefaultRedisMinPoolSize
		cfg.ConnectTimeout = utils.DefaultRedisConnectTimeout
		cfg.ReadTimeout = utils.DefaultRedisReadTimeout
		cfg.WriteTimeout = utils.DefaultRedisWriteTimeout
		cfg.DBIndex = utils.DefaultRedisDBIndex
	}
}

// ValidateRequiredFields 校验数据库配置的必填字段（完整版）
// 核心适配：Mongo仅在配置auth_db时才校验user/password，本地无鉴权场景跳过
func ValidateRequiredFields(cfg *Config) error {
	if cfg == nil {
		return errors.New("配置结构体不能为空")
	}
	// 按数据库类型区分校验规则
	switch cfg.DBType {
	// ========== 1. Mongo/MongoDB 校验（核心修改） ==========
	case utils.DBTypeMongo, utils.DBTypeMongoDB:
		requiredFields := []string{}
		// 通用必填：数据库名（无论模式）
		if cfg.Database == "" {
			requiredFields = append(requiredFields, "database")
		}
		// 按运行模式细分校验
		switch cfg.Mode {
		case utils.DBModeSingle: // 单点模式
			// 单点模式必填：host/port
			if cfg.Host == "" {
				requiredFields = append(requiredFields, "host")
			}
			if cfg.Port == 0 {
				requiredFields = append(requiredFields, "port")
			}
			// 关键适配：仅当配置了auth_db（需要认证）时，才校验user/password
			if cfg.AuthDB != "" {
				if cfg.User == "" {
					requiredFields = append(requiredFields, "user")
				}
				if cfg.Password == "" {
					requiredFields = append(requiredFields, "password")
				}
			}
		case utils.DBModeCluster: // 集群模式
			// 集群模式必填：host（节点列表，逗号分隔）
			if cfg.Host == "" {
				requiredFields = append(requiredFields, "host")
			}
			// 集群模式有认证时才校验user/password
			if cfg.AuthDB != "" {
				if cfg.User == "" {
					requiredFields = append(requiredFields, "user")
				}
				if cfg.Password == "" {
					requiredFields = append(requiredFields, "password")
				}
			}
		default:
			return fmt.Errorf("Mongo不支持的运行模式：%s（仅支持single/cluster）", cfg.Mode)
		}
		// 返回缺失的必填字段
		if len(requiredFields) > 0 {
			return fmt.Errorf("%s模式下缺失必填字段：[%s]", cfg.Mode, strings.Join(requiredFields, " "))
		}
	// ========== 2. Redis 校验 ==========
	case utils.DBTypeRedis:
		requiredFields := []string{}
		switch cfg.Mode {
		case utils.DBModeSingle: // 单点模式
			if cfg.Host == "" {
				requiredFields = append(requiredFields, "host")
			}
			if cfg.Port == 0 {
				requiredFields = append(requiredFields, "port")
			}
			// Redis DB索引默认0，非必填（可留空）
		case utils.DBModeCluster: // 集群模式
			if cfg.Host == "" {
				requiredFields = append(requiredFields, "host") // 集群节点列表，逗号分隔
			}
		case "sentinel": // 哨兵模式（扩展）
			if cfg.Host == "" {
				requiredFields = append(requiredFields, "host") // 哨兵节点列表
			}
			if cfg.User == "" {
				requiredFields = append(requiredFields, "user") // 主节点名
			}
		default:
			return fmt.Errorf("Redis不支持的运行模式：%s（仅支持single/cluster/sentinel）", cfg.Mode)
		}

		if len(requiredFields) > 0 {
			return fmt.Errorf("%s模式下缺失必填字段：[%s]", cfg.Mode, strings.Join(requiredFields, " "))
		}

	// ========== 3. MySQL 校验 ==========
	case utils.DBTypeMySQL:
		requiredFields := []string{}
		switch cfg.Mode {
		case utils.DBModeSingle:
			if cfg.Host == "" {
				requiredFields = append(requiredFields, "host")
			}
			if cfg.Port == 0 {
				requiredFields = append(requiredFields, "port")
			}
			if cfg.User == "" {
				requiredFields = append(requiredFields, "user")
			}
			if cfg.Password == "" {
				requiredFields = append(requiredFields, "password")
			}
			if cfg.Database == "" {
				requiredFields = append(requiredFields, "database")
			}
		case utils.DBModeCluster:
			if cfg.Host == "" {
				requiredFields = append(requiredFields, "host") // 集群节点列表
			}
			if cfg.User == "" {
				requiredFields = append(requiredFields, "user")
			}
			if cfg.Password == "" {
				requiredFields = append(requiredFields, "password")
			}
			if cfg.Database == "" {
				requiredFields = append(requiredFields, "database")
			}
		default:
			return fmt.Errorf("MySQL不支持的运行模式：%s（仅支持single/cluster）", cfg.Mode)
		}
		if len(requiredFields) > 0 {
			return fmt.Errorf("%s模式下缺失必填字段：[%s]", cfg.Mode, strings.Join(requiredFields, " "))
		}
	// ========== 4. DM（达梦）校验 ==========
	case utils.DBTypeDM:
		requiredFields := []string{}
		switch cfg.Mode {
		case utils.DBModeSingle:
			if cfg.Host == "" {
				requiredFields = append(requiredFields, "host")
			}
			if cfg.Port == 0 {
				requiredFields = append(requiredFields, "port")
			}
			if cfg.User == "" {
				requiredFields = append(requiredFields, "user")
			}
			if cfg.Password == "" {
				requiredFields = append(requiredFields, "password")
			}
			if cfg.Schema == "" { // DM用schema替代database
				requiredFields = append(requiredFields, "schema")
			}
		default:
			return fmt.Errorf("DM不支持的运行模式：%s（仅支持single）", cfg.Mode)
		}
		if len(requiredFields) > 0 {
			return fmt.Errorf("%s模式下缺失必填字段：[%s]", cfg.Mode, strings.Join(requiredFields, " "))
		}
	// ========== 5. 其他关系型数据库校验 ==========
	case utils.DBTypePostgres, utils.DBTypeSqlserver, utils.DBTypeOracle, utils.DBTypeKingBase:
		requiredFields := []string{}
		switch cfg.Mode {
		case utils.DBModeSingle:
			if cfg.Host == "" {
				requiredFields = append(requiredFields, "host")
			}
			if cfg.Port == 0 {
				requiredFields = append(requiredFields, "port")
			}
			if cfg.User == "" {
				requiredFields = append(requiredFields, "user")
			}
			if cfg.Password == "" {
				requiredFields = append(requiredFields, "password")
			}
			if cfg.Database == "" {
				requiredFields = append(requiredFields, "database")
			}
		default:
			return fmt.Errorf("%s不支持的运行模式：%s（仅支持single）", cfg.DBType, cfg.Mode)
		}
		if len(requiredFields) > 0 {
			return fmt.Errorf("%s模式下缺失必填字段：[%s]", cfg.Mode, strings.Join(requiredFields, " "))
		}
	// ========== 6. SQLite 校验（特殊：仅需数据库路径） ==========
	case utils.DBTypeSQLite:
		if cfg.Database == "" {
			return errors.New("SQLite缺失必填字段：database（文件路径）")
		}
	// ========== 7. 未支持的数据库类型 ==========
	default:
		return fmt.Errorf("暂不支持的数据库类型：%s", cfg.DBType)
	}
	// 所有校验通过
	return nil
}
