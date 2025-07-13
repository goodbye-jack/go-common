package orm

import (
	"context"
	"database/sql" // 添加这个导入
	"fmt"
	_ "gitea.com/kingbase/gokb" // Kingbase 驱动
	glog "github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/orm/config"
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
	"strings"
	"time"
	//"github.com/jasonlabz/oracle"
	//"github.com/goodbye-jack/go-common/orm/dialect"
)

type Orm struct {
	db *gorm.DB
	//dbtype config.DBType // 新增：存储数据库类型（mysql/dm/kingbase）
}

// NewOrm 创建 ORM 实例
func NewOrm(dsn string, dbtype config.DBType, slowTime int) *Orm {
	glog.Error("NewOrm param:dsn=%s", dsn)
	if dsn == "" {
		glog.Error("NewOrm param dsn is empty:请检查您的DSN参数")
		return nil
	}
	if dbtype == "" {
		glog.Error("您没有输入DBType,默认使用mysql数据源")
		dbtype = config.DBTypeMySQL // 默认使用mysql
	}
	if slowTime <= 0 {
		slowTime = 3
	}
	var dialect gorm.Dialector
	switch dbtype {
	case config.DBTypeMySQL:
		dialect = mysql.Open(dsn)
	case config.DBTypePostgres:
		dialect = postgres.Open(dsn)
	case config.DBTypeSqlserver:
		dialect = sqlserver.Open(dsn)
	//case config.DBTypeOracle:
	//	dialect = oracle.Open(dsn)
	case config.DBTypeSQLite:
		dialect = sqlite.Open(dsn)
	case config.DBTypeDM:
		dialect = dm.Open(dsn)
	case config.DBTtypeKingBase: // 使用人大金仓方言（基于 PostgreSQL）
		//dialect, _ = kingbase.Open(dsn)
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
		NamingStrategy: schema.NamingStrategy{
			// 对于达梦数据库，使用大写表名和列名
			TablePrefix:   "",
			SingularTable: true,
			NameReplacer:  nil,
			NoLowerCase:   dbtype == config.DBTypeDM, // 达梦数据库不使用小写
		},
	}
	db, err := gorm.Open(dialect, dbConfig) // 创建数据库连接
	if err != nil {
		log.Fatalf("%s connect failed, %v", dbtype, err)
	}
	sqlDB, _ := db.DB() // 设置连接池参数
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Minute * 3)
	orm := &Orm{
		db: db,
	}
	if dbtype == config.DBTypeDM { // 注册数据库专用钩子
		orm.registerDMHooks()
	} else if dbtype == config.DBTtypeKingBase {
		orm.registerKingbaseHooks()
	}
	return orm
}

// 注册达梦专用钩子
func (o *Orm) registerDMHooks() {
	// 处理 LIMIT/OFFSET 转换
	o.db.Callback().Query().Before("gorm:query").Register("dm:convert_limit", convertDMLimit)
}

// 注册人大金仓专用钩子
func (o *Orm) registerKingbaseHooks() {
	// 人大金仓基于 PostgreSQL，通常不需要特殊处理
	// 如有特殊需求，可在此添加钩子
}

