package orm

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// KingbaseTimeParserPlugin 修正版时间解析插件
type KingbaseTimeParserPlugin struct{}

func (p *KingbaseTimeParserPlugin) Name() string {
	return "kingbase_time_parser"
}

func (p *KingbaseTimeParserPlugin) Initialize(db *gorm.DB) error {
	// 注册回调到查询和行查询之前
	db.Callback().Query().Before("gorm:query").Register("kingbase:parse_time", p.beforeQuery)
	db.Callback().Row().Before("gorm:row").Register("kingbase:parse_time", p.beforeQuery)
	return nil
}

func (p *KingbaseTimeParserPlugin) beforeQuery(db *gorm.DB) {
	// 替换时间类型处理器
	stmt := db.Statement
	if stmt.Schema == nil {
		stmt.Parse(stmt.Model)
	}

	if stmt.Schema != nil {
		for _, field := range stmt.Schema.Fields {
			// 检查是否是 time.Time 或 *time.Time 类型
			if field.FieldType == reflect.TypeOf(time.Time{}) ||
				field.FieldType == reflect.TypeOf(&time.Time{}) {

				// 使用新版 GORM 的字段值设置方法
				field.ReflectValueOf = func(ctx context.Context, value reflect.Value) reflect.Value {
					// 创建自定义扫描器
					return reflect.ValueOf(&kingbaseTimeScanner{
						field: field,
						dest:  value,
					})
				}
			}
		}
	}
}

// kingbaseTimeScanner 自定义时间扫描器
type kingbaseTimeScanner struct {
	field *schema.Field
	dest  reflect.Value
}

func (s *kingbaseTimeScanner) Scan(value interface{}) error {
	if value == nil {
		// 处理 NULL 值
		if s.field.FieldType.Kind() == reflect.Ptr {
			s.dest.Set(reflect.Zero(s.field.FieldType))
		} else {
			s.dest.Set(reflect.ValueOf(time.Time{}))
		}
		return nil
	}

	var t time.Time
	var err error

	switch v := value.(type) {
	case time.Time:
		t = v
	case []byte:
		t, err = parseKingbaseTime(string(v))
	case string:
		t, err = parseKingbaseTime(v)
	default:
		return fmt.Errorf("unsupported time type: %T", value)
	}

	if err != nil {
		return err
	}

	// 设置值到目标字段
	if s.field.FieldType.Kind() == reflect.Ptr {
		s.dest.Set(reflect.ValueOf(&t))
	} else {
		s.dest.Set(reflect.ValueOf(t))
	}

	return nil
}

// 解析人大金仓时间格式
func parseKingbaseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	layouts := []string{
		"2006-01-02 15:04:05.999",       // 含毫秒
		"2006-01-02 15:04:05",           // 不含毫秒
		"2006-01-02T15:04:05.999Z07:00", // 含时区
		time.RFC3339Nano,                // 标准格式
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("failed to parse kingbase time: %s", s)
}
