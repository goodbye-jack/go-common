package directorysync

import "time"

// RecordType 表示同步明细对应的记录类别。
type RecordType string

const (
	RecordTypeDepartment RecordType = "department"
	RecordTypePosition   RecordType = "position"
	RecordTypeUser       RecordType = "user"
	RecordTypeGroup      RecordType = "group"
)

// DetailStatus 表示单条同步明细的执行结果。
type DetailStatus string

const (
	DetailStatusSuccess DetailStatus = "success"
	DetailStatusFailed  DetailStatus = "failed"
	DetailStatusWarning DetailStatus = "warning"
	DetailStatusSkipped DetailStatus = "skipped"
)

// SyncOptions 描述一次同步执行的通用选项。
type SyncOptions struct {
	BatchNo          string
	TenantCode       string
	DryRun           bool
	ResetPassword    bool
	IncludeDisabled  bool
	OperatorUserID   string
	OperatorUserName string
}

// DepartmentRecord 是业务侧映射后的标准部门记录。
type DepartmentRecord struct {
	Code       string
	Name       string
	ParentCode string
	TenantCode string
	Enabled    bool
	Attributes map[string]string
}

// PositionRecord 是业务侧映射后的标准岗位记录。
type PositionRecord struct {
	Code           string
	Name           string
	DepartmentCode string
	TenantCode     string
	Enabled        bool
	Attributes     map[string]string
}

// UserRecord 是业务侧映射后的标准用户记录。
type UserRecord struct {
	UserID             string
	UserName           string
	DisplayName        string
	Email              string
	Mobile             string
	DepartmentCodes    []string
	PositionCodes      []string
	TenantCode         string
	Enabled            bool
	InitialPassword    string
	MustChangePassword bool
	Attributes         map[string]string
}

// GroupRecord 是业务侧映射后的标准 LDAP 组投影记录。
type GroupRecord struct {
	Code          string
	Name          string
	MemberUserIDs []string
	TenantCode    string
	Enabled       bool
	Attributes    map[string]string
}

// SyncDetail 描述一次同步中的单个动作结果。
type SyncDetail struct {
	RecordType RecordType   `json:"record_type"`
	RecordKey  string       `json:"record_key"`
	RecordName string       `json:"record_name"`
	Action     string       `json:"action"`
	Status     DetailStatus `json:"status"`
	Message    string       `json:"message"`
}

// SyncReport 是一次同步的最终报告。
type SyncReport struct {
	BatchNo         string       `json:"batch_no"`
	DryRun          bool         `json:"dry_run"`
	StartedAt       time.Time    `json:"started_at"`
	FinishedAt      time.Time    `json:"finished_at"`
	TenantCode      string       `json:"tenant_code,omitempty"`
	DepartmentTotal int          `json:"department_total"`
	PositionTotal   int          `json:"position_total"`
	UserTotal       int          `json:"user_total"`
	GroupTotal      int          `json:"group_total,omitempty"`
	SuccessCount    int          `json:"success_count"`
	FailedCount     int          `json:"failed_count"`
	WarningCount    int          `json:"warning_count"`
	Status          string       `json:"status"`
	Details         []SyncDetail `json:"details"`
}
