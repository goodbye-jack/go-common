package orm

import (
	"context"
	_ "database/sql"
	"fmt"
	_ "gitea.com/kingbase/gokb" // Kingbase 驱动
	glog "github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/orm/dbconfig"
	dm "github.com/jasonlabz/gorm-dm-driver"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

// 改进的下划线替换器，正确处理表名和列名
type underscoreReplacer struct{}

func (r underscoreReplacer) Replace(name string) string {
	// 特殊情况处理：如果是全大写，直接转小写
	if strings.ToUpper(name) == name {
		return strings.ToLower(name)
	}

	// 处理XxxYyyZzz格式
	name = regexp.MustCompile("([a-z0-9])([A-Z])").ReplaceAllString(name, "$1_$2")

	// 处理XYZ格式（多个连续大写字母），但保留缩写词的完整性
	// 例如：ID → id，HTML → html，URL → url
	name = regexp.MustCompile("([A-Z]+)([A-Z][a-z])").ReplaceAllString(name, "$1_$2")
	name = regexp.MustCompile("([a-z])([A-Z])").ReplaceAllString(name, "$1_$2")

	// 确保全部小写
	return strings.ToLower(name)
}

// 创建达梦专用的命名策略
func createDMNamingStrategy() schema.NamingStrategy {
	return schema.NamingStrategy{
		TablePrefix:   "",                   // 无表前缀
		SingularTable: false,                // 使用复数表名（LinkNode → link_nodes）
		NoLowerCase:   false,                // 禁用强制大写
		NameReplacer:  underscoreReplacer{}, // 使用改进的替换器
	}
}

// 创建其他数据库的命名策略
func createDefaultNamingStrategy() schema.NamingStrategy {
	return schema.NamingStrategy{
		TablePrefix:   "",
		SingularTable: false, // 使用单数表名
		NoLowerCase:   false,
		// 可以使用其他替换器或默认行为
	}
}

// 强制小写命名策略
type forcedLowerCaseNamingStrategy struct {
	schema.NamingStrategy
}

func (s forcedLowerCaseNamingStrategy) TableName(table string) string {
	return strings.ToLower(s.NamingStrategy.TableName(table))
}

func (s forcedLowerCaseNamingStrategy) ColumnName(table, column string) string {
	return strings.ToLower(s.NamingStrategy.ColumnName(table, column))
}

type Orm struct {
	db *gorm.DB
}

// NewOrm 创建 ORM 实例
func NewOrm(dsn string, dbtype dbconfig.DBType, slowTime int) *Orm {
	glog.Error("NewOrm param:dsn=%s", dsn)
	if dsn == "" {
		glog.Error("NewOrm param dsn is empty:请检查您的DSN参数")
		return nil
	}
	if dbtype == "" {
		glog.Error("您没有输入DBType,默认使用mysql数据源")
		dbtype = dbconfig.DBTypeMySQL // 默认使用mysql
	}
	if slowTime <= 0 {
		slowTime = 3
	}
	var dialect gorm.Dialector
	switch dbtype {
	case dbconfig.DBTypeMySQL:
		dialect = mysql.Open(dsn)
	case dbconfig.DBTypePostgres:
		dialect = postgres.Open(dsn)
	case dbconfig.DBTypeSqlserver:
		dialect = sqlserver.Open(dsn)
	case dbconfig.DBTypeSQLite:
		dialect = sqlite.Open(dsn)
	case dbconfig.DBTypeDM:
		dialect = dm.Open(dsn)
	case dbconfig.DBTypeKingBase:
		dialect = postgres.New(postgres.Config{
			DriverName: "kingbase",
			DSN:        dsn,
		})
	default:
		glog.Error(fmt.Sprintf("unsupported dbType: %s", string(dsn)))
	}
	dbConfig := &gorm.Config{ // 配置 GORM
		Logger: logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
			SlowThreshold:             time.Duration(slowTime) * time.Second, // 这个最小就是5,后面改成可传入数字
			LogLevel:                  logger.Info,
			IgnoreRecordNotFoundError: false,
			Colorful:                  true,
		}).LogMode(logger.Info),
		DisableForeignKeyConstraintWhenMigrating: true,
		PrepareStmt:                              true,
	}
	db, err := gorm.Open(dialect, dbConfig)
	if err != nil {
		log.Fatalf("%s connect failed, %v", dbtype, err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Minute * 3)
	orm := &Orm{
		db: db,
	}
	if dbtype == dbconfig.DBTypeDM {
		orm.registerDMHooks()
	} else if dbtype == dbconfig.DBTypeKingBase {
		orm.registerKingbaseHooks()
	}
	return orm
}

