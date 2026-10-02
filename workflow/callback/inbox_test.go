package callback

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/goodbye-jack/go-common/workflow/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type businessCounter struct {
	ID    uint `gorm:"primaryKey"`
	Value int
}

func testInbox(t *testing.T) (*Inbox, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "inbox.db")+"?_busy_timeout=10000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	inbox, err := NewInbox(db, "test-service")
	if err != nil {
		t.Fatal(err)
	}
	if err := inbox.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&businessCounter{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&businessCounter{ID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	return inbox, db
}

func increment(tx *gorm.DB) error {
	return tx.Model(&businessCounter{}).Where("id = ?", 1).Update("value", gorm.Expr("value + 1")).Error
}

func assertCounter(t *testing.T, db *gorm.DB, want int) {
	t.Helper()
	var counter businessCounter
	if err := db.First(&counter, 1).Error; err != nil {
		t.Fatal(err)
	}
	if counter.Value != want {
		t.Fatalf("counter = %d, want %d", counter.Value, want)
	}
}

func TestInboxRollbackRetryAndPersistentDuplicate(t *testing.T) {
	inbox, db := testInbox(t)
	payload := &types.FlowableCallbackPayload{EventID: "event-1", EventType: "PROCESS_ENDED", ProcessInstanceID: "process-1"}
	failure := errors.New("business update failed")
	duplicate, err := inbox.Process(context.Background(), payload, func(tx *gorm.DB) error {
		if err := increment(tx); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) || duplicate {
		t.Fatalf("failed attempt = %v, %v", duplicate, err)
	}
	assertCounter(t, db, 0)
	var receipts int64
	if err := db.Model(&Receipt{}).Count(&receipts).Error; err != nil || receipts != 0 {
		t.Fatalf("rollback receipts = %d, %v", receipts, err)
	}
	duplicate, err = inbox.Process(context.Background(), payload, increment)
	if err != nil || duplicate {
		t.Fatalf("retry = %v, %v", duplicate, err)
	}
	assertCounter(t, db, 1)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := gorm.Open(db.Dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	reopenedPool, err := reopened.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopenedPool.Close() })
	restarted, err := NewInbox(reopened, "test-service")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err = restarted.Process(context.Background(), payload, increment)
	if err != nil || !duplicate {
		t.Fatalf("duplicate after reopening = %v, %v", duplicate, err)
	}
	assertCounter(t, reopened, 1)
}

func TestInboxConcurrentDuplicates(t *testing.T) {
	inbox, db := testInbox(t)
	var wait sync.WaitGroup
	var applied atomic.Int32
	for worker := 0; worker < 12; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := inbox.Process(context.Background(), &types.FlowableCallbackPayload{EventID: "concurrent"}, func(tx *gorm.DB) error {
				applied.Add(1)
				return increment(tx)
			})
			if err != nil {
				t.Errorf("concurrent callback: %v", err)
			}
		}()
	}
	wait.Wait()
	if applied.Load() != 1 {
		t.Fatalf("applied = %d", applied.Load())
	}
	assertCounter(t, db, 1)
}

func TestInboxConflictAndScopeIsolation(t *testing.T) {
	inbox, db := testInbox(t)
	payload := &types.FlowableCallbackPayload{EventID: "same-id", EventType: "PROCESS_STARTED"}
	if _, err := inbox.Process(context.Background(), payload, increment); err != nil {
		t.Fatal(err)
	}
	payload.EventType = "PROCESS_ENDED"
	if _, err := inbox.Process(context.Background(), payload, increment); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("conflict = %v", err)
	}
	other, err := NewInbox(db, "another-service")
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := other.Process(context.Background(), payload, increment); err != nil || duplicate {
		t.Fatalf("other scope = %v, %v", duplicate, err)
	}
	assertCounter(t, db, 2)
}

func TestInboxValidationAndCancellation(t *testing.T) {
	inbox, db := testInbox(t)
	for _, payload := range []*types.FlowableCallbackPayload{nil, {}, {EventID: " "}, {EventID: "bad-json", Variables: map[string]interface{}{"value": make(chan int)}}} {
		if _, err := inbox.Process(context.Background(), payload, increment); err == nil {
			t.Fatal("invalid payload accepted")
		}
	}
	if _, err := inbox.Process(context.Background(), &types.FlowableCallbackPayload{EventID: "no-handler"}, nil); err == nil {
		t.Fatal("nil handler accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := inbox.Process(ctx, &types.FlowableCallbackPayload{EventID: "cancelled"}, increment); err == nil {
		t.Fatal("cancelled transaction accepted")
	}
	assertCounter(t, db, 0)
}

func TestInboxReceiptFailureRollsBackBusiness(t *testing.T) {
	inbox, db := testInbox(t)
	failure := errors.New("receipt write failed")
	if err := db.Callback().Update().Before("gorm:update").Register("fail_receipt", func(tx *gorm.DB) {
		if tx.Statement.Table == (Receipt{}).TableName() {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := inbox.Process(context.Background(), &types.FlowableCallbackPayload{EventID: "receipt-failure"}, increment); !errors.Is(err, failure) {
		t.Fatalf("receipt failure = %v", err)
	}
	assertCounter(t, db, 0)
}

func TestInboxCommitFailureRollsBackReceipt(t *testing.T) {
	inbox, db := testInbox(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"CREATE TABLE commit_parents (id INTEGER PRIMARY KEY)",
		"CREATE TABLE commit_children (parent_id INTEGER REFERENCES commit_parents(id) DEFERRABLE INITIALLY DEFERRED)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	payload := &types.FlowableCallbackPayload{EventID: "commit-failure"}
	if duplicate, err := inbox.Process(context.Background(), payload, func(tx *gorm.DB) error {
		if err := increment(tx); err != nil {
			return err
		}
		return tx.Exec("INSERT INTO commit_children (parent_id) VALUES (999)").Error
	}); err == nil || duplicate {
		t.Fatalf("commit failure = %v, %v", duplicate, err)
	}
	assertCounter(t, db, 0)
	if duplicate, err := inbox.Process(context.Background(), payload, increment); err != nil || duplicate {
		t.Fatalf("retry after failed commit = %v, %v", duplicate, err)
	}
	assertCounter(t, db, 1)
}