// 达梦 LIMIT/OFFSET 转换钩子
func convertDMLimit(db *gorm.DB) {
	// 从 Clauses 中获取 LIMIT 和 OFFSET 参数
	var limit, offset int
	limitClause, hasLimit := db.Statement.Clauses["LIMIT"]
	offsetClause, hasOffset := db.Statement.Clauses["OFFSET"]

	// 解析 LIMIT 值
	if hasLimit {
		if l, ok := limitClause.Expression.(clause.Limit); ok {
			if l.Limit != nil {
				limit = *l.Limit
			}
		}
	}

	// 解析 OFFSET 值
	if hasOffset {
		if o, ok := offsetClause.Expression.(*clause.Limit); ok {
			offset = o.Offset
		}
	}
	// 只有当存在 LIMIT 或 OFFSET 时才处理
	if limit == 0 && offset == 0 {
		return
	}

	// 达梦不支持 LIMIT/OFFSET，需要转换为 TOP 和子查询
	if offset > 0 {
		// 有 OFFSET 时使用 ROW_NUMBER() 函数
		originalSQL := db.Statement.SQL.String()
		orderExpr := "id ASC" // 默认排序

		// 获取用户指定的排序条件
		if orderClause, hasOrder := db.Statement.Clauses["ORDER BY"]; hasOrder {
			if orderBy, ok := orderClause.Expression.(clause.OrderBy); ok {
				orderExpr = ""
				for _, col := range orderBy.Columns {
					orderExpr += fmt.Sprintf("%s, ", col.Column)
				}
				if orderExpr != "" {
					orderExpr = strings.TrimSuffix(orderExpr, ", ")
				} else {
					orderExpr = "id ASC" // 默认排序
				}
			}
		}
		// 构造达梦的分页查询
		newSQL := fmt.Sprintf(`
			SELECT * FROM (
				SELECT ROW_NUMBER() OVER (ORDER BY %s) AS rn, t.* 
				FROM (%s) t
			) WHERE rn > %d AND rn <= %d
		`, orderExpr, originalSQL, offset, offset+limit)

		// 替换原始 SQL 并清除原有分页参数
		db.Statement.SQL.Reset()
		db.Statement.SQL.WriteString(newSQL)
		delete(db.Statement.Clauses, "LIMIT")
		delete(db.Statement.Clauses, "OFFSET")
	} else if limit > 0 {
		// 只有 LIMIT 时使用 TOP 语法
		originalSQL := db.Statement.SQL.String()
		newSQL := strings.Replace(originalSQL, "SELECT", fmt.Sprintf("SELECT TOP %d", limit), 1)

		// 替换原始 SQL 并清除原有分页参数
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

//	func (o *Orm) RawRows(sql string, value ...interface{}) (*gorm.Rows, error) {
//		return o.db.Raw(sql, value...).Rows()
//	}
//

// 修改后的 RawRows 方法
func (o *Orm) RawRows(sql string, value ...interface{}) (*sql.Rows, error) {
	return o.db.Raw(sql, value...).Rows()
}

//
//type Orm struct {
//	db     *gorm.DB
//	dbtype string // 新增：存储数据库类型（mysql/dm/kingbase）
//}
//
////func NewOrm(dsn, dbtype string) *Orm {
////	goodlog.Info("NewOrm param:dsn=%s", dsn)
////	var dialector gorm.Dialector
////	switch dbtype {
////	case "mysql": // mysql
////		dialector = mysql.Open(dsn)
////	case "kingbase": //人大金仓
////		dialector = postgres.New(postgres.Config{
////			DriverName: "kingbase", // 指定使用 kingbase 驱动
////			DSN:        dsn,
////		})
////	case "dm": // 达梦数据库
////		dialector = postgres.New(postgres.Config{
////			DriverName: "dm", // 指定使用达梦数据库驱动
////			DSN:        dsn,
////		})
////	default:
////		log.Fatalf("Unsupported database type: %s", dbtype)
////	}
////	conf := &gorm.Config{
////		Logger: logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
////			SlowThreshold:             5000 * time.Millisecond, // 这个最小就是5,后面改成可传入数字
////			LogLevel:                  logger.Info,
////			IgnoreRecordNotFoundError: false,
////			Colorful:                  true,
////		}).LogMode(logger.Info),
////	}
////	db, err := gorm.Open(dialector, conf)
////	if err != nil {
////		log.Fatalf("%s connect failed, %v", dbtype, err)
////	}
////	return &Orm{
////		db:     db,
////		dbtype: dbtype, // 保存数据库类型
////	}
////}
//
//// NewOrm 创建 ORM 实例
//func NewOrm(dsn, dbtype string) *Orm {
//	glog.Error("NewOrm param:dsn=%s", dsn)
//	var dialector gorm.Dialector
//	switch dbtype {
//	case "mysql":
//		dialector = mysql.Open(dsn)
//	case "dm": // 使用达梦专用方言
//		dialector = dialect.NewDMDialector(dsn)
//	case "kingbase": // 使用人大金仓方言（基于 PostgreSQL）
//		dialector = postgres.New(postgres.Config{
//			DriverName: "kingbase",
//			DSN:        dsn,
//		})
//	default:
//		log.Fatalf("Unsupported database type: %s", dbtype)
//	}
//	config := &gorm.Config{ // 配置 GORM
//		Logger: logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
//			SlowThreshold:             5000 * time.Millisecond, // 这个最小就是5,后面改成可传入数字
//			LogLevel:                  logger.Info,
//			IgnoreRecordNotFoundError: false,
//			Colorful:                  true,
//		}).LogMode(logger.Info),
//		DisableForeignKeyConstraintWhenMigrating: true,
//		PrepareStmt:                              true,
//	}
//	db, err := gorm.Open(dialector, config) // 创建数据库连接
//	if err != nil {
//		log.Fatalf("%s connect failed, %v", dbtype, err)
//	}
//	sqlDB, _ := db.DB() // 设置连接池参数
//	sqlDB.SetMaxIdleConns(10)
//	sqlDB.SetMaxOpenConns(100)
//	sqlDB.SetConnMaxLifetime(time.Minute * 3)
//	orm := &Orm{
//		db:     db,
//		dbtype: dbtype,
//	}
//	if dbtype == "dm" { // 注册数据库专用钩子
//		orm.registerDMHooks()
//	} else if dbtype == "kingbase" {
//		orm.registerKingbaseHooks()
//	}
//	return orm
//}
//
//// 注册达梦专用钩子
//func (o *Orm) registerDMHooks() {
//	// 处理 LIMIT/OFFSET 转换
//	o.db.Callback().Query().Before("gorm:query").Register("dm:convert_limit", convertDMLimit)
//}
//
//// 注册人大金仓专用钩子
//func (o *Orm) registerKingbaseHooks() {
//	// 人大金仓基于 PostgreSQL，通常不需要特殊处理
//	// 如有特殊需求，可在此添加钩子
//}
//
//// 达梦 LIMIT/OFFSET 转换钩子
//func convertDMLimit(db *gorm.DB) {
//	// 从 Clauses 中获取 LIMIT 和 OFFSET 参数
//	var limit, offset int
//	limitClause, hasLimit := db.Statement.Clauses["LIMIT"]
//	offsetClause, hasOffset := db.Statement.Clauses["OFFSET"]
//
//	// 解析 LIMIT 值
//	if hasLimit {
//		if l, ok := limitClause.Expression.(clause.Limit); ok {
//			if l.Limit != nil {
//				limit = *l.Limit
//			}
//		}
//	}
//
//	// 解析 OFFSET 值
//	if hasOffset {
//		if o, ok := offsetClause.Expression.(*clause.Limit); ok {
//			offset = o.Offset
//		}
//	}
//	// 只有当存在 LIMIT 或 OFFSET 时才处理
//	if limit == 0 && offset == 0 {
//		return
//	}
//
//	// 达梦不支持 LIMIT/OFFSET，需要转换为 TOP 和子查询
//	if offset > 0 {
//		// 有 OFFSET 时使用 ROW_NUMBER() 函数
//		originalSQL := db.Statement.SQL.String()
//		orderExpr := "id ASC" // 默认排序
//
//		// 获取用户指定的排序条件
//		if orderClause, hasOrder := db.Statement.Clauses["ORDER BY"]; hasOrder {
//			if orderBy, ok := orderClause.Expression.(clause.OrderBy); ok {
//				orderExpr = ""
//				for _, col := range orderBy.Columns {
//					// 使用 col.Column 直接获取排序表达式
//					// 不再使用 Direction 属性，直接使用 col.Column 的字符串表示
//					orderExpr += fmt.Sprintf("%s, ", col.Column)
//				}
//				if orderExpr != "" {
//					orderExpr = strings.TrimSuffix(orderExpr, ", ")
//				} else {
//					orderExpr = "id ASC" // 默认排序
//				}
//			}
//		}
//		// 构造达梦的分页查询
//		newSQL := fmt.Sprintf(`
//			SELECT * FROM (
//				SELECT ROW_NUMBER() OVER (ORDER BY %s) AS rn, t.*
//				FROM (%s) t
//			) WHERE rn > %d AND rn <= %d
//		`, orderExpr, originalSQL, offset, offset+limit)
//
//		// 替换原始 SQL 并清除原有分页参数
//		db.Statement.SQL.Reset()
//		db.Statement.SQL.WriteString(newSQL)
//		delete(db.Statement.Clauses, "LIMIT")
//		delete(db.Statement.Clauses, "OFFSET")
//	} else if limit > 0 {
//		// 只有 LIMIT 时使用 TOP 语法
//		originalSQL := db.Statement.SQL.String()
//		newSQL := strings.Replace(originalSQL, "SELECT", fmt.Sprintf("SELECT TOP %d", limit), 1)
//
//		// 替换原始 SQL 并清除原有分页参数
//		db.Statement.SQL.Reset()
//		db.Statement.SQL.WriteString(newSQL)
//		delete(db.Statement.Clauses, "LIMIT")
//	}
//}
//
////func convertDMLimit(db *gorm.DB) {
////	// 从 Clauses 中获取 LIMIT 和 OFFSET 参数
////	var limit, offset int
////	limitClause, hasLimit := db.Statement.Clauses["LIMIT"]
////	offsetClause, hasOffset := db.Statement.Clauses["OFFSET"]
////	// 解析 LIMIT 值
////	if hasLimit {
////		// 断言为 clause.Limit 类型
////		if l, ok := limitClause.Expression.(clause.Limit); ok {
////			if l.Limit != nil {
////				limit = *l.Limit // 提取实际的 LIMIT 数值
////			}
////		}
////	}
////	// 解析 OFFSET 值，这里换一种方式，直接从 offsetClause 的 Value 等属性尝试获取（根据实际 gorm 版本和逻辑调整）
////	if hasOffset {
////		// 假设可以通过反射或者其他合理方式获取，这里先模拟合理逻辑，你可能需要根据实际 gorm 内部结构微调
////		// 比如如果 offsetClause 存储值的方式和 limit 类似，只是类型断言不能用 clause.Offset，尝试其他方式
////		// 以下是一种假设性的调整，你需要结合实际 gorm 代码确认
////		if o, ok := offsetClause.Expression.(*clause.Limit); ok {
////			if o != nil && o.Offset != 0 {
////				offset = o.Offset // 提取实际的 OFFSET 数值，这里假设 o.Offset 是 *int 类型，和 limit 类似
////			}
////		}
////	}
////	// 只有当存在 LIMIT 或 OFFSET 时才处理
////	if limit == 0 && offset == 0 {
////		return
////	}
////	// 达梦不支持 LIMIT/OFFSET，需要转换为 TOP 和子查询
////	if offset > 0 {
////		// 有 OFFSET 时使用 ROW_NUMBER() 函数
////		originalSQL := db.Statement.SQL.String()
////		orderExpr := "id ASC" // 默认排序，可根据实际情况修改
////		// 获取用户指定的排序条件
////		if orderClause, hasOrder := db.Statement.Clauses["ORDER BY"]; hasOrder {
////			if orderBy, ok := orderClause.Expression.(clause.OrderBy); ok && len(orderBy.Orders) > 0 {
////				orderExpr = ""
////				for _, order := range orderBy.Orders {
////					orderExpr += order.Column + " " + order.Direction + ", "
////				}
////				orderExpr = strings.TrimSuffix(orderExpr, ", ")
////			}
////		}
////		// 构造达梦的分页查询（使用 ROW_NUMBER() 实现 OFFSET）
////		newSQL := fmt.Sprintf(`
////            SELECT * FROM (
////                SELECT ROW_NUMBER() OVER (ORDER BY %s) AS rn, t.*
////                FROM (%s) t
////            ) WHERE rn > %d AND rn <= %d
////        `, orderExpr, originalSQL, offset, offset+limit)
////		// 替换原始 SQL 并清除原有分页参数
////		db.Statement.SQL.Reset()
////		db.Statement.SQL.WriteString(newSQL)
////		delete(db.Statement.Clauses, "LIMIT")
////		delete(db.Statement.Clauses, "OFFSET")
////	} else if limit > 0 {
////		// 只有 LIMIT 时使用 TOP 语法
////		originalSQL := db.Statement.SQL.String()
////		newSQL := strings.Replace(originalSQL, "SELECT", fmt.Sprintf("SELECT TOP %d", limit), 1)
////		// 替换原始 SQL 并清除原有分页参数
////		db.Statement.SQL.Reset()
////		db.Statement.SQL.WriteString(newSQL)
////		delete(db.Statement.Clauses, "LIMIT")
////	}
////}
//
////// 达梦 LIMIT/OFFSET 转换钩子
////func convertDMLimit(db *gorm.DB) {
////	// 从 Clauses 中获取 LIMIT 和 OFFSET 参数
////	var limit, offset int
////	limitClause, hasLimit := db.Statement.Clauses["LIMIT"]
////	offsetClause, hasOffset := db.Statement.Clauses["OFFSET"]
////	// 解析 LIMIT 值
////	if hasLimit {
////		if l, ok := limitClause.Expression.(clause.Limit); ok {
////			if l.Limit != nil {
////				limit = *l.Limit // 提取实际的 LIMIT 数值
////			}
////		}
////	}
////	// 解析 OFFSET 值
////	if hasOffset {
////		if o, ok := offsetClause.Expression.(clause.Offset); ok {
////			if o.Offset != nil {
////				offset = *o.Offset.(*int) // 提取实际的 OFFSET 数值
////			}
////		}
////	}
////	// 只有当存在 LIMIT 或 OFFSET 时才处理
////	if limit == 0 && offset == 0 {
////		return
////	}
////	// 达梦不支持 LIMIT/OFFSET，需要转换为 TOP 和子查询
////	if offset > 0 {
////		// 有 OFFSET 时使用 ROW_NUMBER() 函数
////		originalSQL := db.Statement.SQL.String()
////		orderExpr := "id ASC" // 默认排序，可根据实际情况修改
////		// 获取用户指定的排序条件
////		if orderClause, hasOrder := db.Statement.Clauses["ORDER BY"]; hasOrder {
////			if orderBy, ok := orderClause.Expression.(clause.OrderBy); ok && len(orderBy.Orders) > 0 {
////				orderExpr = ""
////				for _, order := range orderBy.Orders {
////					orderExpr += order.Column + " " + order.Direction + ", "
////				}
////				orderExpr = strings.TrimSuffix(orderExpr, ", ")
////			}
////		}
////		// 构造达梦的分页查询（使用 ROW_NUMBER() 实现 OFFSET）
////		newSQL := fmt.Sprintf(`
////			SELECT * FROM (
////				SELECT ROW_NUMBER() OVER (ORDER BY %s) AS rn, t.*
////				FROM (%s) t
////			) WHERE rn > %d AND rn <= %d
////		`, orderExpr, originalSQL, offset, offset+limit)
////		// 替换原始 SQL 并清除原有分页参数
////		db.Statement.SQL.Reset()
////		db.Statement.SQL.WriteString(newSQL)
////		delete(db.Statement.Clauses, "LIMIT")
////		delete(db.Statement.Clauses, "OFFSET")
////	} else if limit > 0 {
////		// 只有 LIMIT 时使用 TOP 语法
////		originalSQL := db.Statement.SQL.String()
////		newSQL := strings.Replace(originalSQL, "SELECT", fmt.Sprintf("SELECT TOP %d", limit), 1)
////		// 替换原始 SQL 并清除原有分页参数
////		db.Statement.SQL.Reset()
////		db.Statement.SQL.WriteString(newSQL)
////		delete(db.Statement.Clauses, "LIMIT")
////	}
////}
//
////func NewOrm(dsn, dbtype string) *Orm {
////	goodlog.Info("NewOrm param:dsn=%s", dsn)
////
////	dialector := mysql.Open(dsn)
////	if dbtype == "kingbase" {
////		dialector = postgres.New(postgres.Config{
////			DriverName: "kingbase", // 指定使用 kingbase 驱动
////			DSN:        dsn,
////		})
////	}
////	conf := &gorm.Config{
////		Logger: logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
////			SlowThreshold:             5000 * time.Millisecond, // 这个最小就是5,后面改成可传入数字
////			LogLevel:                  logger.Info,
////			IgnoreRecordNotFoundError: false,
////			Colorful:                  true,
////		}).LogMode(logger.Info),
////	}
////	db, err := gorm.Open(dialector, conf)
////	if err != nil {
////		log.Fatal("%s connect failed, %v", dbtype, err)
////	}
////	return &Orm{
////		db: db,
////	}
////}
//
//func (o *Orm) AutoMigrate(ptr interface{}) {
//	err := o.db.AutoMigrate(ptr)
//	if err != nil {
//		return
//	}
//}
//
//func (o *Orm) Migrator(ptr interface{}, indexName string) {
//	o.db.Migrator().CreateIndex(ptr, indexName).Error()
//}
//
//func (o *Orm) Table(name string, args ...interface{}) (tx *gorm.DB) {
//	return o.db.Table(name, args...)
//}
//
//func (o *Orm) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) {
//	db := o.db.WithContext(ctx)
//	db.Transaction(fn)
//}
//
//func (o *Orm) Create(ctx context.Context, ptr interface{}) error {
//	db := o.db.WithContext(ctx)
//	return db.Create(ptr).Error
//}
//
//func (o *Orm) First(ctx context.Context, res interface{}, filters ...interface{}) error {
//	db := o.db.WithContext(ctx)
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
//	for index, value := range joins {
//		fmt.Printf("Index: %d, Value: %d\n", index, value)
//		db.Joins(value)
//	}
//	return db.Where(whereCondition).Find(res).Error
//}
//
//func (o *Orm) PageJoins(tableName string, ctx context.Context, res interface{}, returnRows, whereCondition string, page, pageSize int, joins ...string) error {
//	db := o.db.WithContext(ctx).Table(tableName).Select(returnRows)
//	for index, value := range joins {
//		fmt.Printf("Index: %d, Value: %d\n", index, value)
//		db.Joins(value)
//	}
//	return db.Where(whereCondition).Limit(pageSize).Offset((page - 1) * pageSize).Find(res).Error
//}
//
//func (o *Orm) PagePerLoadCondition(key string, ctx context.Context, res interface{}, page, pageSize int, subKey string, subCondition string, filters ...interface{}) error {
//	db := o.db.WithContext(ctx)
//	if len(filters) > 0 {
//		//return db.Where(filters[0], filters[1:]...).Limit(pageSize).Offset((page-1)*pageSize).Preload(key, subKey+" = ?", subCondition).Find(res).Error
//		if len(subKey) != 0 && len(subCondition) != 0 {
//			return db.Where(filters[0], filters[1:]...).Limit(pageSize).Offset((page-1)*pageSize).Preload(key, subKey+" = ?", subCondition).Find(res).Error
//		} else {
//			return db.Where(filters[0], filters[1:]...).Limit(pageSize).Offset((page - 1) * pageSize).Preload(key).Find(res).Error
//		}
//	} else {
//		//return db.Limit(pageSize).Offset((page-1)*pageSize).Preload(key, subKey+" = ?", subCondition).Find(res).Error
//		if len(subKey) != 0 && len(subCondition) != 0 {
//			return db.Limit(pageSize).Offset((page-1)*pageSize).Preload(key, subKey+" = ?", subCondition).Find(res).Error
//		} else {
//			return db.Limit(pageSize).Offset((page - 1) * pageSize).Preload(key).Find(res).Error
//		}
//
//	}
//}
//
//func (o *Orm) PreloadCount(key string, ctx context.Context, res interface{}, total int64, filters ...interface{}) (int64, error) {
//	db := o.db.WithContext(ctx)
//	if len(filters) > 0 {
//		db.Where(filters[0], filters[1:]...).Preload(key).Find(res).Count(&total)
//		return total, nil
//	}
//	db.Preload(key).Find(res).Count(&total)
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
//func (o *Orm) Count(ctx context.Context, model interface{}, total *int64, filters ...interface{}) error {
//	db := o.db.WithContext(ctx).Model(&model)
//	if len(filters) > 0 {
//		return db.Where(filters[0], filters[1:]...).Count(total).Error
//	}
//	return db.Count(total).Error
//}
//
//func (o *Orm) CountIdx(ctx context.Context, model interface{}, selectColumns string, total *int64, filters ...interface{}) error {
//	//db := o.db.WithContext(ctx).Model(&model).Select("count(*) as total")
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
//	return o.db.Exec(sql, value).Error
//}
//
//func (o *Orm) Raw(sql string, result interface{}, value ...interface{}) error {
//	return o.db.Raw(sql, value...).Scan(result).Error
//}
//
//func (o *Orm) RawRows(sql string, value ...interface{}) (gorm.Rows, error) {
//	return o.db.Raw(sql, value).Rows()
//}
