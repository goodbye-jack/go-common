package contract

import (
	"strings"

	"github.com/goodbye-jack/go-common/config"
	"github.com/spf13/viper"
)

const (
	ModeCompatible = "compatible"
	ModeStandard   = "standard"
	ModeStrict     = "strict"

	ConfigMode                            = "workflow.contract.mode"
	ConfigEnforceStandardAssignmentKeys   = "workflow.contract.enforce_standard_assignment_keys"
	ConfigWarnOnNonstandardAssignmentKeys = "workflow.contract.warn_on_nonstandard_assignment_keys"
	ConfigFailOnNonstandardAssignmentKeys = "workflow.contract.fail_on_nonstandard_assignment_keys"
	ConfigEnableBPMNLint                  = "workflow.contract.enable_bpmn_lint"
	ConfigFailOnPrivateBPMNVars           = "workflow.contract.fail_on_private_bpmn_vars"
	ConfigAllowedBPMNAssignmentVars       = "workflow.contract.allowed_bpmn_assignment_vars"
)

const (
	StandardAssigneeKey        = "nextAssignee"
	StandardCandidateUsersKey  = "nextCandidateUsers"
	StandardCandidateGroupsKey = "nextCandidateGroups"
)

type Policy struct {
	Mode                            string
	EnforceStandardAssignmentKeys   bool
	WarnOnNonstandardAssignmentKeys bool
	FailOnNonstandardAssignmentKeys bool
	EnableBPMNLint                  bool
	FailOnPrivateBPMNVars           bool
	AllowedBPMNAssignmentVars       []string
}

func DefaultAllowedBPMNAssignmentVars() []string {
	return []string{
		"starterId",
		"managerId",
		StandardAssigneeKey,
		StandardCandidateUsersKey,
		StandardCandidateGroupsKey,
	}
}

func DefaultPolicy() *Policy {
	return &Policy{
		Mode:                            ModeCompatible,
		EnforceStandardAssignmentKeys:   true,
		WarnOnNonstandardAssignmentKeys: true,
		AllowedBPMNAssignmentVars:       DefaultAllowedBPMNAssignmentVars(),
	}
}

func LoadPolicyFromConfig() *Policy {
	policy := DefaultPolicy()
	if policy == nil {
		return nil
	}

	policy.Mode = normalizeMode(config.GetConfigString(ConfigMode))
	policy.EnforceStandardAssignmentKeys = config.GetConfigBool(ConfigEnforceStandardAssignmentKeys)
	policy.WarnOnNonstandardAssignmentKeys = config.GetConfigBool(ConfigWarnOnNonstandardAssignmentKeys)
	policy.FailOnNonstandardAssignmentKeys = config.GetConfigBool(ConfigFailOnNonstandardAssignmentKeys)
	policy.EnableBPMNLint = config.GetConfigBool(ConfigEnableBPMNLint)
	policy.FailOnPrivateBPMNVars = config.GetConfigBool(ConfigFailOnPrivateBPMNVars)

	if !viper.IsSet(ConfigEnforceStandardAssignmentKeys) {
		policy.EnforceStandardAssignmentKeys = true
	}
	if !viper.IsSet(ConfigWarnOnNonstandardAssignmentKeys) {
		policy.WarnOnNonstandardAssignmentKeys = true
	}
	if !viper.IsSet(ConfigFailOnNonstandardAssignmentKeys) {
		policy.FailOnNonstandardAssignmentKeys = policy.Mode == ModeStrict
	}
	if !viper.IsSet(ConfigEnableBPMNLint) {
		policy.EnableBPMNLint = policy.Mode == ModeStrict
	}
	if !viper.IsSet(ConfigFailOnPrivateBPMNVars) {
		policy.FailOnPrivateBPMNVars = policy.Mode == ModeStrict
	}

	allowed := make([]string, 0)
	for _, item := range config.GetConfigStringSlice(ConfigAllowedBPMNAssignmentVars) {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		allowed = append(allowed, trimmed)
	}
	if len(allowed) == 0 {
		allowed = DefaultAllowedBPMNAssignmentVars()
	}
	policy.AllowedBPMNAssignmentVars = dedupeStrings(allowed)

	return policy
}

func normalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ModeCompatible:
		return ModeCompatible
	case ModeStandard:
		return ModeStandard
	case ModeStrict:
		return ModeStrict
	default:
		return ModeCompatible
	}
}

func (p *Policy) EffectiveMode() string {
	if p == nil {
		return ModeCompatible
	}
	return normalizeMode(p.Mode)
}

func (p *Policy) ShouldWarnOnNonstandardAssignmentKeys() bool {
	if p == nil {
		return true
	}
	return p.WarnOnNonstandardAssignmentKeys || p.EffectiveMode() == ModeStrict
}

func (p *Policy) ShouldFailOnNonstandardAssignmentKeys() bool {
	if p == nil {
		return false
	}
	return p.FailOnNonstandardAssignmentKeys || p.EffectiveMode() == ModeStrict
}

func (p *Policy) EffectiveAllowedBPMNAssignmentVars() []string {
	if p == nil || len(p.AllowedBPMNAssignmentVars) == 0 {
		return DefaultAllowedBPMNAssignmentVars()
	}
	return append([]string(nil), p.AllowedBPMNAssignmentVars...)
}

func AreStandardAssignmentKeys(assigneeKey, candidateUsersKey, candidateGroupsKey string) bool {
	return strings.TrimSpace(assigneeKey) == StandardAssigneeKey &&
		strings.TrimSpace(candidateUsersKey) == StandardCandidateUsersKey &&
		strings.TrimSpace(candidateGroupsKey) == StandardCandidateGroupsKey
}

func dedupeStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}
