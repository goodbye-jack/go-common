package changeguard

import (
	"context"
	"encoding/json"
	"time"

	"github.com/goodbye-jack/go-common/orm"
	"github.com/google/uuid"
)

type NoopSink struct{}

func (s *NoopSink) Emit(context.Context, ChangeEvent) error {
	return nil
}

type GormSink struct{}

func NewGormSink() *GormSink {
	return &GormSink{}
}

func (s *GormSink) Emit(ctx context.Context, event ChangeEvent) error {
	if orm.DB == nil {
		return nil
	}
	beforeJSON, _ := json.Marshal(event.BeforeMasked)
	afterJSON, _ := json.Marshal(event.AfterMasked)
	changesJSON, _ := json.Marshal(event.Changes)
	summaryJSON, _ := json.Marshal(event.Summary)
	metadataJSON, _ := json.Marshal(event.Metadata)
	record := &EventRecord{
		EventID:          chooseNonEmpty(event.EventID, uuid.NewString()),
		ServiceName:      event.ServiceName,
		ResourceKey:      event.ResourceKey,
		ResourceType:     event.ResourceType,
		ResourceID:       event.ResourceID,
		ResourceName:     event.ResourceName,
		Action:           event.Action,
		RiskLevel:        event.RiskLevel,
		Path:             event.Path,
		Method:           event.Method,
		RequestID:        event.RequestID,
		OperatorID:       event.Principal.UserID,
		OperatorName:     event.Principal.UserName,
		OperatorAccount:  event.Principal.UserAccount,
		TenantCode:       event.Principal.TenantCode,
		ClientIP:         event.Principal.ClientIP,
		ClientType:       truncateString(event.Principal.ClientType, 64),
		Success:          event.Success,
		BeforeMaskedJSON: string(beforeJSON),
		AfterMaskedJSON:  string(afterJSON),
		ChangesJSON:      string(changesJSON),
		SummaryJSON:      string(summaryJSON),
		VersionID:        event.VersionID,
		NotifyStatus:     chooseNonEmpty(event.NotifyStatus, "pending"),
		ProcessStatus:    "new",
		MetadataJSON:     string(metadataJSON),
		OccurredAtUnix:   chooseTime(event.OccurredAt).Unix(),
	}
	return orm.DB.Create(ctx, record)
}

func chooseNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func chooseTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now()
	}
	return value
}