// 注册达梦专用钩子
func (o *Orm) registerDMHooks() {
	//err := o.db.Callback().Query().Before("gorm:query").Register("dm:convert_limit", convertDMLimit)
	//if err != nil {
	//	log.Fatalf("register DM hooks failed, %v", err)
	//}
}

// 注册人大金仓专用钩子
func (o *Orm) registerKingbaseHooks() {
	// 人大金仓基于 PostgreSQL，通常不需要特殊处理
}

// 达梦 LIMIT/OFFSET 转换钩子
func convertDMLimit(db *gorm.DB) {
	var limit, offset int
	limitClause, hasLimit := db.Statement.Clauses["LIMIT"]
	offsetClause, hasOffset := db.Statement.Clauses["OFFSET"]

	if hasLimit {
		if l, ok := limitClause.Expression.(clause.Limit); ok {
			if l.Limit != nil {
				limit = *l.Limit
			}
		}
	}

	if hasOffset {
		if o, ok := offsetClause.Expression.(*clause.Limit); ok {
			offset = o.Offset
		}
	}

	if limit == 0 && offset == 0 {
		return
	}

	if offset > 0 {
		originalSQL := db.Statement.SQL.String()
		orderExpr := "id ASC"

		if orderClause, hasOrder := db.Statement.Clauses["ORDER BY"]; hasOrder {
			if orderBy, ok := orderClause.Expression.(clause.OrderBy); ok {
				orderExpr = ""
				for _, col := range orderBy.Columns {
					orderExpr += fmt.Sprintf("%s, ", col.Column)
				}
				if orderExpr != "" {
					orderExpr = strings.TrimSuffix(orderExpr, ", ")
				} else {
					orderExpr = "id ASC"
				}
			}
		}
		newSQL := fmt.Sprintf(`
			SELECT * FROM (
				SELECT ROW_NUMBER() OVER (ORDER BY %s) AS rn, t.* 
				FROM (%s) t
			) WHERE rn > %d AND rn <= %d
		`, orderExpr, originalSQL, offset, offset+limit)
		db.Statement.SQL.Reset()
		db.Statement.SQL.WriteString(newSQL)
		delete(db.Statement.Clauses, "LIMIT")
		delete(db.Statement.Clauses, "OFFSET")
	} else if limit > 0 {
		originalSQL := db.Statement.SQL.String()
		newSQL := strings.Replace(originalSQL, "SELECT", fmt.Sprintf("SELECT TOP %d", limit), 1)

		db.Statement.SQL.Reset()
		db.Statement.SQL.WriteString(newSQL)
		delete(db.Statement.Clauses, "LIMIT")
	}
}

func (o *Orm) AutoMigrate(ptr interface{}) {
	err := o.db.AutoMigrate(ptr)
	if err != nil {
		glog.Error("AutoMigrate error: %v", err)
		return
	}
}

func (o *Orm) Migrator(ptr interface{}, indexName string) {
	if err := o.db.Migrator().CreateIndex(ptr, indexName).Error; err != nil {
		glog.Error("CreateIndex error: %v", err)
	}
}

func (o *Orm) Table(name string, args ...interface{}) (tx *gorm.DB) {
	return o.db.Table(name, args...)
}

func (o *Orm) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) {
	db := o.db.WithContext(ctx)
	if err := db.Transaction(fn); err != nil {
		glog.Error("Transaction error: %v", err)
	}
}

func (o *Orm) Create(ctx context.Context, ptr interface{}) error {
	db := o.db.WithContext(ctx)
	return db.Create(ptr).Error
}

