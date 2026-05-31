package changeguard

type ResourceBuilder struct {
	profile ResourceProfile
}

func SingletonResource(key string) *ResourceBuilder {
	return &ResourceBuilder{profile: ResourceProfile{
		Key:          key,
		Enabled:      true,
		ResourceType: key,
		ProviderType: ProviderSingletonConfig,
	}}
}

func GormResource(key string, model any) *ResourceBuilder {
	return &ResourceBuilder{profile: ResourceProfile{
		Key:          key,
		Enabled:      true,
		ResourceType: key,
		ProviderType: ProviderGormEntitySave,
		ModelValue:   model,
	}}
}

func ToggleResource(key string, model any) *ResourceBuilder {
	return &ResourceBuilder{profile: ResourceProfile{
		Key:          key,
		Enabled:      true,
		ResourceType: key,
		ProviderType: ProviderGormEntityToggle,
		ModelValue:   model,
	}}
}

func CustomResource(key, customKey string) *ResourceBuilder {
	return &ResourceBuilder{profile: ResourceProfile{
		Key:          key,
		Enabled:      true,
		ResourceType: key,
		ProviderType: ProviderCustomFetcher,
		CustomKey:    customKey,
	}}
}

func (b *ResourceBuilder) Name(name string) *ResourceBuilder {
	b.profile.Name = name
	return b
}

func (b *ResourceBuilder) ResourceType(resourceType string) *ResourceBuilder {
	b.profile.ResourceType = resourceType
	return b
}

func (b *ResourceBuilder) Fetcher(fetcherName string) *ResourceBuilder {
	b.profile.FetcherName = fetcherName
	return b
}

func (b *ResourceBuilder) Policy(policyName string) *ResourceBuilder {
	b.profile.PolicyName = policyName
	return b
}

func (b *ResourceBuilder) Keys(keys ...string) *ResourceBuilder {
	b.profile.RequestKeys = append([]string{}, keys...)
	return b
}

func (b *ResourceBuilder) Lookup(keys ...string) *ResourceBuilder {
	b.profile.LookupKeys = append([]string{}, keys...)
	return b
}

func (b *ResourceBuilder) StatusField(field string) *ResourceBuilder {
	b.profile.StatusField = field
	return b
}

func (b *ResourceBuilder) Build() ResourceProfile {
	return b.profile
}

type BindingBuilder struct {
	binding RouteBinding
}

func BindPath(path string, resourceKey string) *BindingBuilder {
	return &BindingBuilder{binding: RouteBinding{
		Path:        path,
		Methods:     []string{"POST"},
		ResourceKey: resourceKey,
		Enabled:     true,
		Action:      ActionUpdate,
	}}
}

func (b *BindingBuilder) Methods(methods ...string) *BindingBuilder {
	b.binding.Methods = append([]string{}, methods...)
	return b
}

func (b *BindingBuilder) Action(action string) *BindingBuilder {
	b.binding.Action = action
	return b
}

func (b *BindingBuilder) GuardPolicy(name string) *BindingBuilder {
	b.binding.GuardPolicy = name
	return b
}

func (b *BindingBuilder) Risk(level string) *BindingBuilder {
	b.binding.OverrideRisk = level
	return b
}

func (b *BindingBuilder) Build() RouteBinding {
	return b.binding
}

type PolicyBuilder struct {
	profile PolicyProfile
}

func NewPolicy(name string) *PolicyBuilder {
	return &PolicyBuilder{profile: PolicyProfile{
		Name:                name,
		Enabled:             true,
		FailMode:            FailModeOpen,
		NotifyOnlyOnSuccess: true,
	}}
}

func (b *PolicyBuilder) RiskLevel(level string) *PolicyBuilder {
	b.profile.RiskLevel = level
	return b
}

func (b *PolicyBuilder) FailMode(mode string) *PolicyBuilder {
	b.profile.FailMode = mode
	return b
}

func (b *PolicyBuilder) IncludeFields(fields ...string) *PolicyBuilder {
	b.profile.IncludeFields = append([]string{}, fields...)
	return b
}

func (b *PolicyBuilder) IgnoreFields(fields ...string) *PolicyBuilder {
	b.profile.IgnoreFields = append([]string{}, fields...)
	return b
}

func (b *PolicyBuilder) SensitiveFields(fields ...string) *PolicyBuilder {
	b.profile.SensitiveFields = append([]string{}, fields...)
	return b
}

func (b *PolicyBuilder) ChangedOnlyFields(fields ...string) *PolicyBuilder {
	b.profile.ChangedOnlyFields = append([]string{}, fields...)
	return b
}

func (b *PolicyBuilder) DisplayNames(values map[string]string) *PolicyBuilder {
	b.profile.DisplayNames = cloneStringMap(values)
	return b
}

func (b *PolicyBuilder) SliceToMapBy(values map[string]string) *PolicyBuilder {
	b.profile.SliceToMapBy = cloneStringMap(values)
	return b
}

func (b *PolicyBuilder) NotifyChannels(channels ...string) *PolicyBuilder {
	b.profile.NotifyChannels = append([]string{}, channels...)
	return b
}

func (b *PolicyBuilder) NotifyTemplate(name string) *PolicyBuilder {
	b.profile.NotifyTemplate = name
	return b
}

func (b *PolicyBuilder) NotifyOnlyOnSuccess(enabled bool) *PolicyBuilder {
	b.profile.NotifyOnlyOnSuccess = enabled
	return b
}

func (b *PolicyBuilder) NotifyOnActions(actions ...string) *PolicyBuilder {
	b.profile.NotifyOnActions = append([]string{}, actions...)
	return b
}

func (b *PolicyBuilder) NotifyOnRiskLevels(levels ...string) *PolicyBuilder {
	b.profile.NotifyOnRiskLevels = append([]string{}, levels...)
	return b
}

func (b *PolicyBuilder) VersioningEnabled(enabled bool) *PolicyBuilder {
	b.profile.VersioningEnabled = enabled
	return b
}

func (b *PolicyBuilder) RollbackEnabled(enabled bool) *PolicyBuilder {
	b.profile.RollbackEnabled = enabled
	return b
}

func (b *PolicyBuilder) DriftCheckEnabled(enabled bool) *PolicyBuilder {
	b.profile.DriftCheckEnabled = enabled
	return b
}

func (b *PolicyBuilder) DriftFields(fields ...string) *PolicyBuilder {
	b.profile.DriftFields = append([]string{}, fields...)
	return b
}

func (b *PolicyBuilder) DriftThresholds(values map[string]string) *PolicyBuilder {
	b.profile.DriftThresholds = cloneStringMap(values)
	return b
}

func (b *PolicyBuilder) DriftBaselineMode(mode string) *PolicyBuilder {
	b.profile.DriftBaselineMode = mode
	return b
}

func (b *PolicyBuilder) RequirePasswordReverify(enabled bool) *PolicyBuilder {
	b.profile.RequirePasswordReverify = enabled
	return b
}

func (b *PolicyBuilder) RequireSecondFactor(mode string) *PolicyBuilder {
	b.profile.RequireSecondFactor = mode != ""
	b.profile.SecondFactorMode = mode
	return b
}

func (b *PolicyBuilder) SecondFactorOnActions(actions ...string) *PolicyBuilder {
	b.profile.SecondFactorOnActions = append([]string{}, actions...)
	return b
}

func (b *PolicyBuilder) Build() PolicyProfile {
	return b.profile
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
