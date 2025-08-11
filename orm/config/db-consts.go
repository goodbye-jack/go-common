package config

const (
	DefaultDBNameMaster = "__default_master__"
	DefaultDBNameSlave  = "__default_slave__"
	DBNameMock          = "__mock__"
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
)

// DBDsnMap 关系型数据库类型  username、password、address、port、dbname
var DBDsnMap = map[DBType]string{
	DBTypeSQLite:    "%s",
	DBTypeDM:        "dm://%s:%s@%s:%d?schema=%s",
	DBTypeOracle:    "%s/%s@%s:%d/%s",
	DBTypeMySQL:     "%s:%s@tcp(%s:%d)/%s?parseTime=True&loc=Local",
	DBTypePostgres:  "user=%s password=%s host=%s port=%d dbname=%s sslmode=disable TimeZone=Asia/Shanghai",
	DBTypeSqlserver: "user id=%s;password=%s;server=%s;port=%d;database=%s;encrypt=disable",
}