// 新增：将驼峰转换为下划线小写
func camelToSnake(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

// snakeToCamel 将下划线命名转换为驼峰命名（首字母大写）
// 例：process_type → ProcessType，link_code → LinkCode
func snakeToCamel(s string) string {
	if s == "" {
		return s
	}
	parts := strings.Split(s, "_")
	var camelParts []string
	for _, part := range parts {
		// 每个部分首字母大写（如 "process" → "Process"，"type" → "Type"）
		camelParts = append(camelParts, strings.Title(strings.ToLower(part)))
	}
	return strings.Join(camelParts, "")
}

// 修改First方法的查询条件处理
func (o *Orm) First(ctx context.Context, res interface{}, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	return db.First(res, filters...).Error
}

func (o *Orm) Last(ctx context.Context, res interface{}, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	return db.Last(res, filters...).Error
}

func (o *Orm) FindAll(ctx context.Context, res interface{}, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Find(res).Error
	}
	return db.Find(res).Error
}

func (o *Orm) FindAllWithOrder(ctx context.Context, res interface{}, order interface{}, filters ...interface{}) error {
	db := o.db.WithContext(ctx).Order(order)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Find(res).Error
	}
	return db.Find(res).Error
}

func (o *Orm) Preload(key string, ctx context.Context, res interface{}, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		db = db.Where(filters[0], filters[1:]...)
	}
	for _, k := range strings.Split(key, ",") {
		k = strings.TrimSpace(k)
		db = db.Preload(k)
	}
	return db.Find(res).Error
}

func (o *Orm) Association(column string) *gorm.Association {
	return o.db.Association(column)
}

func (o *Orm) Page(ctx context.Context, res interface{}, page, pageSize int, sortColumn string, sortSc string, filters ...interface{}) error {
	sortBy := sortColumn + " " + sortSc
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Order(sortBy).Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
	}
	return db.Order(sortBy).Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
}

func (o *Orm) FindJoins(tableName string, ctx context.Context, res interface{}, returnRows, whereCondition string, joins ...string) error {
	db := o.db.WithContext(ctx).Table(tableName).Select(returnRows)
	for _, value := range joins {
		db = db.Joins(value)
	}
	return db.Where(whereCondition).Find(res).Error
}

func (o *Orm) PageJoins(tableName string, ctx context.Context, res interface{}, returnRows, whereCondition string, page, pageSize int, joins ...string) error {
	db := o.db.WithContext(ctx).Table(tableName).Select(returnRows)
	for _, value := range joins {
		db = db.Joins(value)
	}
	return db.Where(whereCondition).Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
}

func (o *Orm) PagePerLoadCondition(key string, ctx context.Context, res interface{}, page, pageSize int, subKey string, subCondition string, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		db = db.Where(filters[0], filters[1:]...)
	}
	if len(subKey) != 0 && len(subCondition) != 0 {
		db = db.Preload(key, subKey+" = ?", subCondition)
	} else {
		db = db.Preload(key)
	}
	return db.Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
}

func (o *Orm) PreloadCount(key string, ctx context.Context, res interface{}, total int64, filters ...interface{}) (int64, error) {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		db = db.Where(filters[0], filters[1:]...)
	}
	db = db.Preload(key).Find(res).Count(&total)
	return total, nil
}

func (o *Orm) PagePerLoad(key string, ctx context.Context, res interface{}, page, pageSize int, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		db = db.Where(filters[0], filters[1:]...)
	}
	db = db.Limit(pageSize).Offset((page - 1) * pageSize)
	for _, k := range strings.Split(key, ",") {
		k = strings.TrimSpace(k)
		db = db.Preload(k)
	}
	return db.Find(res).Error
}

func (o *Orm) Count(ctx context.Context, model interface{}, total *int64, filters ...interface{}) error {
	db := o.db.WithContext(ctx).Model(&model)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Count(total).Error
	}
	return db.Count(total).Error
}

