package changeguard

import (
	commonModel "github.com/goodbye-jack/go-common/model"
)

type EventRecord struct {
	commonModel.ModelBase
	EventID          string `gorm:"size:64;uniqueIndex" json:"event_id"`
	ServiceName      string `gorm:"size:64;index" json:"service_name"`
	ResourceKey      string `gorm:"size:128;index" json:"resource_key"`
	ResourceType     string `gorm:"size:128;index" json:"resource_type"`
	ResourceID       string `gorm:"size:128;index" json:"resource_id"`
	ResourceName     string `gorm:"size:255" json:"resource_name"`
	Action           string `gorm:"size:32;index" json:"action"`
	RiskLevel        string `gorm:"size:32;index" json:"risk_level"`
	Path             string `gorm:"size:255" json:"path"`
	Method           string `gorm:"size:16" json:"method"`
	RequestID        string `gorm:"size:128;index" json:"request_id"`
	OperatorID       string `gorm:"size:64;index" json:"operator_id"`
	OperatorName     string `gorm:"size:128" json:"operator_name"`
	OperatorAccount  string `gorm:"size:128" json:"operator_account"`
	TenantCode       string `gorm:"size:64;index" json:"tenant_code"`
	ClientIP         string `gorm:"size:64" json:"client_ip"`
	ClientType       string `gorm:"size:64" json:"client_type"`
	Success          bool   `gorm:"index" json:"success"`
	BeforeMaskedJSON string `gorm:"type:mediumtext" json:"before_masked_json"`
	AfterMaskedJSON  string `gorm:"type:mediumtext" json:"after_masked_json"`
	ChangesJSON      string `gorm:"type:mediumtext" json:"changes_json"`
	SummaryJSON      string `gorm:"type:text" json:"summary_json"`
	VersionID        string `gorm:"size:64;index" json:"version_id"`
	NotifyStatus     string `gorm:"size:32;index" json:"notify_status"`
	NotifyAttempts   int    `json:"notify_attempts"`
	LastNotifyError  string `gorm:"size:1024" json:"last_notify_error"`
	ProcessStatus    string `gorm:"size:32;index" json:"process_status"`
	MetadataJSON     string `gorm:"type:text" json:"metadata_json"`
	OccurredAtUnix   int64  `gorm:"index" json:"occurred_at_unix"`
}

type VersionRecord struct {
	commonModel.ModelBase
	VersionID     string `gorm:"size:64;uniqueIndex" json:"version_id"`
	ServiceName   string `gorm:"size:64;index" json:"service_name"`
	ResourceKey   string `gorm:"size:128;index" json:"resource_key"`
	ResourceType  string `gorm:"size:128;index" json:"resource_type"`
	ResourceID    string `gorm:"size:128;index" json:"resource_id"`
	ResourceName  string `gorm:"size:255" json:"resource_name"`
	VersionNo     int64  `gorm:"index" json:"version_no"`
	Action        string `gorm:"size:32" json:"action"`
	RiskLevel     string `gorm:"size:32" json:"risk_level"`
	EventID       string `gorm:"size:64;index" json:"event_id"`
	SnapshotJSON  string `gorm:"type:mediumtext" json:"snapshot_json"`
	SnapshotHash  string `gorm:"size:128;index" json:"snapshot_hash"`
	SchemaVersion string `gorm:"size:32" json:"schema_version"`
	OperatorID    string `gorm:"size:64" json:"operator_id"`
	OperatorName  string `gorm:"size:128" json:"operator_name"`
	TenantCode    string `gorm:"size:64" json:"tenant_code"`
	IsRollbackBase bool  `json:"is_rollback_base"`
	TagsJSON      string `gorm:"type:text" json:"tags_json"`
}

type DriftReportRecord struct {
	commonModel.ModelBase
	ReportID       string `gorm:"size:64;uniqueIndex" json:"report_id"`
	ServiceName    string `gorm:"size:64;index" json:"service_name"`
	ResourceKey    string `gorm:"size:128;index" json:"resource_key"`
	ResourceType   string `gorm:"size:128;index" json:"resource_type"`
	ResourceID     string `gorm:"size:128;index" json:"resource_id"`
	Severity       string `gorm:"size:32;index" json:"severity"`
	FindingsJSON   string `gorm:"type:mediumtext" json:"findings_json"`
	RuleKeysJSON   string `gorm:"type:text" json:"rule_keys_json"`
	Resolved       bool   `gorm:"index" json:"resolved"`
	ResolvedAtUnix int64  `json:"resolved_at_unix"`
	OccurredAtUnix int64  `gorm:"index" json:"occurred_at_unix"`
}

type SecondFactorChallengeRecord struct {
	commonModel.ModelBase
	ChallengeID         string `gorm:"size:64;uniqueIndex" json:"challenge_id"`
	ServiceName         string `gorm:"size:64;index" json:"service_name"`
	ScenarioKey         string `gorm:"size:128;index" json:"scenario_key"`
	ResourceKey         string `gorm:"size:128;index" json:"resource_key"`
	Action              string `gorm:"size:32;index" json:"action"`
	PrincipalUserID     string `gorm:"size:64;index" json:"principal_user_id"`
	PrincipalTenantCode string `gorm:"size:64;index" json:"principal_tenant_code"`
	Phone               string `gorm:"size:32;index" json:"phone"`
	MaskedPhone         string `gorm:"size:32" json:"masked_phone"`
	RequestDigest       string `gorm:"size:128;index" json:"request_digest"`
	SMSCode             string `gorm:"size:16" json:"sms_code"`
	ReplyToken          string `gorm:"size:16;index" json:"reply_token"`
	ApprovalStatus      string `gorm:"size:32;index" json:"approval_status"`
	VerifyAttempts      int    `json:"verify_attempts"`
	Verified            bool   `gorm:"index" json:"verified"`
	VerifiedAtUnix      int64  `json:"verified_at_unix"`
	ExpiresAtUnix       int64  `gorm:"index" json:"expires_at_unix"`
	LastSentAtUnix      int64  `json:"last_sent_at_unix"`
	ConsumedAtUnix      int64  `json:"consumed_at_unix"`
	MetadataJSON        string `gorm:"type:text" json:"metadata_json"`
}
