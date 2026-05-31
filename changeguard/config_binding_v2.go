package changeguard

import (
	"context"
	"fmt"
	"strings"

	goodhttp "github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/log"
)

type ProvidersConfig struct {
	SMS struct {
		Enabled      bool   `mapstructure:"enabled"`
		ConfigPrefix string `mapstructure:"config_prefix"`
	} `mapstructure:"sms"`
	SecondFactorSMS SecondFactorSMSConfig `mapstructure:"second_factor_sms"`
	RecipientProfiles map[string]RecipientProfileConfig `mapstructure:"recipient_profiles"`
}

type SecondFactorSMSConfig struct {
	Enabled            bool   `mapstructure:"enabled"`
	ConfigPrefix       string `mapstructure:"config_prefix"`
	Template           string `mapstructure:"template"`
	ReplyTemplate      string `mapstructure:"reply_template"`
	CodeTTL            string `mapstructure:"code_ttl"`
	ResendInterval     string `mapstructure:"resend_interval"`
	VerifiedTTL        string `mapstructure:"verified_ttl"`
	MaxVerifyAttempts  int    `mapstructure:"max_verify_attempts"`
	ChallengeHeader    string `mapstructure:"challenge_header"`
	CodeHeader         string `mapstructure:"code_header"`
	ReplyCallbackPath  string `mapstructure:"reply_callback_path"`
	ReplyTokenSize     int    `mapstructure:"reply_token_size"`
	ReplyApprovePrefix string `mapstructure:"reply_approve_prefix"`
	ReplyRejectPrefix  string `mapstructure:"reply_reject_prefix"`
	UserIDField        string `mapstructure:"user_id_field"`
	PhoneField         string `mapstructure:"phone_field"`
	TenantField        string `mapstructure:"tenant_field"`
	UserTypeField      string `mapstructure:"user_type_field"`
	StatusField        string `mapstructure:"status_field"`
	RequiredUserType   string `mapstructure:"required_user_type"`
	RequiredStatus     string `mapstructure:"required_status"`
}

type RecipientProfileConfig struct {
	Mode        string   `mapstructure:"mode"`
	UserType    string   `mapstructure:"user_type"`
	Status      string   `mapstructure:"status"`
	PhoneField  string   `mapstructure:"phone_field"`
	TenantField string   `mapstructure:"tenant_field"`
	Values      []string `mapstructure:"values"`
	Resolver    string   `mapstructure:"resolver"`
}

type ScenarioConfig struct {
	Key       string                 `mapstructure:"key"`
	Enabled   *bool                  `mapstructure:"enabled"`
	Kind      string                 `mapstructure:"kind"`
	Name      string                 `mapstructure:"name"`
	Source    ScenarioSourceConfig   `mapstructure:"source"`
	Routes    []ScenarioRouteConfig  `mapstructure:"routes"`
	Notify    ScenarioNotifyConfig   `mapstructure:"notify"`
	Overrides ScenarioOverrideConfig `mapstructure:"overrides"`
}

type ScenarioSourceConfig struct {
	Type        string   `mapstructure:"type"`
	Fetcher     string   `mapstructure:"fetcher"`
	Model       string   `mapstructure:"model"`
	CustomKey   string   `mapstructure:"custom_key"`
	RequestKeys []string `mapstructure:"request_keys"`
	LookupKeys  []string `mapstructure:"lookup_keys"`
	BatchKeys   []string `mapstructure:"batch_keys"`
	StatusField string   `mapstructure:"status_field"`
}

type ScenarioRouteConfig struct {
	Path    string   `mapstructure:"path"`
	Action  string   `mapstructure:"action"`
	Methods []string `mapstructure:"methods"`
	Enabled *bool    `mapstructure:"enabled"`
}

type ScenarioNotifyConfig struct {
	Enabled          *bool    `mapstructure:"enabled"`
	Channels         []string `mapstructure:"channels"`
	RecipientProfile string   `mapstructure:"recipient_profile"`
	Template         string   `mapstructure:"template"`
}

