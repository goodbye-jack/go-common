package changeguard

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type businessSummaryField struct {
	Key   string
	Label string
}

func buildBusinessSummaryText(scenarioKey string, values map[string]any, limit int) string {
	if len(values) == 0 {
		return ""
	}
	if limit <= 0 {
		limit = 4
	}
	fields := preferredBusinessSummaryFields(scenarioKey)
	if len(fields) == 0 {
		fields = defaultBusinessSummaryFields()
	}
	parts := make([]string, 0, limit)
	seen := map[string]struct{}{}
	appendField := func(field businessSummaryField) {
		if len(parts) >= limit {
			return
		}
		key := strings.TrimSpace(field.Key)
		if key == "" {
			return
		}
		if _, ok := seen[strings.ToLower(key)]; ok {
			return
		}
		value, ok := findMapValueCaseInsensitive(values, key)
		if !ok || value == nil {
			return
		}
		formatted := formatBusinessSummaryValue(key, value)
		if formatted == "" {
			return
		}
		label := strings.TrimSpace(field.Label)
		if label == "" {
			label = key
		}
		parts = append(parts, fmt.Sprintf("%s=%s", label, formatted))
		seen[strings.ToLower(key)] = struct{}{}
	}
	for _, field := range fields {
		appendField(field)
	}
	if len(parts) >= limit {
		return strings.Join(parts, "；")
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if _, ok := seen[lowerKey]; ok {
			continue
		}
		if isSensitiveSummaryField(lowerKey) {
			continue
		}
		appendField(businessSummaryField{Key: key, Label: normalizeSummaryLabel(key)})
		if len(parts) >= limit {
			break
		}
	}
	return strings.Join(parts, "；")
}

func buildEventBusinessSummary(event *ChangeEvent) string {
	if event == nil {
		return ""
	}
	scenarioKey := strings.TrimSpace(event.Metadata["scenario_key"])
	if summary := buildBusinessSummaryText(scenarioKey, event.AfterMasked, 4); summary != "" {
		return summary
	}
	if summary := buildBusinessSummaryText(scenarioKey, event.BeforeMasked, 4); summary != "" {
		return summary
	}
	return ""
}

func preferredBusinessSummaryFields(scenarioKey string) []businessSummaryField {
	switch strings.TrimSpace(scenarioKey) {
	case "membership_plan_guard":
		return []businessSummaryField{
			{Key: "plan_name", Label: "套餐名称"},
			{Key: "plan_code", Label: "套餐编码"},
			{Key: "sale_price_fen", Label: "销售价"},
			{Key: "list_price_fen", Label: "原价"},
			{Key: "duration_days", Label: "有效期"},
			{Key: "plan_type", Label: "套餐类型"},
		}
	case "token_package_guard":
		return []businessSummaryField{
			{Key: "package_name", Label: "充值包名称"},
			{Key: "name", Label: "充值包名称"},
			{Key: "package_code", Label: "充值包编码"},
			{Key: "token_amount", Label: "Token数量"},
			{Key: "sale_price_fen", Label: "销售价"},
			{Key: "list_price_fen", Label: "原价"},
			{Key: "duration_days", Label: "有效期"},
		}
	case "storage_package_guard":
		return []businessSummaryField{
			{Key: "package_name", Label: "容量包名称"},
			{Key: "name", Label: "容量包名称"},
			{Key: "package_code", Label: "容量包编码"},
			{Key: "storage_gb", Label: "容量"},
			{Key: "sale_price_fen", Label: "销售价"},
			{Key: "list_price_fen", Label: "原价"},
			{Key: "duration_days", Label: "有效期"},
		}
	default:
		return defaultBusinessSummaryFields()
	}
}

func defaultBusinessSummaryFields() []businessSummaryField {
	return []businessSummaryField{
		{Key: "name", Label: "名称"},
		{Key: "plan_name", Label: "名称"},
		{Key: "package_name", Label: "名称"},
		{Key: "resource_name", Label: "名称"},
		{Key: "id", Label: "ID"},
		{Key: "plan_code", Label: "编码"},
		{Key: "package_code", Label: "编码"},
		{Key: "sale_price_fen", Label: "销售价"},
		{Key: "list_price_fen", Label: "原价"},
		{Key: "duration_days", Label: "有效期"},
	}
}

func formatBusinessSummaryValue(key string, value any) string {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "sale_price_fen", "list_price_fen", "market_price_fen":
		return formatFenAmount(value)
	case "duration_days":
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
			return text + "天"
		}
		return ""
	case "storage_gb":
		if text := strings.TrimSpace(formatNotificationValue(value)); text != "" && text != "-" {
			return text + "GB"
		}
		return ""
	case "token_amount":
		return strings.TrimSpace(formatNotificationValue(value))
	default:
		text := strings.TrimSpace(formatNotificationValue(value))
		if text == "-" {
			return ""
		}
		return text
	}
}

func formatFenAmount(value any) string {
	if amount, ok := toInt64(value); ok {
		sign := ""
		if amount < 0 {
			sign = "-"
			amount = -amount
		}
		return fmt.Sprintf("%s%d.%02d元", sign, amount/100, amount%100)
	}
	text := strings.TrimSpace(formatNotificationValue(value))
	if text == "-" {
		return ""
	}
	return text
}

func toInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > ^uint64(0)>>1 {
			return 0, false
		}
		return int64(typed), true
	case float32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	case string:
		if strings.TrimSpace(typed) == "" {
			return 0, false
		}
		number, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return number, true
		}
		floatNumber, floatErr := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if floatErr == nil {
			return int64(floatNumber), true
		}
	}
	return 0, false
}

func normalizeSummaryLabel(key string) string {
	key = strings.TrimSpace(key)
	switch strings.ToLower(key) {
	case "plan_name":
		return "套餐名称"
	case "plan_code":
		return "套餐编码"
	case "package_name":
		return "名称"
	case "package_code":
		return "编码"
	case "sale_price_fen":
		return "销售价"
	case "list_price_fen":
		return "原价"
	case "duration_days":
		return "有效期"
	case "storage_gb":
		return "容量"
	case "token_amount":
		return "Token数量"
	default:
		return key
	}
}

func isSensitiveSummaryField(key string) bool {
	switch key {
	case "password", "secret", "token", "private_key", "public_key", "app_secret", "access_key_secret":
		return true
	default:
		return false
	}
}
