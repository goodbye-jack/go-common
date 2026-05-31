package changeguard

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

type ResourceProfile struct {
	Key          string
	Name         string
	Enabled      bool
	ResourceType string
	ProviderType string
	PolicyName   string
	ModelValue   any
	FetcherName  string
	CustomKey    string
	RequestKeys  []string
	LookupKeys   []string
	BatchKeys    []string
	StatusField  string
	Metadata     map[string]string
}

type RouteBinding struct {
	Key          string
	Path         string
	Methods      []string
	ResourceKey  string
	Action       string
	Enabled      bool
	GuardPolicy  string
	OverrideRisk string
	Metadata     map[string]string
}

type PolicyProfile struct {
	Name                    string
	Enabled                 bool
	RiskLevel               string
	FailMode                string
	IncludeFields           []string
	IgnoreFields            []string
	SensitiveFields         []string
	ChangedOnlyFields       []string
	DisplayNames            map[string]string
	SliceToMapBy            map[string]string
	MaxDiffChanges          int
	SummaryFieldLimit       int
	SummaryValueLimit       int
	NotifyChannels          []string
	NotifyTemplate          string
	NotifyOnlyOnSuccess     bool
	NotifyOnActions         []string
	NotifyOnRiskLevels      []string
	VersioningEnabled       bool
	VersionOnActions        []string
	RollbackEnabled         bool
	RollbackRequireGuard    bool
	RetentionDays           int
	VersionSnapshotMode     string
	DriftCheckEnabled       bool
	DriftCheckPolicy        string
	DriftBaselineMode       string
	DriftFields             []string
	DriftThresholds         map[string]string
	RequirePasswordReverify bool
	RequireSecondFactor     bool
	SecondFactorMode        string
	SecondFactorOnActions   []string
	SuccessHTTPStatuses     []int
	SuccessJSONPaths        []string
	FailureJSONPaths        []string
	Tags                    map[string]string
}

type SecondFactorResult struct {
	Allowed         bool
	Responded       bool
	HTTPStatus      int
	ResponseCode    string
	ResponseMessage string
	ResponseData    map[string]any
}

type Principal struct {
	UserID      string
	UserName    string
	UserAccount string
	TenantCode  string
	Roles       []string
	ClientIP    string
	ClientType  string
}

type RequestMeta struct {
	RequestID   string
	Path        string
	Method      string
	QueryString string
	RawBody     []byte
}

type ResponseMeta struct {
	StatusCode int
}

type Session struct {
	RequestID    string
	StartedAt    time.Time
	Context      *gin.Context
	Principal    Principal
	Binding      RouteBinding
	Resource     ResourceProfile
	Policy       PolicyProfile
	Action       string
	Store        map[string]any
	RequestMeta  RequestMeta
	ResponseMeta ResponseMeta
}

type ResourceState struct {
	ResourceType string
	ResourceID   string
	ResourceName string
	Value        map[string]any
	RawValue     any
	Metadata     map[string]any
}

type FieldChange struct {
	Path        string `json:"path"`
	DisplayName string `json:"display_name"`
	ChangeType  string `json:"change_type"`
	Sensitive   bool   `json:"sensitive"`
	Before      any    `json:"before,omitempty"`
	After       any    `json:"after,omitempty"`
}

type DiffResult struct {
	Changed      bool
	Changes      []FieldChange
	BeforeMasked map[string]any
	AfterMasked  map[string]any
	Summary      []string
}

type ChangeEvent struct {
	EventID      string
	ServiceName  string
	PolicyName   string
	ResourceKey  string
	ResourceType string
	ResourceID   string
	ResourceName string
	Action       string
	RiskLevel    string
	Path         string
	Method       string
	Success      bool
	OccurredAt   time.Time
	Principal    Principal
	BeforeMasked map[string]any
	AfterMasked  map[string]any
	Changes      []FieldChange
	Summary      []string
	VersionID    string
	NotifyStatus string
	Metadata     map[string]string
	RequestID    string
}

type Provider interface {
	Before(*Session) (*ResourceState, error)
	After(*Session) (*ResourceState, error)
}

type SingletonFetcher func(ctx context.Context) (any, error)

type CustomProvider interface {
	Before(*Session) (*ResourceState, error)
	After(*Session) (*ResourceState, error)
}

type EventSink interface {
	Emit(context.Context, ChangeEvent) error
}

type BaselineResolver interface {
	Resolve(ctx context.Context, engine *Engine, resource ResourceProfile, policy PolicyProfile, current *ResourceState) (*ResourceState, []string, error)
}
