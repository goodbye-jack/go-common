package changeguard

import (
	"fmt"
	"strings"
)

func buildScenarioPolicy(key, kind string, scenario ScenarioConfig) (PolicyProfile, bool) {
	builder, ok := newScenarioPolicyTemplate(key, kind)
	if !ok {
		return PolicyProfile{}, false
	}
	applyScenarioNotify(builder, scenario.Notify)
	applyScenarioOverrides(builder, scenario.Overrides)
	return builder.Build(), true
}

func newScenarioPolicyTemplate(key, kind string) (*PolicyBuilder, bool) {
	builder := NewPolicy(key)
	switch strings.TrimSpace(kind) {
	case "critical_config":
		builder.
			RiskLevel(RiskLevelCritical).
			NotifyChannels("sms").
			VersioningEnabled(true).
			RollbackEnabled(true).
			DriftCheckEnabled(true).
			SensitiveFields(
				"*.private_key",
				"*.public_key",
				"*.platform_public_key",
				"*.api_v3_key",
				"*.access_key_secret",
				"*.app_secret",
				"*.token",
				"*.secret",
			).
			ChangedOnlyFields(
				"*.private_key",
				"*.public_key",
				"*.platform_public_key",
				"*.api_v3_key",
				"*.access_key_secret",
				"*.app_secret",
				"*.token",
				"*.secret",
			).
			SliceToMapBy(map[string]string{
				"channels":  "channel_code",
				"providers": "provider_code",
			})
		return builder, true
	case "price_resource":
		builder.
			RiskLevel(RiskLevelHigh).
			NotifyChannels("sms").
			VersioningEnabled(true).
			RollbackEnabled(true).
			DriftCheckEnabled(true).
			IgnoreFields("CreatedAt", "UpdatedAt", "DeletedAt").
			DisplayNames(map[string]string{
				"sale_price_fen":   "销售价",
				"list_price_fen":   "原价",
				"market_price_fen": "市场价",
				"token_amount":     "Token数量",
				"storage_gb":       "容量(GB)",
			})
		return builder, true
	case "status_toggle_resource":
		builder.
			RiskLevel(RiskLevelHigh).
			NotifyChannels("sms").
			VersioningEnabled(true).
			RollbackEnabled(true).
			DriftCheckEnabled(false).
			NotifyOnActions(ActionEnable, ActionDisable, ActionToggle)
		return builder, true
	case "secret_config":
		builder.
			RiskLevel(RiskLevelCritical).
			NotifyChannels("sms").
			VersioningEnabled(true).
			RollbackEnabled(true).
			DriftCheckEnabled(true).
			SensitiveFields(
				"*.private_key",
				"*.public_key",
				"*.platform_public_key",
				"*.api_v3_key",
				"*.access_key_secret",
				"*.app_secret",
				"*.token",
				"*.secret",
			).
			ChangedOnlyFields(
				"*.private_key",
				"*.public_key",
				"*.platform_public_key",
				"*.api_v3_key",
				"*.access_key_secret",
				"*.app_secret",
				"*.token",
				"*.secret",
			)
		return builder, true
	case "custom":
		return builder, true
	default:
		return nil, false
	}
}

func applyScenarioNotify(builder *PolicyBuilder, notify ScenarioNotifyConfig) {
	if builder == nil {
		return
	}
	if notify.Enabled != nil && !*notify.Enabled {
		builder.profile.NotifyChannels = nil
		builder.profile.NotifyTemplate = ""
		return
	}
	if len(notify.Channels) > 0 {
		builder.profile.NotifyChannels = append([]string{}, notify.Channels...)
	}
	if strings.TrimSpace(notify.Template) != "" {
		builder.profile.NotifyTemplate = strings.TrimSpace(notify.Template)
	}
}

