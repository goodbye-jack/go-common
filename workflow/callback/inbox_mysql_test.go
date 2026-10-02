package callback

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/goodbye-jack/go-common/workflow/types"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 仅使用显式指定的测试实例，创建随机隔离库；不读写 DSN 原有业务库。
func TestInboxMySQLIntegration(t *testing.T) {
	dsn := os.Getenv("WORKFLOW_CALLBACK_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set WORKFLOW_CALLBACK_TEST_MYSQL_DSN for isolated MySQL validation")
	}
	connection, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid MySQL test DSN")
	}
	connection.DBName = ""
	connection.ParseTime = true
	connection.Timeout = 5 * time.Second
	connection.ReadTimeout = 10 * time.Second
	connection.WriteTimeout = 10 * time.Second
	admin, err := gorm.Open(mysql.Open(connection.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect MySQL test instance: %v", err)
	}
	adminPool, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminPool.Close() })
	schema := fmt.Sprintf("callback_test_%x", time.Now().UnixNano())
	if err := admin.Exec("CREATE DATABASE " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP DATABASE " + schema).Error; err != nil {
			t.Errorf("remove isolated test schema %s: %v", schema, err)
		}
	})
	connection.DBName = schema
	for _, foundRows := range []bool{false, true} {
		t.Run(fmt.Sprintf("clientFoundRows=%t", foundRows), func(t *testing.T) {
			connection.ClientFoundRows = foundRows
			db, err := gorm.Open(mysql.Open(connection.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			pool, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = pool.Close() })
			pool.SetMaxOpenConns(16)
			inbox, err := NewInbox(db, t.Name())
			if err != nil {
				t.Fatal(err)
			}
			if err := inbox.Migrate(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&businessCounter{}); err != nil {
				t.Fatal(err)
			}
			if err := db.Where("id = ?", 1).Delete(&businessCounter{}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&businessCounter{ID: 1}).Error; err != nil {
				t.Fatal(err)
			}
			payload := &types.FlowableCallbackPayload{EventID: "mysql-event", EventType: "PROCESS_ENDED"}
			failure := errors.New("injected business failure")
			if _, err := inbox.Process(context.Background(), payload, func(tx *gorm.DB) error {
				if err := increment(tx); err != nil {
					return err
				}
				return failure
			}); !errors.Is(err, failure) {
				t.Fatalf("rollback error: %v", err)
			}
			assertCounter(t, db, 0)
			var wait sync.WaitGroup
			var applied atomic.Int32
			for worker := 0; worker < 12; worker++ {
				wait.Add(1)
				go func() {
					defer wait.Done()
					_, err := inbox.Process(context.Background(), payload, func(tx *gorm.DB) error { applied.Add(1); return increment(tx) })
					if err != nil {
						t.Errorf("concurrent retry: %v", err)
					}
				}()
			}
			wait.Wait()
			assertCounter(t, db, 1)
			if applied.Load() != 1 {
				t.Fatalf("applied = %d, want 1", applied.Load())
			}
			payload.EventType = "PROCESS_STARTED"
			if _, err := inbox.Process(context.Background(), payload, increment); !errors.Is(err, ErrEventConflict) {
				t.Fatalf("payload conflict = %v", err)
			}
			assertCounter(t, db, 1)
		})
	}
}
