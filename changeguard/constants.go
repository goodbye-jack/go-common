package changeguard

const (
	ProviderSingletonConfig = "singleton_config"
	ProviderGormEntitySave  = "gorm_entity_save"
	ProviderGormEntityToggle = "gorm_entity_toggle"
	ProviderGormBatchSave   = "gorm_batch_save"
	ProviderCustomFetcher   = "custom_fetcher"
)

const (
	FailModeOpen  = "fail_open"
	FailModeClose = "fail_close"
)

const (
	RiskLevelLow      = "low"
	RiskLevelMedium   = "medium"
	RiskLevelHigh     = "high"
	RiskLevelCritical = "critical"
)

const (
	ActionSave    = "save"
	ActionPublish = "publish"
	ActionEnable  = "enable"
	ActionDisable = "disable"
	ActionDelete  = "delete"
	ActionRestore = "restore"
	ActionUpdate  = "update"
	ActionToggle  = "toggle"
	ActionRollback = "rollback"
)

const (
	SecondFactorModeSMSCode        = "sms_code"
	SecondFactorModeSMSReply       = "sms_reply"
	SecondFactorModeSMSCodeOrReply = "sms_code_or_reply"
)

const (
	SecondFactorStatusPending  = "pending"
	SecondFactorStatusVerified = "verified"
	SecondFactorStatusApproved = "approved"
	SecondFactorStatusRejected = "rejected"
	SecondFactorStatusConsumed = "consumed"
)