func applyScenarioOverrides(builder *PolicyBuilder, overrides ScenarioOverrideConfig) {
	if builder == nil {
		return
	}
	if strings.TrimSpace(overrides.RiskLevel) != "" {
		builder.profile.RiskLevel = strings.TrimSpace(overrides.RiskLevel)
	}
	if strings.TrimSpace(overrides.FailMode) != "" {
		builder.profile.FailMode = strings.TrimSpace(overrides.FailMode)
	}
	if overrides.IncludeFields != nil {
		builder.profile.IncludeFields = append([]string{}, overrides.IncludeFields...)
	}
	if overrides.IgnoreFields != nil {
		builder.profile.IgnoreFields = append([]string{}, overrides.IgnoreFields...)
	}
	if overrides.SensitiveFields != nil {
		builder.profile.SensitiveFields = append([]string{}, overrides.SensitiveFields...)
	}
	if overrides.ChangedOnlyFields != nil {
		builder.profile.ChangedOnlyFields = append([]string{}, overrides.ChangedOnlyFields...)
	}
	if len(overrides.DisplayNames) > 0 {
		builder.profile.DisplayNames = mergeStringMaps(builder.profile.DisplayNames, overrides.DisplayNames)
	}
	if len(overrides.SliceToMapBy) > 0 {
		builder.profile.SliceToMapBy = mergeStringMaps(builder.profile.SliceToMapBy, overrides.SliceToMapBy)
	}
	if overrides.MaxDiffChanges > 0 {
		builder.profile.MaxDiffChanges = overrides.MaxDiffChanges
	}
	if overrides.SummaryFieldLimit > 0 {
		builder.profile.SummaryFieldLimit = overrides.SummaryFieldLimit
	}
	if overrides.SummaryValueLimit > 0 {
		builder.profile.SummaryValueLimit = overrides.SummaryValueLimit
	}
	if overrides.VersioningEnabled != nil {
		builder.profile.VersioningEnabled = *overrides.VersioningEnabled
	}
	if overrides.VersionOnActions != nil {
		builder.profile.VersionOnActions = append([]string{}, overrides.VersionOnActions...)
	}
	if overrides.RollbackEnabled != nil {
		builder.profile.RollbackEnabled = *overrides.RollbackEnabled
	}
	if overrides.RollbackRequireGuard != nil {
		builder.profile.RollbackRequireGuard = *overrides.RollbackRequireGuard
	}
	if overrides.RetentionDays > 0 {
		builder.profile.RetentionDays = overrides.RetentionDays
	}
	if strings.TrimSpace(overrides.VersionSnapshotMode) != "" {
		builder.profile.VersionSnapshotMode = strings.TrimSpace(overrides.VersionSnapshotMode)
	}
	if overrides.DriftCheckEnabled != nil {
		builder.profile.DriftCheckEnabled = *overrides.DriftCheckEnabled
	}
	if strings.TrimSpace(overrides.DriftCheckPolicy) != "" {
		builder.profile.DriftCheckPolicy = strings.TrimSpace(overrides.DriftCheckPolicy)
	}
	if strings.TrimSpace(overrides.DriftBaselineMode) != "" {
		builder.profile.DriftBaselineMode = strings.TrimSpace(overrides.DriftBaselineMode)
	}
	if overrides.DriftFields != nil {
		builder.profile.DriftFields = append([]string{}, overrides.DriftFields...)
	}
	if len(overrides.DriftThresholds) > 0 {
		builder.profile.DriftThresholds = mergeStringMaps(builder.profile.DriftThresholds, overrides.DriftThresholds)
	}
	if overrides.SecondFactorEnabled != nil {
		builder.profile.RequireSecondFactor = *overrides.SecondFactorEnabled
		if !*overrides.SecondFactorEnabled {
			builder.profile.SecondFactorMode = ""
			builder.profile.SecondFactorOnActions = nil
		}
	}
	if strings.TrimSpace(overrides.SecondFactorMode) != "" {
		builder.profile.RequireSecondFactor = true
		builder.profile.SecondFactorMode = strings.TrimSpace(overrides.SecondFactorMode)
	}
	if overrides.SecondFactorOnActions != nil {
		builder.profile.SecondFactorOnActions = append([]string{}, overrides.SecondFactorOnActions...)
	}
	if len(overrides.Tags) > 0 {
		builder.profile.Tags = mergeStringMaps(builder.profile.Tags, overrides.Tags)
	}
}

