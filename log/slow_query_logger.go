package log

import (
	"context"
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

func (l *SlowQueryLogger) Info(ctx context.Context, i ...interface{}) {
	fmt.Printf("Info: %s %v\n", ctx, i)
}

func (l *SlowQueryLogger) Warn(ctx context.Context, i ...interface{}) {
	fmt.Printf("Warn: %s %v\n", ctx, i)
}

func (l *SlowQueryLogger) Error(ctx context.Context, i ...interface{}) {
	fmt.Printf("Error: %s %v\n", ctx, i)
}

func (l *SlowQueryLogger) Trace(ctx interface{}, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	if elapsed > l.Threshold {
		sql, rows := fc()
		fmt.Printf("SLOW QUERY: %s took %v and affected %d rows\n", sql, elapsed, rows)
	}
}
