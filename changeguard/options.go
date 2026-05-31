package changeguard

import "time"

type EngineOptions struct {
	Enabled                   bool
	Strict                    bool
	FailMode                  string
	ServiceName               string
	DefaultRiskLevel          string
	DefaultNotifyChannels     []string
	DefaultMaxDiffChanges     int
	DefaultSummaryFieldLimit  int
	DefaultSummaryValueLimit  int
	DefaultVersionEnabled     bool
	DefaultRollbackEnabled    bool
	DefaultRetentionDays      int
	DefaultDriftEnabled       bool
	DispatcherEnabled         bool
	DriftRunnerEnabled        bool
	RetentionEnabled          bool
	AllowNoSink               bool
	AllowNoVersionStore       bool
	AllowNoNotifier           bool
	RequestIDHeader           string
	AutoStartWorkers          bool
	NotificationWorkerEnabled bool
	DriftWorkerEnabled        bool
	NotificationPollInterval  time.Duration
	DriftPollInterval         time.Duration
}

func DefaultEngineOptions(serviceName string) EngineOptions {
	return EngineOptions{
		Enabled:                   true,
		Strict:                    false,
		FailMode:                  FailModeOpen,
		ServiceName:               serviceName,
		DefaultRiskLevel:          RiskLevelMedium,
		DefaultNotifyChannels:     []string{"sms"},
		DefaultMaxDiffChanges:     50,
		DefaultSummaryFieldLimit:  8,
		DefaultSummaryValueLimit:  64,
		DefaultVersionEnabled:     true,
		DefaultRollbackEnabled:    true,
		DefaultRetentionDays:      180,
		DefaultDriftEnabled:       true,
		DispatcherEnabled:         true,
		DriftRunnerEnabled:        true,
		RetentionEnabled:          true,
		AllowNoSink:               true,
		AllowNoVersionStore:       true,
		AllowNoNotifier:           true,
		RequestIDHeader:           "X-Request-Id",
		AutoStartWorkers:          true,
		NotificationWorkerEnabled: true,
		DriftWorkerEnabled:        true,
		NotificationPollInterval:  time.Minute,
		DriftPollInterval:         10 * time.Minute,
	}
}

func (o EngineOptions) normalize() EngineOptions {
	if o.FailMode == "" {
		o.FailMode = FailModeOpen
	}
	if o.DefaultRiskLevel == "" {
		o.DefaultRiskLevel = RiskLevelMedium
	}
	if len(o.DefaultNotifyChannels) == 0 {
		o.DefaultNotifyChannels = []string{"sms"}
	}
	if o.DefaultMaxDiffChanges <= 0 {
		o.DefaultMaxDiffChanges = 50
	}
	if o.DefaultSummaryFieldLimit <= 0 {
		o.DefaultSummaryFieldLimit = 8
	}
	if o.DefaultSummaryValueLimit <= 0 {
		o.DefaultSummaryValueLimit = 64
	}
	if o.DefaultRetentionDays <= 0 {
		o.DefaultRetentionDays = 180
	}
	if o.RequestIDHeader == "" {
		o.RequestIDHeader = "X-Request-Id"
	}
	if o.NotificationPollInterval <= 0 {
		o.NotificationPollInterval = time.Minute
	}
	if o.DriftPollInterval <= 0 {
		o.DriftPollInterval = 10 * time.Minute
	}
	return o
}