func buildScenarioResource(key, kind, name string, source ScenarioSourceConfig, spec AppSpec) (ResourceProfile, bool, string) {
	sourceType := strings.TrimSpace(source.Type)
	if sourceType == "" {
		return ResourceProfile{}, false, "source.type is empty"
	}
	if !kindSupportsSourceType(kind, sourceType) {
		return ResourceProfile{}, false, fmt.Sprintf("kind=%s not compatible with source.type=%s", kind, sourceType)
	}
	var builder *ResourceBuilder
	switch sourceType {
	case "singleton":
		if strings.TrimSpace(source.Fetcher) == "" {
			return ResourceProfile{}, false, "singleton source requires fetcher"
		}
		builder = SingletonResource(key).Fetcher(strings.TrimSpace(source.Fetcher))
	case "gorm_entity":
		model := spec.Models[strings.TrimSpace(source.Model)]
		if model == nil {
			return ResourceProfile{}, false, fmt.Sprintf("model not registered: %s", source.Model)
		}
		builder = GormResource(key, model)
	case "gorm_toggle":
		model := spec.Models[strings.TrimSpace(source.Model)]
		if model == nil {
			return ResourceProfile{}, false, fmt.Sprintf("model not registered: %s", source.Model)
		}
		builder = ToggleResource(key, model)
	case "custom":
		if strings.TrimSpace(source.CustomKey) == "" {
			return ResourceProfile{}, false, "custom source requires custom_key"
		}
		if spec.CustomProviders[strings.TrimSpace(source.CustomKey)] == nil {
			return ResourceProfile{}, false, fmt.Sprintf("custom provider not registered: %s", source.CustomKey)
		}
		builder = CustomResource(key, strings.TrimSpace(source.CustomKey))
	default:
		return ResourceProfile{}, false, fmt.Sprintf("unsupported source.type=%s", sourceType)
	}
	profile := builder.
		Name(name).
		ResourceType(key).
		Policy(key).
		Keys(source.RequestKeys...).
		Lookup(source.LookupKeys...).
		StatusField(source.StatusField).
		Build()
	profile.BatchKeys = append([]string{}, source.BatchKeys...)
	profile.Metadata = map[string]string{
		"scenario_key":  key,
		"scenario_kind": kind,
		"source_type":   sourceType,
	}
	return profile, true, ""
}

func kindSupportsSourceType(kind, sourceType string) bool {
	switch strings.TrimSpace(kind) {
	case "critical_config", "secret_config":
		return sourceType == "singleton" || sourceType == "custom"
	case "price_resource":
		return sourceType == "gorm_entity"
	case "status_toggle_resource":
		return sourceType == "gorm_toggle" || sourceType == "gorm_entity"
	case "custom":
		return sourceType == "singleton" || sourceType == "gorm_entity" || sourceType == "gorm_toggle" || sourceType == "custom"
	default:
		return false
	}
}

func buildScenarioBindings(key, kind string, scenario ScenarioConfig, providers ProvidersConfig) ([]RouteBinding, bool, string) {
	if len(scenario.Routes) == 0 {
		return nil, false, "routes is empty"
	}
	recipientProfile := strings.TrimSpace(scenario.Notify.RecipientProfile)
	if recipientProfile != "" {
		if _, ok := providers.RecipientProfiles[recipientProfile]; !ok {
			return nil, false, fmt.Sprintf("recipient_profile not found: %s", recipientProfile)
		}
	}
	bindings := make([]RouteBinding, 0, len(scenario.Routes))
	for _, route := range scenario.Routes {
		path := strings.TrimSpace(route.Path)
		action := strings.TrimSpace(route.Action)
		if path == "" {
			return nil, false, "route.path is empty"
		}
		if action == "" {
			return nil, false, "route.action is empty"
		}
		if !isAllowedScenarioAction(action) {
			return nil, false, fmt.Sprintf("unsupported route.action=%s", action)
		}
		builder := BindPath(path, key).Action(action)
		methods := normalizeMethods(route.Methods)
		if len(methods) > 0 {
			builder.Methods(methods...)
		}
		binding := builder.Build()
		binding.Key = fmt.Sprintf("%s#%s#%s", key, action, path)
		if route.Enabled != nil {
			binding.Enabled = *route.Enabled
		}
		binding.Metadata = map[string]string{
			"scenario_key":  key,
			"scenario_kind": kind,
		}
		if recipientProfile != "" {
			binding.Metadata["recipient_profile"] = recipientProfile
		} else if _, ok := providers.RecipientProfiles["default"]; ok {
			binding.Metadata["recipient_profile"] = "default"
		}
		bindings = append(bindings, binding)
	}
	return bindings, true, ""
}

func normalizeMethods(methods []string) []string {
	if len(methods) == 0 {
		return nil
	}
	result := make([]string, 0, len(methods))
	for _, method := range methods {
		trimmed := strings.ToUpper(strings.TrimSpace(method))
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func isAllowedScenarioAction(action string) bool {
	switch strings.TrimSpace(action) {
	case ActionSave, ActionPublish, ActionEnable, ActionDisable, ActionToggle, ActionDelete, ActionRollback, ActionRestore, ActionUpdate:
		return true
	default:
		return false
	}
}

func mergeStringMaps(base map[string]string, extra map[string]string) map[string]string {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	result := map[string]string{}
	for key, value := range base {
		result[key] = value
	}
	for key, value := range extra {
		result[key] = value
	}
	return result
}