func (o *Orm) CountIdx(ctx context.Context, model interface{}, selectColumns string, total *int64, filters ...interface{}) error {
	db := o.db.WithContext(ctx).Model(&model).Select(selectColumns)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Count(total).Error
	}
	return db.Count(total).Error
}

func (o *Orm) Update(ctx context.Context, ptr interface{}) error {
	db := o.db.WithContext(ctx)
	return db.Save(ptr).Error
}

func (o *Orm) Delete(ctx context.Context, ptr interface{}) error {
	db := o.db.WithContext(ctx)
	return db.Delete(ptr).Error
}

func (o *Orm) DeleteCondition(ctx context.Context, ptr interface{}, filters ...interface{}) error {
	db := o.db.WithContext(ctx)
	if len(filters) > 0 {
		return db.Where(filters[0], filters[1:]...).Delete(ptr).Error
	}
	return db.Delete(ptr).Error
}

func (o *Orm) GroupBy(ctx context.Context, tableName string, selectColumns string, whereClause interface{}, results interface{}, groupColumns string) error {
	db := o.db.WithContext(ctx)
	return db.Table(tableName).Select(selectColumns).Where(whereClause).Group(groupColumns).Find(results).Error
}

func (o *Orm) Top(ctx context.Context, tableName string, selectColumns string, whereClause interface{}, groupColumn string, sortColumn string, sortSc string, limitCount int, results interface{}) error {
	db := o.db.WithContext(ctx)
	sortBy := sortColumn + " " + sortSc
	return db.Table(tableName).Select(selectColumns).Where(whereClause).Group(groupColumn).Order(sortBy).Limit(limitCount).Find(results).Error
}

func (o *Orm) Exec(sql string, value ...interface{}) error {
	return o.db.Exec(sql, value...).Error
}

func (o *Orm) Raw(sql string, result interface{}, value ...interface{}) error {
	return o.db.Raw(sql, value...).Scan(result).Error
}

