package changeguard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/goodbye-jack/go-common/orm"
	"github.com/google/uuid"
)

type VersionSnapshot struct {
	VersionID    string
	ServiceName  string
	ResourceKey  string
	ResourceType string
	ResourceID   string
	ResourceName string
	VersionNo    int64
	Snapshot     map[string]any
}

type SaveVersionRequest struct {
	ServiceName  string
	ResourceKey  string
	ResourceType string
	ResourceID   string
	ResourceName string
	Action       string
	RiskLevel    string
	Snapshot     map[string]any
	EventID      string
	Operator     Principal
	Tags         map[string]string
}

type VersionStore interface {
	Save(ctx context.Context, req SaveVersionRequest) (string, error)
	Latest(ctx context.Context, serviceName, resourceKey, resourceID string) (*VersionSnapshot, error)
	ByVersion(ctx context.Context, serviceName, resourceKey, resourceID string, versionNo int64) (*VersionSnapshot, error)
}

type NoopVersionStore struct{}

func (s *NoopVersionStore) Save(context.Context, SaveVersionRequest) (string, error) {
	return "", nil
}

func (s *NoopVersionStore) Latest(context.Context, string, string, string) (*VersionSnapshot, error) {
	return nil, nil
}

func (s *NoopVersionStore) ByVersion(context.Context, string, string, string, int64) (*VersionSnapshot, error) {
	return nil, nil
}

type GormVersionStore struct{}

func NewGormVersionStore() *GormVersionStore {
	return &GormVersionStore{}
}

func (s *GormVersionStore) Save(ctx context.Context, req SaveVersionRequest) (string, error) {
	if orm.DB == nil {
		return "", nil
	}
	snapshotJSON, _ := json.Marshal(req.Snapshot)
	tagsJSON, _ := json.Marshal(req.Tags)
	versionNo, err := s.nextVersionNo(ctx, req.ServiceName, req.ResourceKey, req.ResourceID)
	if err != nil {
		return "", err
	}
	versionID := uuid.NewString()
	hash := sha256.Sum256(snapshotJSON)
	record := &VersionRecord{
		VersionID:      versionID,
		ServiceName:    req.ServiceName,
		ResourceKey:    req.ResourceKey,
		ResourceType:   req.ResourceType,
		ResourceID:     req.ResourceID,
		ResourceName:   req.ResourceName,
		VersionNo:      versionNo,
		Action:         req.Action,
		RiskLevel:      req.RiskLevel,
		EventID:        req.EventID,
		SnapshotJSON:   string(snapshotJSON),
		SnapshotHash:   hex.EncodeToString(hash[:]),
		SchemaVersion:  "v1",
		OperatorID:     req.Operator.UserID,
		OperatorName:   req.Operator.UserName,
		TenantCode:     req.Operator.TenantCode,
		TagsJSON:       string(tagsJSON),
		IsRollbackBase: req.Action == ActionRollback,
	}
	return versionID, orm.DB.Create(ctx, record)
}

func (s *GormVersionStore) Latest(ctx context.Context, serviceName, resourceKey, resourceID string) (*VersionSnapshot, error) {
	if orm.DB == nil {
		return nil, nil
	}
	record := &VersionRecord{}
	err := orm.DB.GetDB().WithContext(ctx).
		Model(record).
		Where("service_name = ? AND resource_key = ? AND resource_id = ?", serviceName, resourceKey, resourceID).
		Order("version_no DESC").
		First(record).Error
	if err != nil {
		return nil, nil
	}
	return toVersionSnapshot(record)
}

func (s *GormVersionStore) ByVersion(ctx context.Context, serviceName, resourceKey, resourceID string, versionNo int64) (*VersionSnapshot, error) {
	if orm.DB == nil {
		return nil, nil
	}
	record := &VersionRecord{}
	err := orm.DB.GetDB().WithContext(ctx).
		Model(record).
		Where("service_name = ? AND resource_key = ? AND resource_id = ? AND version_no = ?", serviceName, resourceKey, resourceID, versionNo).
		First(record).Error
	if err != nil {
		return nil, nil
	}
	return toVersionSnapshot(record)
}

func (s *GormVersionStore) nextVersionNo(ctx context.Context, serviceName, resourceKey, resourceID string) (int64, error) {
	record := &VersionRecord{}
	err := orm.DB.GetDB().WithContext(ctx).
		Model(record).
		Where("service_name = ? AND resource_key = ? AND resource_id = ?", serviceName, resourceKey, resourceID).
		Order("version_no DESC").
		First(record).Error
	if err != nil {
		return 1, nil
	}
	return record.VersionNo + 1, nil
}

func toVersionSnapshot(record *VersionRecord) (*VersionSnapshot, error) {
	if record == nil {
		return nil, nil
	}
	result := map[string]any{}
	if record.SnapshotJSON != "" {
		if err := json.Unmarshal([]byte(record.SnapshotJSON), &result); err != nil {
			return nil, fmt.Errorf("unmarshal version snapshot failed: %w", err)
		}
	}
	return &VersionSnapshot{
		VersionID:    record.VersionID,
		ServiceName:  record.ServiceName,
		ResourceKey:  record.ResourceKey,
		ResourceType: record.ResourceType,
		ResourceID:   record.ResourceID,
		ResourceName: record.ResourceName,
		VersionNo:    record.VersionNo,
		Snapshot:     result,
	}, nil
}
