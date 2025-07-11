package dialect

import (
	_ "database/sql"
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
	"strings"
)

// DMDialector 达梦数据库方言
type DMDialector struct {
	gorm.Dialector
}

// NewDMDialector 创建达梦方言实例
func NewDMDialector(dsn string) gorm.Dialector {
	return &DMDialector{
		Dialector: postgres.New(postgres.Config{
			DriverName: "dm",
			DSN:        dsn,
		}),
	}
}

// 重写 BuildLimitAndOffset 方法，处理达梦的分页语法
func (d *DMDialector) BuildLimitAndOffset(clause *clause.Limit) (string, error) {
	if clause.Limit == nil && clause.Offset == 0 {
		return "", nil
	}
	limit := 0
	if clause.Limit != nil {
		limit = *clause.Limit
	}
	offset := clause.Offset
	if offset == 0 {
		// 只有 LIMIT，使用 TOP 语法
		return fmt.Sprintf("TOP %d", limit), nil
	}
	// 有 OFFSET，需要在钩子中处理，这里返回空
	return "", nil
}

// 重写 QuoteTo 方法，处理达梦的标识符引用
func (d *DMDialector) QuoteTo(writer clause.Writer, str string) {
	if str == "" {
		return
	}
	if strings.Contains(str, ".") { // 处理模式名和表名
		parts := strings.Split(str, ".")
		for idx, part := range parts {
			if idx > 0 {
				writer.WriteByte('.')
			}
			writer.WriteByte('"')
			writer.WriteString(part)
			writer.WriteByte('"')
		}
		return
	}
	writer.WriteByte('"')
	writer.WriteString(str)
	writer.WriteByte('"')
}

// 实现 BindVarTo 方法
func (d *DMDialector) BindVarTo(writer clause.Writer, _ *gorm.Statement, v interface{}) {
	writer.WriteByte('?')
}

// 重写 DataTypeOf 方法，处理达梦的数据类型映射
func (d *DMDialector) DataTypeOf(field *schema.Field) string {
	switch field.DataType {
	case "json":
		return "CLOB" // 达梦使用 CLOB 存储 JSON
	case "tinyint(1)":
		return "BOOLEAN" // 达梦使用 BOOLEAN 类型
	case "varchar":
		if field.Size == 0 {
			// 达梦 VARCHAR 必须指定长度
			return "VARCHAR(255)"
		}
		return fmt.Sprintf("VARCHAR(%d)", field.Size)
	case "text":
		return "CLOB" // 达梦使用 CLOB 替代 TEXT
	default:
		return d.Dialector.DataTypeOf(field)
	}
}

//// 重写 Migrator 方法，处理达梦的迁移逻辑
//func (d *DMDialector) Migrator(db *gorm.DB) gorm.Migrator {
//	return DMMigrator{
//		Migrator: db.Dialector.Migrator(db),
//	}
//}

// 修改后的 Migrator 方法
func (d *DMDialector) Migrator(db *gorm.DB) gorm.Migrator {
	return DMMigrator{
		Migrator: d.Dialector.Migrator(db),
	}
}

// DMMigrator 达梦数据库迁移器
type DMMigrator struct {
	gorm.Migrator
}

// FullDataTypeOf 重写数据类型映射
func (m DMMigrator) FullDataTypeOf(field *schema.Field) clause.Expr {
	// 处理达梦特有的数据类型限制
	if field.DataType == "varchar" && field.Size == 0 {
		field.Size = 255 // 默认长度
	}

	// 处理自增主键
	if field.AutoIncrement && field.PrimaryKey {
		if field.DataType == "int" || field.DataType == "bigint" {
			// 达梦使用 IDENTITY 关键字实现自增
			return clause.Expr{SQL: "NUMBER(?,?) IDENTITY(1,1)", Vars: []interface{}{field.Size, field.Precision}}
		}
	}

	// 调用父类方法获取基础类型
	expr := m.Migrator.FullDataTypeOf(field)

	// 处理其他需要调整的类型
	if field.DataType == "text" {
		expr.SQL = "CLOB" // 达梦使用 CLOB 替代 TEXT
	} else if field.DataType == "json" {
		expr.SQL = "CLOB" // 达梦使用 CLOB 存储 JSON
	}

	return expr
}