//type Orm struct {
//	db *gorm.DB
//}
//
//// NewOrm 创建 ORM 实例
//func NewOrm(dsn string, dbtype config.DBType, slowTime int) *Orm {
//	glog.Error("NewOrm param:dsn=%s", dsn)
//	if dsn == "" {
//		glog.Error("NewOrm param dsn is empty:请检查您的DSN参数")
//		return nil
//	}
//	if dbtype == "" {
//		glog.Error("您没有输入DBType,默认使用mysql数据源")
//		dbtype = config.DBTypeMySQL // 默认使用mysql
//	}
//	if slowTime <= 0 {
//		slowTime = 3
//	}
//	var dialect gorm.Dialector
//	switch dbtype {
//	case config.DBTypeMySQL:
//		dialect = mysql.Open(dsn)
//	case config.DBTypePostgres:
//		dialect = postgres.Open(dsn)
//	case config.DBTypeSqlserver:
//		dialect = sqlserver.Open(dsn)
//	case config.DBTypeSQLite:
//		dialect = sqlite.Open(dsn)
//	case config.DBTypeDM:
//		dialect = dm.Open(dsn)
//	case config.DBTtypeKingBase:
//		dialect = postgres.New(postgres.Config{
//			DriverName: "kingbase",
//			DSN:        dsn,
//		})
//	default:
//		glog.Error(fmt.Sprintf("unsupported dbType: %s", string(dsn)))
//	}
//
//	// 统一命名策略：表名和列名使用小写下划线格式
//	namingStrategy := schema.NamingStrategy{
//		TablePrefix:   "",
//		SingularTable: true,
//		NameReplacer:  regexp.MustCompile("([A-Z])"),
//		// 将驼峰转换为下划线小写格式
//		Translate: func(s string) string {
//			return strings.ToLower(
//				schema.NamingStrategy{
//					NameReplacer: regexp.MustCompile("([A-Z])"),
//				}.Translate(s),
//			)
//		},
//		NoLowerCase: false, // 所有数据库都使用小写
//	}
//	dbConfig := &gorm.Config{
//		Logger: logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
//			SlowThreshold:             time.Duration(slowTime) * time.Second,
//			LogLevel:                  logger.Info,
//			IgnoreRecordNotFoundError: false,
//			Colorful:                  true,
//		}).LogMode(logger.Info),
//		DisableForeignKeyConstraintWhenMigrating: true,
//		PrepareStmt:                              true,
//		NamingStrategy:                           namingStrategy,
//	}
//	db, err := gorm.Open(dialect, dbConfig)
//	if err != nil {
//		log.Fatalf("%s connect failed, %v", dbtype, err)
//	}
//	sqlDB, _ := db.DB()
//	sqlDB.SetMaxIdleConns(10)
//	sqlDB.SetMaxOpenConns(100)
//	sqlDB.SetConnMaxLifetime(time.Minute * 3)
//	orm := &Orm{
//		db: db,
//	}
//	if dbtype == config.DBTypeDM {
//		orm.registerDMHooks()
//	} else if dbtype == config.DBTtypeKingBase {
//		orm.registerKingbaseHooks()
//	}
//	return orm
//}
//
//// 注册达梦专用钩子
//func (o *Orm) registerDMHooks() {
//	err := o.db.Callback().Query().Before("gorm:query").Register("dm:convert_limit", convertDMLimit)
//	if err != nil {
//		log.Fatalf("register DM hooks failed, %v", err)
//	}
//}
//
//// 注册人大金仓专用钩子
//func (o *Orm) registerKingbaseHooks() {
//	// 人大金仓基于 PostgreSQL，通常不需要特殊处理
//}
//
//// 达梦 LIMIT/OFFSET 转换钩子
//func convertDMLimit(db *gorm.DB) {
//	var limit, offset int
//	limitClause, hasLimit := db.Statement.Clauses["LIMIT"]
//	offsetClause, hasOffset := db.Statement.Clauses["OFFSET"]
//
//	if hasLimit {
//		if l, ok := limitClause.Expression.(clause.Limit); ok {
//			if l.Limit != nil {
//				limit = *l.Limit
//			}
//		}
//	}
//
//	if hasOffset {
//		if o, ok := offsetClause.Expression.(*clause.Limit); ok {
//			offset = o.Offset
//		}
//	}
//
//	if limit == 0 && offset == 0 {
//		return
//	}
//
//	if offset > 0 {
//		originalSQL := db.Statement.SQL.String()
//		orderExpr := "id ASC"
//
//		if orderClause, hasOrder := db.Statement.Clauses["ORDER BY"]; hasOrder {
//			if orderBy, ok := orderClause.Expression.(clause.OrderBy); ok {
//				orderExpr = ""
//				for _, col := range orderBy.Columns {
//					orderExpr += fmt.Sprintf("%s, ", col.Column)
//				}
//				if orderExpr != "" {
//					orderExpr = strings.TrimSuffix(orderExpr, ", ")
//				} else {
//					orderExpr = "id ASC"
//				}
//			}
//		}
//		newSQL := fmt.Sprintf(`
//			SELECT * FROM (
//				SELECT ROW_NUMBER() OVER (ORDER BY %s) AS rn, t.*
//				FROM (%s) t
//			) WHERE rn > %d AND rn <= %d
//		`, orderExpr, originalSQL, offset, offset+limit)
//		db.Statement.SQL.Reset()
//		db.Statement.SQL.WriteString(newSQL)
//		delete(db.Statement.Clauses, "LIMIT")
//		delete(db.Statement.Clauses, "OFFSET")
//	} else if limit > 0 {
//		originalSQL := db.Statement.SQL.String()
//		newSQL := strings.Replace(originalSQL, "SELECT", fmt.Sprintf("SELECT TOP %d", limit), 1)
//
//		db.Statement.SQL.Reset()
//		db.Statement.SQL.WriteString(newSQL)
//		delete(db.Statement.Clauses, "LIMIT")
//	}
//}
//
//func (o *Orm) AutoMigrate(ptr interface{}) {
//	err := o.db.AutoMigrate(ptr)
//	if err != nil {
//		glog.Error("AutoMigrate error: %v", err)
//		return
//	}
//}
//
//func (o *Orm) Migrator(ptr interface{}, indexName string) {
//	if err := o.db.Migrator().CreateIndex(ptr, indexName).Error; err != nil {
//		glog.Error("CreateIndex error: %v", err)
//	}
//}
//
//func (o *Orm) Table(name string, args ...interface{}) (tx *gorm.DB) {
//	return o.db.Table(name, args...)
//}
//
//func (o *Orm) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) {
//	db := o.db.WithContext(ctx)
//	if err := db.Transaction(fn); err != nil {
//		glog.Error("Transaction error: %v", err)
//	}
//}
//
//func (o *Orm) Create(ctx context.Context, ptr interface{}) error {
//	db := o.db.WithContext(ctx)
//	return db.Create(ptr).Error
//}
//
//// 新增：将驼峰转换为下划线小写
//func camelToSnake(s string) string {
//	var result strings.Builder
//	for i, r := range s {
//		if i > 0 && r >= 'A' && r <= 'Z' {
//			result.WriteRune('_')
//		}
//		result.WriteRune(r)
//	}
//	return strings.ToLower(result.String())
//}
//
//// 修改First方法的查询条件处理
//func (o *Orm) First(ctx context.Context, res interface{}, filters ...interface{}) error {
//	db := o.db.WithContext(ctx)
//	if o.db.Dialector.Name() == "dm" && len(filters) > 0 {
//		if where, ok := filters[0].(string); ok {
//			// 对于达梦数据库，将查询条件中的列名保持小写下划线格式
//			// 不需要转换，因为我们的命名策略已经将数据库列名设置为小写下划线
//			filters[0] = where
//		}
//	}
//	return db.First(res, filters...).Error
//}
//
//func (o *Orm) Last(ctx context.Context, res interface{}, filters ...interface{}) error {
//	db := o.db.WithContext(ctx)
//	return db.Last(res, filters...).Error
//}
//
//func (o *Orm) FindAll(ctx context.Context, res interface{}, filters ...interface{}) error {
//	db := o.db.WithContext(ctx)
//	if len(filters) > 0 {
//		return db.Where(filters[0], filters[1:]...).Find(res).Error
//	}
//	return db.Find(res).Error
//}
//
//func (o *Orm) FindAllWithOrder(ctx context.Context, res interface{}, order interface{}, filters ...interface{}) error {
//	db := o.db.WithContext(ctx).Order(order)
//	if len(filters) > 0 {
//		return db.Where(filters[0], filters[1:]...).Find(res).Error
//	}
//	return db.Find(res).Error
//}
//
//func (o *Orm) Preload(key string, ctx context.Context, res interface{}, filters ...interface{}) error {
//	db := o.db.WithContext(ctx)
//	if len(filters) > 0 {
//		db = db.Where(filters[0], filters[1:]...)
//	}
//	for _, k := range strings.Split(key, ",") {
//		k = strings.TrimSpace(k)
//		db = db.Preload(k)
//	}
//	return db.Find(res).Error
//}
//
//func (o *Orm) Association(column string) *gorm.Association {
//	return o.db.Association(column)
//}
//
//func (o *Orm) Page(ctx context.Context, res interface{}, page, pageSize int, sortColumn string, sortSc string, filters ...interface{}) error {
//	sortBy := sortColumn + " " + sortSc
//	db := o.db.WithContext(ctx)
//	if len(filters) > 0 {
//		return db.Where(filters[0], filters[1:]...).Order(sortBy).Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
//	}
//	return db.Order(sortBy).Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
//}
//
//func (o *Orm) FindJoins(tableName string, ctx context.Context, res interface{}, returnRows, whereCondition string, joins ...string) error {
//	db := o.db.WithContext(ctx).Table(tableName).Select(returnRows)
//	for _, value := range joins {
//		db = db.Joins(value)
//	}
//	return db.Where(whereCondition).Find(res).Error
//}
//
//func (o *Orm) PageJoins(tableName string, ctx context.Context, res interface{}, returnRows, whereCondition string, page, pageSize int, joins ...string) error {
//	db := o.db.WithContext(ctx).Table(tableName).Select(returnRows)
//	for _, value := range joins {
//		db = db.Joins(value)
//	}
//	return db.Where(whereCondition).Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
//}
//
//func (o *Orm) PagePerLoadCondition(key string, ctx context.Context, res interface{}, page, pageSize int, subKey string, subCondition string, filters ...interface{}) error {
//	db := o.db.WithContext(ctx)
//	if len(filters) > 0 {
//		db = db.Where(filters[0], filters[1:]...)
//	}
//	if len(subKey) != 0 && len(subCondition) != 0 {
//		db = db.Preload(key, subKey+" = ?", subCondition)
//	} else {
//		db = db.Preload(key)
//	}
//	return db.Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
//}
//
//func (o *Orm) PreloadCount(key string, ctx context.Context, res interface{}, total int64, filters ...interface{}) (int64, error) {
//	db := o.db.WithContext(ctx)
//	if len(filters) > 0 {
//		db = db.Where(filters[0], filters[1:]...)
//	}
//	db = db.Preload(key).Find(res).Count(&total)
//	return total, nil
//}
//
//func (o *Orm) PagePerLoad(key string, ctx context.Context, res interface{}, page, pageSize int, filters ...interface{}) error {
//	db := o.db.WithContext(ctx)
//	if len(filters) > 0 {
//		db = db.Where(filters[0], filters[1:]...)
//	}
//	db = db.Limit(pageSize).Offset((page - 1) * pageSize)
//	for _, k := range strings.Split(key, ",") {
//		k = strings.TrimSpace(k)
//		db = db.Preload(k)
//	}
//	return db.Find(res).Error
//}
//
//func (o *Orm) Count(ctx context.Context, model interface{}, total *int64, filters ...interface{}) error {
//	db := o.db.WithContext(ctx).Model(&model)
//	if len(filters) > 0 {
//		return db.Where(filters[0], filters[1:]...).Count(total).Error
//	}
//	return db.Count(total).Error
//}
//
//func (o *Orm) CountIdx(ctx context.Context, model interface{}, selectColumns string, total *int64, filters ...interface{}) error {
//	db := o.db.WithContext(ctx).Model(&model).Select(selectColumns)
//	if len(filters) > 0 {
//		return db.Where(filters[0], filters[1:]...).Count(total).Error
//	}
//	return db.Count(total).Error
//}
//
//func (o *Orm) Update(ctx context.Context, ptr interface{}) error {
//	db := o.db.WithContext(ctx)
//	return db.Save(ptr).Error
//}
//
//func (o *Orm) Delete(ctx context.Context, ptr interface{}) error {
//	db := o.db.WithContext(ctx)
//	return db.Delete(ptr).Error
//}
//
//func (o *Orm) DeleteCondition(ctx context.Context, ptr interface{}, filters ...interface{}) error {
//	db := o.db.WithContext(ctx)
//	if len(filters) > 0 {
//		return db.Where(filters[0], filters[1:]...).Delete(ptr).Error
//	}
//	return db.Delete(ptr).Error
//}
//
//func (o *Orm) GroupBy(ctx context.Context, tableName string, selectColumns string, whereClause interface{}, results interface{}, groupColumns string) error {
//	db := o.db.WithContext(ctx)
//	return db.Table(tableName).Select(selectColumns).Where(whereClause).Group(groupColumns).Find(results).Error
//}
//
//func (o *Orm) Top(ctx context.Context, tableName string, selectColumns string, whereClause interface{}, groupColumn string, sortColumn string, sortSc string, limitCount int, results interface{}) error {
//	db := o.db.WithContext(ctx)
//	sortBy := sortColumn + " " + sortSc
//	return db.Table(tableName).Select(selectColumns).Where(whereClause).Group(groupColumn).Order(sortBy).Limit(limitCount).Find(results).Error
//}
//
//func (o *Orm) Exec(sql string, value ...interface{}) error {
//	return o.db.Exec(sql, value...).Error
//}
//
//func (o *Orm) Raw(sql string, result interface{}, value ...interface{}) error {
//	return o.db.Raw(sql, value...).Scan(result).Error
//}
