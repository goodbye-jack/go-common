package callback

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/goodbye-jack/go-common/workflow/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrEventConflict = errors.New("callback event ID reused with different payload")

// Receipt 与业务更新必须写入同一个事务数据库，不保存回调中的敏感变量。
type Receipt struct {
	ReceiptID   string     `gorm:"column:receipt_id;size:64;primaryKey"`
	Scope       string     `gorm:"column:scope;size:160;not null"`
	EventID     string     `gorm:"column:event_id;size:512;not null"`
	PayloadHash string     `gorm:"column:payload_hash;size:64;not null"`
	ProcessedAt *time.Time `gorm:"column:processed_at"`
}

func (Receipt) TableName() string { return "workflow_callback_receipts" }

type Inbox struct {
	db    *gorm.DB
	scope string
}

// NewInbox 不执行建表；应用应在接收回调之前显式调用 Migrate 并检查错误。
func NewInbox(db *gorm.DB, scope string) (*Inbox, error) {
	scope = strings.TrimSpace(scope)
	if db == nil {
		return nil, errors.New("callback inbox database is required")
	}
	if scope == "" || len(scope) > 160 {
		return nil, errors.New("callback inbox scope must contain 1 to 160 bytes")
	}
	return &Inbox{db: db, scope: scope}, nil
}

func (inbox *Inbox) Migrate(ctx context.Context) error {
	return inbox.db.WithContext(ctx).AutoMigrate(&Receipt{})
}

// Process 仅对使用 tx 的本地写入提供原子去重，不包含网络请求或其他数据库。
// duplicate 只在事务成功结束且事件已处理时为 true；事务失败不会留下成功回执。
func (inbox *Inbox) Process(ctx context.Context, payload *types.FlowableCallbackPayload, apply func(*gorm.DB) error) (duplicate bool, err error) {
	if payload == nil || strings.TrimSpace(payload.EventID) == "" || len(payload.EventID) > 512 {
		return false, errors.New("callback eventId must contain 1 to 512 bytes")
	}
	if apply == nil {
		return false, errors.New("callback transaction handler is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("encode callback payload: %w", err)
	}
	receiptID := hash([]byte(inbox.scope + "\x00" + payload.EventID))
	payloadHash := hash(encoded)
	err = inbox.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		receipt := Receipt{ReceiptID: receiptID, Scope: inbox.scope, EventID: payload.EventID, PayloadHash: payloadHash}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&receipt).Error; err != nil {
			return fmt.Errorf("create callback receipt: %w", err)
		}
		// 先插入再锁定，串行化相同事件；不能依赖 MySQL 的 RowsAffected 判断是否首次插入。
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("receipt_id = ?", receiptID).First(&receipt).Error; err != nil {
			return fmt.Errorf("lock callback receipt: %w", err)
		}
		if receipt.Scope != inbox.scope || receipt.EventID != payload.EventID || receipt.PayloadHash != payloadHash {
			return ErrEventConflict
		}
		if receipt.ProcessedAt != nil {
			duplicate = true
			return nil
		}
		if err := apply(tx); err != nil {
			return err
		}
		return tx.Model(&Receipt{}).Where("receipt_id = ?", receiptID).Update("processed_at", time.Now().UTC()).Error
	})
	if err != nil {
		return false, err
	}
	return duplicate, nil
}

func hash(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
