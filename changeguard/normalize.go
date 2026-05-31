package changeguard

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

func NormalizeAny(raw any, policy PolicyProfile) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}
	switch typed := raw.(type) {
	case map[string]any:
		return NormalizeMap(typed, policy), nil
	default:
		value := reflect.ValueOf(raw)
		if value.Kind() == reflect.Ptr && value.IsNil() {
			return map[string]any{}, nil
		}
		bytes, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("marshal normalize source failed: %w", err)
		}
		result := map[string]any{}
		if err := json.Unmarshal(bytes, &result); err != nil {
			return nil, fmt.Errorf("unmarshal normalize source failed: %w", err)
		}
		return NormalizeMap(result, policy), nil
	}
}

func NormalizeMap(input map[string]any, policy PolicyProfile) map[string]any {
	normalized := normalizeMapRecursive(input, policy.SliceToMapBy)
	if len(policy.IncludeFields) == 0 && len(policy.IgnoreFields) == 0 {
		return normalized
	}
	flat := flattenMap("", normalized)
	filtered := map[string]any{}
	for key, value := range flat {
		if shouldIgnoreField(key, policy.IgnoreFields) {
			continue
		}
		if len(policy.IncludeFields) > 0 && !matchesAny(key, policy.IncludeFields) {
			continue
		}
		setNestedValue(filtered, strings.Split(key, "."), value)
	}
	return filtered
}

func normalizeMapRecursive(input map[string]any, sliceToMapBy map[string]string) map[string]any {
	result := map[string]any{}
	for key, value := range input {
		switch typed := value.(type) {
		case map[string]any:
			result[key] = normalizeMapRecursive(typed, sliceToMapBy)
		case []any:
			if mapKey := sliceToMapBy[key]; mapKey != "" {
				result[key] = sliceToMap(typed, mapKey, sliceToMapBy)
			} else {
				result[key] = normalizeSlice(typed, sliceToMapBy)
			}
		default:
			result[key] = value
		}
	}
	return result
}

func normalizeSlice(items []any, sliceToMapBy map[string]string) []any {
	result := make([]any, 0, len(items))
	for _, item := range items {
		if nested, ok := item.(map[string]any); ok {
			result = append(result, normalizeMapRecursive(nested, sliceToMapBy))
			continue
		}
		result = append(result, item)
	}
	return result
}

func sliceToMap(items []any, keyField string, sliceToMapBy map[string]string) map[string]any {
	result := map[string]any{}
	for _, item := range items {
		nested, ok := item.(map[string]any)
		if !ok {
			continue
		}
		normalized := normalizeMapRecursive(nested, sliceToMapBy)
		mapKey := fmt.Sprint(normalized[keyField])
		if strings.TrimSpace(mapKey) == "" {
			continue
		}
		result[mapKey] = normalized
	}
	return result
}
