package changeguard

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/goodbye-jack/go-common/config"
	goodhttp "github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/log"
	commonsms "github.com/goodbye-jack/go-common/notify/sms"
	"github.com/goodbye-jack/go-common/orm"
	"github.com/spf13/viper"
)

type AppSpec struct {
	ServiceName string
	UserModel   any
	// Fetchers 允许业务侧直接传入具体函数，go-common 会自动适配成 SingletonFetcher。
	// 这样业务侧无需再写一层 context/any 的包装代码。
	Fetchers           map[string]any
	Models             map[string]any
	CustomProviders    map[string]CustomProvider
	RecipientResolvers map[string]RecipientResolver
}

type ConfigSpec struct {
	Enabled   bool             `mapstructure:"enabled"`
	Runtime   EngineConfig     `mapstructure:"runtime"`
	Providers ProvidersConfig  `mapstructure:"providers"`
	Scenarios []ScenarioConfig `mapstructure:"scenarios"`
}

type EngineConfig struct {
	Strict                    *bool    `mapstructure:"strict"`
	FailMode                  string   `mapstructure:"fail_mode"`
	DefaultRiskLevel          string   `mapstructure:"default_risk_level"`
	DefaultNotifyChannels     []string `mapstructure:"default_notify_channels"`
	DefaultMaxDiffChanges     int      `mapstructure:"default_max_diff_changes"`
	DefaultSummaryFieldLimit  int      `mapstructure:"default_summary_field_limit"`
	DefaultSummaryValueLimit  int      `mapstructure:"default_summary_value_limit"`
	DefaultVersionEnabled     *bool    `mapstructure:"default_version_enabled"`
	DefaultRollbackEnabled    *bool    `mapstructure:"default_rollback_enabled"`
	DefaultRetentionDays      int      `mapstructure:"default_retention_days"`
	DefaultDriftEnabled       *bool    `mapstructure:"default_drift_enabled"`
	DispatcherEnabled         *bool    `mapstructure:"dispatcher_enabled"`
	DriftRunnerEnabled        *bool    `mapstructure:"drift_runner_enabled"`
	RetentionEnabled          *bool    `mapstructure:"retention_enabled"`
	AllowNoSink               *bool    `mapstructure:"allow_no_sink"`
	AllowNoVersionStore       *bool    `mapstructure:"allow_no_version_store"`
	AllowNoNotifier           *bool    `mapstructure:"allow_no_notifier"`
	RequestIDHeader           string   `mapstructure:"request_id_header"`
	AutoStartWorkers          *bool    `mapstructure:"auto_start_workers"`
	NotificationWorkerEnabled *bool    `mapstructure:"notification_worker_enabled"`
	DriftWorkerEnabled        *bool    `mapstructure:"drift_worker_enabled"`
	NotificationPollInterval  string   `mapstructure:"notification_poll_interval"`
	DriftPollInterval         string   `mapstructure:"drift_poll_interval"`
}

type AdminUserRecipientOptions struct {
	UserModel   any
	UserType    string
	Status      string
	PhoneField  string
	TenantField string
}

type adminUserRecipientResolver struct {
	opts AdminUserRecipientOptions
}

type configPrefixSMSResolver struct {
	prefix string
}

func BindFromConfig(server *goodhttp.HTTPServer, spec AppSpec) error {
	if server == nil {
		return nil
	}
	if viper.Get("changeguard") == nil {
		return nil
	}
	cfg := ConfigSpec{}
	if err := decodeConfigSpec(&cfg); err != nil {
		// changeguard 属于增强能力，配置解析失败时仅告警，不能拖垮业务启动。
		log.Warnf("changeguard config decode skipped: %v", err)
		return nil
	}
	if !cfg.Enabled {
		return nil
	}
	serviceName := strings.TrimSpace(spec.ServiceName)
	if serviceName == "" {
		serviceName = config.GetAppName()
	}
	return bindFromV2Config(server, spec, cfg, serviceName)
}

func NewConfigPrefixSMSResolver(prefix string) SMSConfigResolver {
	return &configPrefixSMSResolver{prefix: strings.TrimSpace(prefix)}
}

