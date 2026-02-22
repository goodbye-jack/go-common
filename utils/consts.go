package utils

import "time"

const (
	RoleIdle             = ""
	RoleManager          = "manager"
	RoleAdministrator    = "ADMINISTRATOR_ROLE"     //管理员角色
	RoleDefault          = "DEFAULT_ROLE"           //默认角色
	RoleAppraisalStation = "APPRAISAL_STATION_ROLE" // 鉴定站角色
	RoleMuseum           = "MUSEUM_ROLE"            // 博物馆角色
	RoleMuseumOffice     = "MUSEUM_OFFICE_ROLE"     // 博物馆处角色
	UserAnonymous        = "anonymous"              //not login
	TenantAnonymous      = ""
	TenantContextName    = "ContextTenant"
	TenantHeaderName     = "X-Tenant"
	CasbinRedisAddrName  = "redis_addr"
	JWTSecret            = "goodbye-jack,comeon"
	ConfigNameDomain     = "cookie_domain_name"
	ConfigNameToken      = "cookie_token"
	// 统一登录校验是否开启
	SsoEnabledVerify       = "sso_enable_verify"
	SsoVerifyHandlerName   = "sso_verify_handler_name"
	ConfigNameTokenExpired = "cookie_token_expired_seconds"
)

const (
	DefaultDBNameMaster = "__default_master__"
	DefaultDBNameSlave  = "__default_slave__"
	DBNameMock          = "__mock__"
)

// 新增：数据库运行模式（单点/集群）
type DBMode string

const (
	DBModeSingle  DBMode = "single"  // 单点
	DBModeCluster DBMode = "cluster" // 集群
)

type LogMode string

const (
	LogModeSilent LogMode = "silent"
	LogModeInfo   LogMode = "info"
	LogModeWarn   LogMode = "warn"
	LogModeError  LogMode = "error"
)

type DBType string

const (
	DBTypePostgres  DBType = "postgres"
	DBTypeMySQL     DBType = "mysql"
	DBTypeSqlserver DBType = "sqlserver"
	DBTypeOracle    DBType = "oracle"
	DBTypeSQLite    DBType = "sqlite"
	DBTypeDM        DBType = "dm"
	DBTypeKingBase  DBType = "kingbase"
	DBTypeMongo     DBType = "mongo"
	DBTypeMongoDB   DBType = "mongodb"
	DBTypeRedis     DBType = "redis"
)

// DBDsnMap 关系型数据库类型  username、password、address、port、dbname
var DBDsnMap = map[DBType]string{
	DBTypeSQLite:    "%s",
	DBTypeDM:        "dm://%s:%s@%s:%d?schema=%s",
	DBTypeOracle:    "%s/%s@%s:%d/%s",
	DBTypeMySQL:     "%s:%s@tcp(%s:%d)/%s?parseTime=True&loc=Local",
	DBTypePostgres:  "user=%s password=%s host=%s port=%d dbname=%s sslmode=disable TimeZone=Asia/Shanghai",
	DBTypeSqlserver: "user id=%s;password=%s;server=%s;port=%d;database=%s;encrypt=disable",
	DBTypeMongo:     "mongodb://%s:%s@%s:%d/%s?maxPoolSize=%d&minPoolSize=%d&connectTimeoutMS=%d",
	DBTypeRedis:     "redis://%s%s@%s:%d/%d?dial_timeout=%ds&read_timeout=%ds&write_timeout=%ds",
	//DBTypeRedis:     "redis://%s:%s@%s:%d/%d?dial_timeout=%ds&read_timeout=%ds&write_timeout=%ds",
}

// 各类型数据库的默认值常量（统一管理）
const (
	// MySQL默认值
	DefaultMySQLMaxOpenConn     = 100
	DefaultMySQLMaxIdleConn     = 10
	DefaultMySQLConnMaxLifeTime = 5 * time.Minute
	// Mongo默认值
	DefaultMongoMaxPoolSize    = 20
	DefaultMongoMinPoolSize    = 5
	DefaultMongoConnectTimeout = 5 * time.Second
	// Redis默认值
	DefaultRedisMaxPoolSize    = 20
	DefaultRedisMinPoolSize    = 5
	DefaultRedisConnectTimeout = 5 * time.Second
	DefaultRedisReadTimeout    = 3 * time.Second
	DefaultRedisWriteTimeout   = 3 * time.Second
	DefaultRedisDBIndex        = 0
)