type ScenarioOverrideConfig struct {
	RiskLevel            string            `mapstructure:"risk_level"`
	FailMode             string            `mapstructure:"fail_mode"`
	IncludeFields        []string          `mapstructure:"include_fields"`
	IgnoreFields         []string          `mapstructure:"ignore_fields"`
	SensitiveFields      []string          `mapstructure:"sensitive_fields"`
	ChangedOnlyFields    []string          `mapstructure:"changed_only_fields"`
	DisplayNames         map[string]string `mapstructure:"display_names"`
	SliceToMapBy         map[string]string `mapstructure:"slice_to_map_by"`
	MaxDiffChanges       int               `mapstructure:"max_diff_changes"`
	SummaryFieldLimit    int               `mapstructure:"summary_field_limit"`
	SummaryValueLimit    int               `mapstructure:"summary_value_limit"`
	VersioningEnabled    *bool             `mapstructure:"versioning_enabled"`
	VersionOnActions     []string          `mapstructure:"version_on_actions"`
	RollbackEnabled      *bool             `mapstructure:"rollback_enabled"`
	RollbackRequireGuard *bool             `mapstructure:"rollback_require_guard"`
	RetentionDays        int               `mapstructure:"retention_days"`
	VersionSnapshotMode  string            `mapstructure:"version_snapshot_mode"`
	DriftCheckEnabled    *bool             `mapstructure:"drift_check_enabled"`
	DriftCheckPolicy     string            `mapstructure:"drift_check_policy"`
	DriftBaselineMode    string            `mapstructure:"drift_baseline_mode"`
	DriftFields          []string          `mapstructure:"drift_fields"`
	DriftThresholds      map[string]string `mapstructure:"drift_thresholds"`
	SecondFactorEnabled  *bool             `mapstructure:"second_factor_enabled"`
	SecondFactorMode     string            `mapstructure:"second_factor_mode"`
	SecondFactorOnActions []string         `mapstructure:"second_factor_on_actions"`
	Tags                 map[string]string `mapstructure:"tags"`
}

type scenarioBindingResult struct {
	AppliedScenarios   int
	SkippedScenarios   int
	GeneratedResources int
	GeneratedBindings  int
	UnmatchedRoutes    int
}

type recipientProfileResolver struct {
	resolvers map[string]RecipientResolver
}

func bindFromV2Config(server *goodhttp.HTTPServer, spec AppSpec, cfg ConfigSpec, serviceName string) error {
	engine := NewEngine(buildEngineOptionsFromConfig(serviceName, cfg.Runtime))
	engine.SetSink(NewGormSink())
	engine.SetVersionStore(NewGormVersionStore())
	engine.SetDriftReportStore(NewGormDriftReportStore())
	registerFetchersFromSpec(engine, spec)
	registerCustomProvidersFromSpec(engine, spec)
	if cfg.Providers.SMS.Enabled {
		engine.RegisterNotifier(NewSMSNotifier(NewConfigPrefixSMSResolver(cfg.Providers.SMS.ConfigPrefix)))
	}
	if secondFactor, ok := newSecondFactorService(spec, serviceName, cfg.Providers.SecondFactorSMS); ok {
		engine.SetSecondFactor(secondFactor)
		registerSecondFactorReplyRoute(server, secondFactor)
	}
	profileResolver, ok := newRecipientProfileResolver(cfg.Providers, spec)
	if ok {
		engine.SetRecipientResolver(profileResolver)
	}
	result := bindV2Scenarios(engine, server, spec, cfg)
	log.Infof("changeguard v2 bind summary, service=%s, applied=%d, skipped=%d, resources=%d, bindings=%d, unmatched_routes=%d",
		serviceName, result.AppliedScenarios, result.SkippedScenarios, result.GeneratedResources, result.GeneratedBindings, result.UnmatchedRoutes)
	return engine.Bind(server)
}

func bindV2Scenarios(engine *Engine, server *goodhttp.HTTPServer, spec AppSpec, cfg ConfigSpec) scenarioBindingResult {
	result := scenarioBindingResult{}
	if engine == nil {
		return result
	}
	log.Infof("changeguard v2 loaded, scenarios=%d", len(cfg.Scenarios))
	seenScenarioKeys := map[string]struct{}{}
	routeRefs := collectRouteRefs(server)
	policies := make([]PolicyProfile, 0, len(cfg.Scenarios))
	resources := make([]ResourceProfile, 0, len(cfg.Scenarios))
	bindings := make([]RouteBinding, 0, len(cfg.Scenarios))
	for _, scenario := range cfg.Scenarios {
		outcome := buildScenarioOutcome(scenario, spec, cfg.Providers, seenScenarioKeys)
		if outcome.skipReason != "" {
			result.SkippedScenarios++
			log.Warnf("changeguard scenario skipped, key=%s, reason=%s", scenario.Key, outcome.skipReason)
			continue
		}
		policies = append(policies, outcome.policy)
		resources = append(resources, outcome.resource)
		bindings = append(bindings, outcome.bindings...)
		result.AppliedScenarios++
		result.GeneratedResources++
		result.GeneratedBindings += len(outcome.bindings)
		log.Infof("changeguard scenario applied, key=%s, kind=%s", outcome.scenarioKey, outcome.scenarioKind)
		for _, binding := range outcome.bindings {
			if !routeMatchedByBinding(routeRefs, binding) {
				result.UnmatchedRoutes++
				log.Warnf("changeguard route unmatched, key=%s, path=%s, methods=%s", outcome.scenarioKey, binding.Path, strings.Join(binding.Methods, ","))
			}
		}
	}
	engine.RegisterPolicies(policies...)
	engine.RegisterResources(resources...)
	engine.RegisterBindings(bindings...)
	return result
}

type routeRef struct {
	path    string
	methods []string
}

