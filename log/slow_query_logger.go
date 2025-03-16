package log

import (
	"fmt"
	"gorm.io/gorm/logger"
	"time"
)

type SlowQueryLogger struct {
	Threshold time.Duration // 慢查询阈值
}

func (l *SlowQueryLogger) LogMode(logger.LogLevel) logger.Interface {
	panic("implement me")
}

// Info(context. Context, string, ...interface{})
func (l *SlowQueryLogger) Info(ctx string, i ...interface{}) {
	fmt.Printf("Info: %s %v\n", ctx, i)
}

func (l *SlowQueryLogger) Warn(ctx string, i ...interface{}) {
	fmt.Printf("Warn: %s %v\n", ctx, i)
}

func (l *SlowQueryLogger) Error(ctx string, i ...interface{}) {
	fmt.Printf("Error: %s %v\n", ctx, i)
}

func (l *SlowQueryLogger) Trace(ctx interface{}, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	if elapsed > l.Threshold {
		sql, rows := fc()
		fmt.Printf("SLOW QUERY: %s took %v and affected %d rows\n", sql, elapsed, rows)
	}
}