func (r *configPrefixSMSResolver) Resolve(context.Context) (commonsms.Config, error) {
	prefix := strings.TrimSpace(r.prefix)
	if prefix == "" {
		prefix = "notifications.sms"
	}
	cfg := commonsms.DefaultConfig()
	cfg.Enabled = config.GetConfigBool(prefix + ".enabled")
	cfg.Provider = config.GetConfigString(prefix + ".provider")
	cfg.DefaultSign = config.GetConfigString(prefix + ".default_sign")
	cfg.DefaultTemplate = config.GetConfigString(prefix + ".default_template")
	cfg.Mock.Enabled = config.GetConfigBool(prefix + ".providers.mock.enabled")
	cfg.Luosimao.Enabled = config.GetConfigBool(prefix + ".providers.luosimao.enabled")
	cfg.Luosimao.APIKey = config.GetConfigString(prefix + ".providers.luosimao.api_key")
	cfg.Luosimao.Sign = config.GetConfigString(prefix + ".providers.luosimao.sign_name")
	cfg.Luosimao.Endpoint = config.GetConfigString(prefix + ".providers.luosimao.endpoint")
	cfg.Aliyun.Enabled = config.GetConfigBool(prefix + ".providers.aliyun.enabled")
	cfg.Aliyun.AccessKeyID = config.GetConfigString(prefix + ".providers.aliyun.access_key_id")
	cfg.Aliyun.AccessKeySecret = config.GetConfigString(prefix + ".providers.aliyun.access_key_secret")
	cfg.Aliyun.Endpoint = config.GetConfigString(prefix + ".providers.aliyun.endpoint")
	cfg.Aliyun.SignName = config.GetConfigString(prefix + ".providers.aliyun.sign_name")
	cfg.Aliyun.TemplateCode = config.GetConfigString(prefix + ".providers.aliyun.template_code")
	cfg.Normalize()
	return cfg, cfg.Validate()
}

func AdminUserRecipients(opts AdminUserRecipientOptions) RecipientResolver {
	return &adminUserRecipientResolver{opts: opts}
}

func (r *adminUserRecipientResolver) Resolve(ctx context.Context, event ChangeEvent, channel string) ([]string, error) {
	if strings.TrimSpace(channel) != "sms" || orm.DB == nil || r == nil || r.opts.UserModel == nil {
		return nil, nil
	}
	userType := firstNonBlank(r.opts.UserType, "admin")
	status := firstNonBlank(r.opts.Status, "normal")
	phoneField := firstNonBlank(r.opts.PhoneField, "phone")
	tenantField := firstNonBlank(r.opts.TenantField, "tenant_code")
	rows := make([]map[string]any, 0)
	db := orm.DB.GetDB().WithContext(ctx).Model(r.opts.UserModel).
		Select(phoneField).
		Where("user_type = ? AND status = ?", userType, status)
	if strings.TrimSpace(event.Principal.TenantCode) != "" {
		db = db.Where(tenantField+" = ?", event.Principal.TenantCode)
	}
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]string, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		phone, ok := findMapValueCaseInsensitive(row, phoneField)
		if !ok {
			continue
		}
		value := strings.TrimSpace(fmt.Sprint(phone))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func decodeConfigSpec(target *ConfigSpec) error {
	if target == nil {
		return nil
	}
	if err := viper.UnmarshalKey("changeguard", target); err != nil {
		return fmt.Errorf("decode changeguard config failed: %w", err)
	}
	return nil
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildEngineOptionsFromConfig(serviceName string, cfg EngineConfig) EngineOptions {
	opts := DefaultEngineOptions(serviceName)
	if cfg.Strict != nil {
		opts.Strict = *cfg.Strict
	}
	if strings.TrimSpace(cfg.FailMode) != "" {
		opts.FailMode = strings.TrimSpace(cfg.FailMode)
	}
	if strings.TrimSpace(cfg.DefaultRiskLevel) != "" {
		opts.DefaultRiskLevel = strings.TrimSpace(cfg.DefaultRiskLevel)
	}
	if len(cfg.DefaultNotifyChannels) > 0 {
		opts.DefaultNotifyChannels = append([]string{}, cfg.DefaultNotifyChannels...)
	}
	if cfg.DefaultMaxDiffChanges > 0 {
		opts.DefaultMaxDiffChanges = cfg.DefaultMaxDiffChanges
	}
	if cfg.DefaultSummaryFieldLimit > 0 {
		opts.DefaultSummaryFieldLimit = cfg.DefaultSummaryFieldLimit
	}
	if cfg.DefaultSummaryValueLimit > 0 {
		opts.DefaultSummaryValueLimit = cfg.DefaultSummaryValueLimit
	}
	if cfg.DefaultVersionEnabled != nil {
		opts.DefaultVersionEnabled = *cfg.DefaultVersionEnabled
	}
	if cfg.DefaultRollbackEnabled != nil {
		opts.DefaultRollbackEnabled = *cfg.DefaultRollbackEnabled
	}
	if cfg.DefaultRetentionDays > 0 {
		opts.DefaultRetentionDays = cfg.DefaultRetentionDays
	}
	if cfg.DefaultDriftEnabled != nil {
		opts.DefaultDriftEnabled = *cfg.DefaultDriftEnabled
	}
	if cfg.DispatcherEnabled != nil {
		opts.DispatcherEnabled = *cfg.DispatcherEnabled
	}
	if cfg.DriftRunnerEnabled != nil {
		opts.DriftRunnerEnabled = *cfg.DriftRunnerEnabled
	}
	if cfg.RetentionEnabled != nil {
		opts.RetentionEnabled = *cfg.RetentionEnabled
	}
	if cfg.AllowNoSink != nil {
		opts.AllowNoSink = *cfg.AllowNoSink
	}
	if cfg.AllowNoVersionStore != nil {
		opts.AllowNoVersionStore = *cfg.AllowNoVersionStore
	}
	if cfg.AllowNoNotifier != nil {
		opts.AllowNoNotifier = *cfg.AllowNoNotifier
	}
	if strings.TrimSpace(cfg.RequestIDHeader) != "" {
		opts.RequestIDHeader = strings.TrimSpace(cfg.RequestIDHeader)
	}
	if cfg.AutoStartWorkers != nil {
		opts.AutoStartWorkers = *cfg.AutoStartWorkers
	}
	if cfg.NotificationWorkerEnabled != nil {
		opts.NotificationWorkerEnabled = *cfg.NotificationWorkerEnabled
	}
	if cfg.DriftWorkerEnabled != nil {
		opts.DriftWorkerEnabled = *cfg.DriftWorkerEnabled
	}
	if duration, ok := parseOptionalDuration(cfg.NotificationPollInterval); ok {
		opts.NotificationPollInterval = duration
	}
	if duration, ok := parseOptionalDuration(cfg.DriftPollInterval); ok {
		opts.DriftPollInterval = duration
	}
	return opts
}

func parseOptionalDuration(raw string) (time.Duration, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, false
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		log.Warnf("changeguard duration config ignored: value=%s, err=%v", value, err)
		return 0, false
	}
	return duration, true
}

