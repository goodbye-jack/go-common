package dialect

import (
	_ "database/sql"
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
	"strings"
)

type DMDialector struct {
	*postgres.Dialector
}

func NewDMDialector(dsn string) gorm.Dialector {
	return &DMDialector{
		Dialector: postgres.New(postgres.Config{
			DriverName: "dm",
			DSN:        dsn,
		}).(*postgres.Dialector),
	}
}

func (d *DMDialector) Migrator(db *gorm.DB) gorm.Migrator {
	return DMMigrator{
		Migrator: migrator.Migrator{
			Config: migrator.Config{
				DB:        db,
				Dialector: d,
			},
		},
	}
}

type DMMigrator struct {
	migrator.Migrator
}

// HasTable 检查表是否存在 (达梦兼容版本)
func (m DMMigrator) HasTable(value interface{}) bool {
	var count int64
	var tableName string

	if name, ok := value.(string); ok {
		tableName = name
	} else {
		stmt := &gorm.Statement{DB: m.DB}
		if err := stmt.Parse(value); err == nil {
			tableName = stmt.Table
		} else {
			return false
		}
	}
	// 达梦使用USER_TABLES而不是information_schema
	err := m.DB.Raw(`SELECT COUNT(*) FROM USER_TABLES WHERE TABLE_NAME = ?`,
		strings.ToUpper(tableName),
	).Row().Scan(&count)

	return err == nil && count > 0
}

