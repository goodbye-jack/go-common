package orm

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/goodbye-jack/go-common/log"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type kingbaseTimeScanner struct {
	field *schema.Field
	dest  reflect.Value // 存储字段地址的反射值
}

func (s *kingbaseTimeScanner) Scan(value interface{}) error {
	if value == nil {
		if s.field.FieldType.Kind() == reflect.Ptr {
			s.dest.Elem().Set(reflect.Zero(s.field.FieldType))
		} else {
			s.dest.Elem().Set(reflect.ValueOf(time.Time{}))
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

	log.Info("Time=%v", t)
	elem := s.dest.Elem()
	if s.field.FieldType.Kind() == reflect.Ptr {
		newT := t // 创建副本避免指针逃逸
		elem.Set(reflect.ValueOf(&newT))
	} else {
		elem.Set(reflect.ValueOf(t))
	}
	return nil
}

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
	stmt := db.Statement
	if stmt.Schema == nil {
		stmt.Parse(stmt.Model)
	}

	if stmt.Schema != nil {
		for _, field := range stmt.Schema.Fields {
			if field.FieldType == reflect.TypeOf(time.Time{}) || field.FieldType == reflect.TypeOf(&time.Time{}) {
				// 使用局部变量避免闭包捕获问题
				currentField := field
				field.ReflectValueOf = func(ctx context.Context, value reflect.Value) reflect.Value {
					if value.CanAddr() {
						// 存储字段地址而非字段值
						return reflect.ValueOf(&kingbaseTimeScanner{
							field: currentField,
							dest:  value.Addr(),
						})
					}
					// 无法寻址时回退到默认行为
					return value
				}
			}
		}
	}
}