func collectRouteRefs(server *goodhttp.HTTPServer) []routeRef {
	if server == nil {
		return nil
	}
	routes := server.GetRoutes()
	result := make([]routeRef, 0, len(routes))
	for _, route := range routes {
		if route == nil {
			continue
		}
		result = append(result, routeRef{
			path:    route.Url,
			methods: append([]string{}, route.Methods...),
		})
	}
	return result
}

func routeMatchedByBinding(routes []routeRef, binding RouteBinding) bool {
	for _, route := range routes {
		if route.path != binding.Path {
			continue
		}
		if methodMatched(route.methods, binding.Methods) {
			return true
		}
	}
	return false
}

func newRecipientProfileResolver(cfg ProvidersConfig, spec AppSpec) (RecipientResolver, bool) {
	if len(cfg.RecipientProfiles) == 0 {
		return nil, false
	}
	resolvers := map[string]RecipientResolver{}
	for name, profile := range cfg.RecipientProfiles {
		resolver, ok := buildRecipientResolver(name, profile, spec)
		if !ok {
			log.Warnf("changeguard recipient profile skipped, name=%s, reason=unsupported or invalid config", name)
			continue
		}
		resolvers[name] = resolver
	}
	if len(resolvers) == 0 {
		return nil, false
	}
	return &recipientProfileResolver{resolvers: resolvers}, true
}

func buildRecipientResolver(name string, profile RecipientProfileConfig, spec AppSpec) (RecipientResolver, bool) {
	mode := strings.TrimSpace(profile.Mode)
	switch mode {
	case "admin_users":
		if spec.UserModel == nil {
			return nil, false
		}
		return AdminUserRecipients(AdminUserRecipientOptions{
			UserModel:   spec.UserModel,
			UserType:    profile.UserType,
			Status:      profile.Status,
			PhoneField:  profile.PhoneField,
			TenantField: profile.TenantField,
		}), true
	case "fixed_phones", "fixed_emails":
		return &fixedRecipientsResolver{values: append([]string{}, profile.Values...)}, true
	case "custom":
		resolver := spec.RecipientResolvers[strings.TrimSpace(profile.Resolver)]
		if resolver == nil {
			return nil, false
		}
		return resolver, true
	default:
		_ = name
		return nil, false
	}
}

func (r *recipientProfileResolver) Resolve(ctx context.Context, event ChangeEvent, channel string) ([]string, error) {
	if r == nil || len(r.resolvers) == 0 {
		return nil, nil
	}
	profile := strings.TrimSpace(event.Metadata["recipient_profile"])
	if profile == "" {
		profile = "default"
	}
	resolver := r.resolvers[profile]
	if resolver == nil {
		return nil, nil
	}
	return resolver.Resolve(ctx, event, channel)
}

type fixedRecipientsResolver struct {
	values []string
}

func (r *fixedRecipientsResolver) Resolve(context.Context, ChangeEvent, string) ([]string, error) {
	return compactStrings(append([]string{}, r.values...)), nil
}

type scenarioBuildOutcome struct {
	scenarioKey  string
	scenarioKind string
	policy       PolicyProfile
	resource     ResourceProfile
	bindings     []RouteBinding
	skipReason   string
}

func buildScenarioOutcome(scenario ScenarioConfig, spec AppSpec, providers ProvidersConfig, seen map[string]struct{}) scenarioBuildOutcome {
	key := strings.TrimSpace(scenario.Key)
	if key == "" {
		return scenarioBuildOutcome{skipReason: "scenario.key is empty"}
	}
	if _, exists := seen[key]; exists {
		return scenarioBuildOutcome{skipReason: "scenario.key duplicated"}
	}
	seen[key] = struct{}{}
	if scenario.Enabled != nil && !*scenario.Enabled {
		return scenarioBuildOutcome{skipReason: "scenario disabled"}
	}
	kind := strings.TrimSpace(scenario.Kind)
	if kind == "" {
		return scenarioBuildOutcome{skipReason: "scenario.kind is empty"}
	}
	name := strings.TrimSpace(scenario.Name)
	if name == "" {
		return scenarioBuildOutcome{skipReason: "scenario.name is empty"}
	}
	policy, ok := buildScenarioPolicy(key, kind, scenario)
	if !ok {
		return scenarioBuildOutcome{skipReason: fmt.Sprintf("unsupported scenario.kind=%s", kind)}
	}
	resource, ok, reason := buildScenarioResource(key, kind, name, scenario.Source, spec)
	if !ok {
		return scenarioBuildOutcome{skipReason: reason}
	}
	bindings, ok, reason := buildScenarioBindings(key, kind, scenario, providers)
	if !ok {
		return scenarioBuildOutcome{skipReason: reason}
	}
	return scenarioBuildOutcome{
		scenarioKey:  key,
		scenarioKind: kind,
		policy:       policy,
		resource:     resource,
		bindings:     bindings,
	}
}