func (m DMMigrator) CreateTable(values ...interface{}) error {
	// 实现 clause.Writer 的内部类型
	type stringBuilderWriter struct {
		*strings.Builder
	}
	for _, value := range values {
		if m.HasTable(value) {
			continue
		}
		if err := m.RunWithValue(value, func(stmt *gorm.Statement) error { // 正确的引号方法
			quote := func(str string) string {
				builder := &strings.Builder{}
				writer := &stringBuilderWriter{builder}
				m.DB.Dialector.QuoteTo(writer, str)
				return builder.String()
			}
			var (
				primaryFields []string
				createSQL     strings.Builder
			)
			createSQL.WriteString("CREATE TABLE ")
			createSQL.WriteString(quote(stmt.Table))
			createSQL.WriteString(" (")
			for _, field := range stmt.Schema.Fields {
				if field.AutoIncrement {
					createSQL.WriteString(fmt.Sprintf("%s %s IDENTITY(1,1),",
						quote(field.DBName),
						m.DataTypeOf(field)))
					continue
				}
				createSQL.WriteString(fmt.Sprintf("%s %s,",
					quote(field.DBName),
					m.DataTypeOf(field)))
				if field.Unique {
					createSQL.WriteString(fmt.Sprintf("CONSTRAINT uni_%s_%s UNIQUE (%s),",
						stmt.Table,
						field.DBName,
						quote(field.DBName)))
				}
				if field.PrimaryKey {
					primaryFields = append(primaryFields, quote(field.DBName))
				}
			}
			if len(primaryFields) > 0 {
				createSQL.WriteString("PRIMARY KEY (")
				createSQL.WriteString(strings.Join(primaryFields, ","))
				createSQL.WriteString(")")
			} else { // 移除最后一个逗号
				sqlStr := createSQL.String()
				if len(sqlStr) > 0 && sqlStr[len(sqlStr)-1] == ',' {
					sqlStr = sqlStr[:len(sqlStr)-1]
				}
				createSQL.Reset()
				createSQL.WriteString(sqlStr)
			}
			createSQL.WriteString(")")
			return m.DB.Exec(createSQL.String()).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

// DataTypeOf 数据类型映射 (达梦兼容版本)
func (d *DMDialector) DataTypeOf(field *schema.Field) string {
	switch field.DataType {
	case schema.String:
		if field.Size > 0 {
			return fmt.Sprintf("VARCHAR(%d)", field.Size)
		}
		return "VARCHAR(255)"
	case schema.Bool:
		return "BOOLEAN"
	case schema.Time:
		return "TIMESTAMP"
	case schema.Int, schema.Uint:
		if field.AutoIncrement {
			return "INT IDENTITY(1,1)"
		}
		return "INT"
	case schema.Float:
		return "FLOAT"
	case schema.Bytes:
		return "BLOB"
	default:
		return d.Dialector.DataTypeOf(field)
	}
}

//// DMDialector 达梦数据库方言
//type DMDialector struct {
//	gorm.Dialector
//}
//
//// NewDMDialector 创建达梦方言实例
//func NewDMDialector(dsn string) gorm.Dialector {
//	return &DMDialector{
//		Dialector: postgres.New(postgres.Config{
//			DriverName: "dm",
//			DSN:        dsn,
//		}),
//	}
//}
//
//// 重写 BuildLimitAndOffset 方法，处理达梦的分页语法
//func (d *DMDialector) BuildLimitAndOffset(clause *clause.Limit) (string, error) {
//	if clause.Limit == nil && clause.Offset == 0 {
//		return "", nil
//	}
//	limit := 0
//	if clause.Limit != nil {
//		limit = *clause.Limit
//	}
//	offset := clause.Offset
//	if offset == 0 {
//		// 只有 LIMIT，使用 TOP 语法
//		return fmt.Sprintf("TOP %d", limit), nil
//	}
//	// 有 OFFSET，需要在钩子中处理，这里返回空
//	return "", nil
//}
//
//// 重写 QuoteTo 方法，处理达梦的标识符引用
//func (d *DMDialector) QuoteTo(writer clause.Writer, str string) {
//	if str == "" {
//		return
//	}
//	if strings.Contains(str, ".") { // 处理模式名和表名
//		parts := strings.Split(str, ".")
//		for idx, part := range parts {
//			if idx > 0 {
//				writer.WriteByte('.')
//			}
//			writer.WriteByte('"')
//			writer.WriteString(part)
//			writer.WriteByte('"')
//		}
//		return
//	}
//	writer.WriteByte('"')
//	writer.WriteString(str)
//	writer.WriteByte('"')
//}
//
//// 实现 BindVarTo 方法
//func (d *DMDialector) BindVarTo(writer clause.Writer, _ *gorm.Statement, v interface{}) {
//	writer.WriteByte('?')
//}
//
//// 重写 DataTypeOf 方法，处理达梦的数据类型映射
//func (d *DMDialector) DataTypeOf(field *schema.Field) string {
//	switch field.DataType {
//	case "json":
//		return "CLOB" // 达梦使用 CLOB 存储 JSON
//	case "tinyint(1)":
//		return "BOOLEAN" // 达梦使用 BOOLEAN 类型
//	case "varchar":
//		if field.Size == 0 {
//			// 达梦 VARCHAR 必须指定长度
//			return "VARCHAR(255)"
//		}
//		return fmt.Sprintf("VARCHAR(%d)", field.Size)
//	case "text":
//		return "CLOB" // 达梦使用 CLOB 替代 TEXT
//	default:
//		return d.Dialector.DataTypeOf(field)
//	}
//}
//
////// 重写 Migrator 方法，处理达梦的迁移逻辑
////func (d *DMDialector) Migrator(db *gorm.DB) gorm.Migrator {
////	return DMMigrator{
////		Migrator: db.Dialector.Migrator(db),
////	}
////}
//
//// 修改后的 Migrator 方法
//func (d *DMDialector) Migrator(db *gorm.DB) gorm.Migrator {
//	return DMMigrator{
//		Migrator: d.Dialector.Migrator(db),
//	}
//}
//
//// DMMigrator 达梦数据库迁移器
//type DMMigrator struct {
//	gorm.Migrator
//}
//
//// FullDataTypeOf 重写数据类型映射
//func (m DMMigrator) FullDataTypeOf(field *schema.Field) clause.Expr {
//	// 处理达梦特有的数据类型限制
//	if field.DataType == "varchar" && field.Size == 0 {
//		field.Size = 255 // 默认长度
//	}
//
//	// 处理自增主键
//	if field.AutoIncrement && field.PrimaryKey {
//		if field.DataType == "int" || field.DataType == "bigint" {
//			// 达梦使用 IDENTITY 关键字实现自增
//			return clause.Expr{SQL: "NUMBER(?,?) IDENTITY(1,1)", Vars: []interface{}{field.Size, field.Precision}}
//		}
//	}
//
//	// 调用父类方法获取基础类型
//	expr := m.Migrator.FullDataTypeOf(field)
//
//	// 处理其他需要调整的类型
//	if field.DataType == "text" {
//		expr.SQL = "CLOB" // 达梦使用 CLOB 替代 TEXT
//	} else if field.DataType == "json" {
//		expr.SQL = "CLOB" // 达梦使用 CLOB 存储 JSON
//	}
//
//	return expr
//}