func registerFetchersFromSpec(engine *Engine, spec AppSpec) {
	if engine == nil {
		return
	}
	for name, fetcher := range spec.Fetchers {
		adapted, ok := adaptSingletonFetcher(fetcher)
		if !ok {
			log.Warnf("changeguard fetcher skipped: name=%s, reason=unsupported signature", name)
			continue
		}
		engine.RegisterSingletonFetcher(name, adapted)
	}
}

func registerCustomProvidersFromSpec(engine *Engine, spec AppSpec) {
	if engine == nil {
		return
	}
	for name, provider := range spec.CustomProviders {
		if provider == nil {
			log.Warnf("changeguard custom provider skipped: name=%s, reason=nil provider", name)
			continue
		}
		engine.RegisterCustomProvider(name, provider)
	}
}

// adaptSingletonFetcher 将业务侧传入的任意“func(ctx) (*T, error)”或“func(ctx) (any, error)”
// 统一适配成 changeguard 内部使用的 SingletonFetcher，避免业务侧编写重复胶水代码。
func adaptSingletonFetcher(raw any) (SingletonFetcher, bool) {
	if raw == nil {
		return nil, false
	}
	if fetcher, ok := raw.(SingletonFetcher); ok {
		return fetcher, true
	}
	value := reflect.ValueOf(raw)
	typ := value.Type()
	if typ.Kind() != reflect.Func {
		return nil, false
	}
	if typ.NumIn() != 1 || !typ.In(0).Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
		return nil, false
	}
	if typ.NumOut() != 2 || !typ.Out(1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return nil, false
	}
	return func(ctx context.Context) (any, error) {
		results := value.Call([]reflect.Value{reflect.ValueOf(ctx)})
		if errValue := results[1]; !errValue.IsNil() {
			return nil, errValue.Interface().(error)
		}
		return results[0].Interface(), nil
	}, true
}
