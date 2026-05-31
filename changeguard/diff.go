package changeguard

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

func Compare(before, after map[string]any, policy PolicyProfile) (*DiffResult, error) {
	result := &DiffResult{
		BeforeMasked: cloneAnyMap(before),
		AfterMasked:  cloneAnyMap(after),
	}
	flatBefore := flattenMap("", before)
	flatAfter := flattenMap("", after)
	keysMap := map[string]struct{}{}
	for key := range flatBefore {
		keysMap[key] = struct{}{}
	}
	for key := range flatAfter {
		keysMap[key] = struct{}{}
	}
	keys := make([]string, 0, len(keysMap))
	for key := range keysMap {
		if shouldIgnoreField(key, policy.IgnoreFields) {
			continue
		}
		if len(policy.IncludeFields) > 0 && !matchesAny(key, policy.IncludeFields) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		beforeVal, beforeOK := flatBefore[key]
		afterVal, afterOK := flatAfter[key]
		if beforeOK && afterOK && reflect.DeepEqual(beforeVal, afterVal) {
			continue
		}
		change := FieldChange{
			Path:        key,
			DisplayName: displayNameFor(key, policy.DisplayNames),
			ChangeType:  inferChangeType(beforeOK, afterOK),
			Sensitive:   matchesAny(key, policy.SensitiveFields) || matchesAny(key, policy.ChangedOnlyFields),
		}
		if !matchesAny(key, policy.ChangedOnlyFields) {
			change.Before = beforeVal
			change.After = afterVal
		}
		if change.Sensitive {
			change.Before = maskedValue(beforeVal, beforeOK)
			change.After = maskedValue(afterVal, afterOK)
		}
		result.Changes = append(result.Changes, change)
		if len(result.Summary) < policy.SummaryFieldLimit || policy.SummaryFieldLimit <= 0 {
			result.Summary = append(result.Summary, buildSummary(change, policy.SummaryValueLimit))
		}
	}
	if policy.MaxDiffChanges > 0 && len(result.Changes) > policy.MaxDiffChanges {
		result.Changes = result.Changes[:policy.MaxDiffChanges]
	}
	result.Changed = len(result.Changes) > 0
	result.BeforeMasked = maskNested(result.BeforeMasked, policy)
	result.AfterMasked = maskNested(result.AfterMasked, policy)
	return result, nil
}

func inferChangeType(beforeOK, afterOK bool) string {
	switch {
	case !beforeOK && afterOK:
		return "added"
	case beforeOK && !afterOK:
		return "removed"
	default:
		return "updated"
	}
}

func displayNameFor(path string, values map[string]string) string {
	if len(values) == 0 {
		return path
	}
	if value, ok := values[path]; ok && strings.TrimSpace(value) != "" {
		return value
	}
	return path
}

func buildSummary(change FieldChange, maxValueLen int) string {
	label := change.DisplayName
	if label == "" {
		label = change.Path
	}
	if change.Sensitive {
		return fmt.Sprintf("%s 已变更", label)
	}
	afterText := fmt.Sprint(change.After)
	if maxValueLen > 0 && len(afterText) > maxValueLen {
		afterText = afterText[:maxValueLen] + "..."
	}
	switch change.ChangeType {
	case "added":
		return fmt.Sprintf("%s 已新增", label)
	case "removed":
		return fmt.Sprintf("%s 已移除", label)
	default:
		return fmt.Sprintf("%s 已更新为 %s", label, afterText)
	}
}

func flattenMap(prefix string, input map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range input {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if nested, ok := value.(map[string]any); ok {
			for nestedKey, nestedValue := range flattenMap(path, nested) {
				result[nestedKey] = nestedValue
			}
			continue
		}
		result[path] = value
	}
	return result
}

func shouldIgnoreField(path string, ignores []string) bool {
	return matchesAny(path, ignores)
}

func matchesAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchPath(path, pattern) {
			return true
		}
	}
	return false
}

func matchPath(path, pattern string) bool {
	if pattern == "" {
		return false
	}
	pathParts := strings.Split(path, ".")
	patternParts := strings.Split(pattern, ".")
	if len(pathParts) != len(patternParts) {
		return false
	}
	for i := range pathParts {
		if patternParts[i] == "*" {
			continue
		}
		if pathParts[i] != patternParts[i] {
			return false
		}
	}
	return true
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		if nested, ok := value.(map[string]any); ok {
			dst[key] = cloneAnyMap(nested)
			continue
		}
		dst[key] = value
	}
	return dst
}

func maskNested(input map[string]any, policy PolicyProfile) map[string]any {
	output := cloneAnyMap(input)
	flat := flattenMap("", output)
	for key := range flat {
		if !matchesAny(key, policy.SensitiveFields) && !matchesAny(key, policy.ChangedOnlyFields) {
			continue
		}
		setNestedValue(output, strings.Split(key, "."), "******")
	}
	return output
}

func setNestedValue(target map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}
	current := target
	for i, part := range path {
		if i == len(path)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
}

func maskedValue(value any, ok bool) any {
	if !ok {
		return nil
	}
	return "******"
}
