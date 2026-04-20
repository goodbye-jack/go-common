package identity

import (
	"fmt"
	"strings"

	"github.com/goodbye-jack/go-common/workflow/types"
	"github.com/spf13/viper"
)

const (
	configRoleAliases  = "workflow.identity.role_aliases"
	configGroupAliases = "workflow.identity.group_aliases"
	configRolePrefix   = "workflow.flowable.role_prefix"
	configGroupPrefix  = "workflow.flowable.group_prefix"
)

// Normalizer 用于把不同来源的身份编码归一为统一的工作流角色/组编码。
// 目的：让 HTTP / LDAP 等不同目录提供方在进入 Flowable 前，尽量落到同一套契约。
type Normalizer struct {
	roleAliases  map[string]string
	groupAliases map[string]string
	rolePrefix   string
	groupPrefix  string
}

// NewNormalizerFromConfig 根据 workflow.identity.* 和 workflow.flowable.* 配置创建归一器。
func NewNormalizerFromConfig() *Normalizer {
	return &Normalizer{
		roleAliases:  loadAliasMap(configRoleAliases),
		groupAliases: loadAliasMap(configGroupAliases),
		rolePrefix:   strings.TrimSpace(viper.GetString(configRolePrefix)),
		groupPrefix:  strings.TrimSpace(viper.GetString(configGroupPrefix)),
	}
}

// RoleAliasCount 返回当前配置的角色别名数量。
func (n *Normalizer) RoleAliasCount() int {
	if n == nil {
		return 0
	}
	return len(n.roleAliases)
}

// GroupAliasCount 返回当前配置的组别名数量。
func (n *Normalizer) GroupAliasCount() int {
	if n == nil {
		return 0
	}
	return len(n.groupAliases)
}

// NormalizeRole 把原始角色码归一为规范角色码；未命中映射时保持原值。
func (n *Normalizer) NormalizeRole(role string) string {
	if n == nil {
		return strings.TrimSpace(role)
	}
	return normalizeValue(role, n.roleAliases)
}

// NormalizeGroup 把原始组编码归一为规范组编码；未命中映射时保持原值。
func (n *Normalizer) NormalizeGroup(group string) string {
	if n == nil {
		return strings.TrimSpace(group)
	}
	return normalizeValue(group, n.groupAliases)
}

// NormalizeRoles 对角色列表做去重、去空、归一。
func (n *Normalizer) NormalizeRoles(roles []string) []string {
	return normalizeValues(roles, n.NormalizeRole)
}

// NormalizeGroups 对组列表做去重、去空、归一。
func (n *Normalizer) NormalizeGroups(groups []string) []string {
	return normalizeValues(groups, n.NormalizeGroup)
}

// CandidateGroupIDs 根据当前配置把 groups / roles 转成 Flowable 最终识别的 groupId。
func (n *Normalizer) CandidateGroupIDs(groups, roles []string) []string {
	if n == nil {
		n = NewNormalizerFromConfig()
	}
	result := make([]string, 0, len(groups)+len(roles))
	for _, group := range n.NormalizeGroups(groups) {
		result = appendIfMissing(result, n.groupPrefix+group)
	}
	for _, role := range n.NormalizeRoles(roles) {
		result = appendIfMissing(result, n.rolePrefix+role)
	}
	return result
}

// CandidateGroupIDsForProfile 把目录用户资料转换为 Flowable 最终候选组。
// 当前默认使用岗位编码作为 role 来源，便于 LDAP / HTTP 统一对齐到业务角色契约。
func (n *Normalizer) CandidateGroupIDsForProfile(profile *types.DirectoryUserProfile) []string {
	if profile == nil {
		return nil
	}
	roleCodes := make([]string, 0, 1)
	if profile.Position != nil {
		roleCodes = append(roleCodes, strings.TrimSpace(profile.Position.PositionID))
	}
	return n.CandidateGroupIDs(nil, roleCodes)
}

func loadAliasMap(key string) map[string]string {
	raw := viper.GetStringMapString(key)
	if len(raw) == 0 {
		converted := viper.GetStringMap(key)
		if len(converted) == 0 {
			return nil
		}
		raw = make(map[string]string, len(converted))
		for alias, canonical := range converted {
			raw[alias] = strings.TrimSpace(toString(canonical))
		}
	}
	if len(raw) == 0 {
		return nil
	}
	result := make(map[string]string, len(raw))
	for alias, canonical := range raw {
		aliasKey := normalizeLookupKey(alias)
		canonicalValue := strings.TrimSpace(canonical)
		if aliasKey == "" || canonicalValue == "" {
			continue
		}
		result[aliasKey] = canonicalValue
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeValue(value string, aliases map[string]string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(aliases) == 0 {
		return trimmed
	}
	if canonical, ok := aliases[normalizeLookupKey(trimmed)]; ok && strings.TrimSpace(canonical) != "" {
		return strings.TrimSpace(canonical)
	}
	return trimmed
}

func normalizeValues(values []string, normalize func(string) string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalize(value)
		if normalized == "" {
			continue
		}
		result = appendIfMissing(result, normalized)
	}
	return result
}

func appendIfMissing(target []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return target
	}
	for _, current := range target {
		if current == value {
			return target
		}
	}
	return append(target, value)
}

func normalizeLookupKey(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func toString(value interface{}) string {
	switch current := value.(type) {
	case nil:
		return ""
	case string:
		return current
	default:
		return strings.TrimSpace(fmt.Sprint(current))
	}
}
