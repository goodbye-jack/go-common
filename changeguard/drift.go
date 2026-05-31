package changeguard

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/goodbye-jack/go-common/orm"
	"github.com/google/uuid"
)

type DriftFinding struct {
	Field    string `json:"field"`
	RuleType string `json:"rule_type"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Message  string `json:"message"`
}

type DriftReport struct {
	ReportID       string         `json:"report_id"`
	ServiceName    string         `json:"service_name"`
	ResourceKey    string         `json:"resource_key"`
	ResourceType   string         `json:"resource_type"`
	ResourceID     string         `json:"resource_id"`
	Severity       string         `json:"severity"`
	MatchedRules   []string       `json:"matched_rules"`
	Findings       []DriftFinding `json:"findings"`
	Resolved       bool           `json:"resolved"`
	OccurredAtUnix int64          `json:"occurred_at_unix"`
}

type DriftReportStore interface {
	Save(ctx context.Context, report DriftReport) error
}

type NoopDriftReportStore struct{}

func (s *NoopDriftReportStore) Save(context.Context, DriftReport) error {
	return nil
}

type GormDriftReportStore struct{}

func NewGormDriftReportStore() *GormDriftReportStore {
	return &GormDriftReportStore{}
}

func (s *GormDriftReportStore) Save(ctx context.Context, report DriftReport) error {
	if orm.DB == nil {
		return nil
	}
	findingsJSON, _ := json.Marshal(report.Findings)
	ruleKeysJSON, _ := json.Marshal(report.MatchedRules)
	record := &DriftReportRecord{
		ReportID:       chooseNonEmpty(report.ReportID, uuid.NewString()),
		ServiceName:    report.ServiceName,
		ResourceKey:    report.ResourceKey,
		ResourceType:   report.ResourceType,
		ResourceID:     report.ResourceID,
		Severity:       report.Severity,
		FindingsJSON:   string(findingsJSON),
		RuleKeysJSON:   string(ruleKeysJSON),
		Resolved:       report.Resolved,
		OccurredAtUnix: chooseTime(time.Unix(report.OccurredAtUnix, 0)).Unix(),
	}
	return orm.DB.Create(ctx, record)
}

type VersionBaselineResolver struct{}

// Resolve 默认从版本快照中取基线。
// 这样业务侧不需要额外保存“期望配置”，只要开启版本能力就能做基础 drift 检测。
func (r *VersionBaselineResolver) Resolve(ctx context.Context, engine *Engine, resource ResourceProfile, policy PolicyProfile, current *ResourceState) (*ResourceState, []string, error) {
	if engine == nil || engine.versionStore == nil || current == nil {
		return nil, nil, nil
	}
	snapshot, err := engine.versionStore.Latest(ctx, engine.opts.ServiceName, resource.Key, current.ResourceID)
	if err != nil || snapshot == nil {
		return nil, nil, err
	}
	return &ResourceState{
		ResourceType: current.ResourceType,
		ResourceID:   current.ResourceID,
		ResourceName: current.ResourceName,
		Value:        cloneAnyMap(snapshot.Snapshot),
		RawValue:     snapshot.Snapshot,
	}, []string{"version_latest"}, nil
}

type DriftRunner struct {
	engine   *Engine
	store    DriftReportStore
	baseline BaselineResolver
}

// NewDriftRunner 负责组装漂移检测链路：
// 拉取当前值 -> 解析基线 -> 执行内置规则 -> 落 drift 报告。
func NewDriftRunner(engine *Engine, store DriftReportStore, baseline BaselineResolver) *DriftRunner {
	if store == nil {
		store = &NoopDriftReportStore{}
	}
	if baseline == nil {
		baseline = &VersionBaselineResolver{}
	}
	return &DriftRunner{
		engine:   engine,
		store:    store,
		baseline: baseline,
	}
}

// RunOnce 扫描所有启用 drift 的资源。
// 当前优先覆盖 singleton 配置类资源，后续可继续扩展到实体类资源。
func (r *DriftRunner) RunOnce(ctx context.Context) error {
	if r == nil || r.engine == nil {
		return nil
	}
	for _, resource := range r.engine.resources {
		policy, ok := r.engine.policies[resource.PolicyName]
		if !ok || !policy.DriftCheckEnabled {
			continue
		}
		if resource.ProviderType != ProviderSingletonConfig {
			continue
		}
		provider, err := r.engine.providers.resolve(resource)
		if err != nil {
			continue
		}
		session := &Session{
			RequestID: uuid.NewString(),
			StartedAt: time.Now(),
			Resource:  resource,
			Policy:    clonePolicy(policy),
			Store:     map[string]any{},
		}
		current, err := provider.After(session)
		if err != nil || current == nil {
			continue
		}
		baseline, baselineRules, err := r.baseline.Resolve(ctx, r.engine, resource, policy, current)
		if err != nil {
			continue
		}
		findings, ruleKeys := builtinDriftFindings(current, baseline, policy)
		ruleKeys = append(ruleKeys, baselineRules...)
		if len(findings) == 0 {
			continue
		}
		_ = r.store.Save(ctx, DriftReport{
			ReportID:       uuid.NewString(),
			ServiceName:    r.engine.opts.ServiceName,
			ResourceKey:    resource.Key,
			ResourceType:   resource.ResourceType,
			ResourceID:     current.ResourceID,
			Severity:       policy.RiskLevel,
			MatchedRules:   uniqueStrings(ruleKeys),
			Findings:       findings,
			Resolved:       false,
			OccurredAtUnix: time.Now().Unix(),
		})
	}
	return nil
}

// builtinDriftFindings 内置了最常见的关键资源校验：
// 必填、枚举、域名范围、数值偏移、数值区间。
func builtinDriftFindings(current, baseline *ResourceState, policy PolicyProfile) ([]DriftFinding, []string) {
	result := make([]DriftFinding, 0)
	ruleKeys := make([]string, 0)
	currentFlat := flattenMap("", current.Value)
	baselineFlat := map[string]any{}
	if baseline != nil {
		baselineFlat = flattenMap("", baseline.Value)
	}
	for _, field := range policy.DriftFields {
		currentValue, ok := currentFlat[field]
		if ok && currentValue != nil && fmt.Sprint(currentValue) != "" {
			continue
		}
		result = append(result, DriftFinding{
			Field:    field,
			RuleType: "required_not_empty",
			Message:  field + " 当前为空或缺失",
		})
		ruleKeys = append(ruleKeys, "required_not_empty")
	}
	for field, expression := range policy.DriftThresholds {
		findings := checkDriftExpression(field, expression, currentFlat[field], baselineFlat[field])
		if len(findings) == 0 {
			continue
		}
		result = append(result, findings...)
		ruleKeys = append(ruleKeys, driftRuleNames(findings)...)
	}
	return result, ruleKeys
}

// checkDriftExpression 通过轻量 DSL 解释阈值规则，
// 让业务侧大部分场景只配字符串规则即可，不必编写自定义代码。
func checkDriftExpression(field, expression string, current any, baseline any) []DriftFinding {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil
	}
	switch {
	case expression == "required":
		if current == nil || strings.TrimSpace(fmt.Sprint(current)) == "" {
			return []DriftFinding{{
				Field:    field,
				RuleType: "required_not_empty",
				Expected: "non-empty",
				Actual:   fmt.Sprint(current),
				Message:  field + " 当前为空或缺失",
			}}
		}
	case strings.HasPrefix(expression, "enum:"):
		allowed := compactStrings(strings.Split(strings.TrimPrefix(expression, "enum:"), "|"))
		actual := strings.TrimSpace(fmt.Sprint(current))
		if actual == "" || containsString(allowed, actual) {
			return nil
		}
		return []DriftFinding{{
			Field:    field,
			RuleType: "enum",
			Expected: strings.Join(allowed, "|"),
			Actual:   actual,
			Message:  field + " 不在允许枚举范围内",
		}}
	case strings.HasPrefix(expression, "domain:"):
		allowed := compactStrings(strings.Split(strings.TrimPrefix(expression, "domain:"), "|"))
		actual := strings.TrimSpace(fmt.Sprint(current))
		if actual == "" || hasDomainPrefix(actual, allowed) {
			return nil
		}
		return []DriftFinding{{
			Field:    field,
			RuleType: "domain",
			Expected: strings.Join(allowed, "|"),
			Actual:   actual,
			Message:  field + " 不在允许域名范围内",
		}}
	case strings.HasPrefix(expression, "max_delta:"):
		limit, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(expression, "max_delta:")), 64)
		if err != nil {
			return nil
		}
		currentValue, currentOK := toFloat64(current)
		baselineValue, baselineOK := toFloat64(baseline)
		if !currentOK || !baselineOK {
			return nil
		}
		delta := currentValue - baselineValue
		if delta < 0 {
			delta = -delta
		}
		if delta <= limit {
			return nil
		}
		return []DriftFinding{{
			Field:    field,
			RuleType: "max_delta",
			Expected: fmt.Sprintf("<= %.4f", limit),
			Actual:   fmt.Sprintf("%.4f", delta),
			Message:  field + " 偏移量超过阈值",
		}}
	case strings.HasPrefix(expression, "range:"):
		values := compactStrings(strings.Split(strings.TrimPrefix(expression, "range:"), ","))
		if len(values) != 2 {
			return nil
		}
		minValue, err1 := strconv.ParseFloat(values[0], 64)
		maxValue, err2 := strconv.ParseFloat(values[1], 64)
		actual, ok := toFloat64(current)
		if err1 != nil || err2 != nil || !ok {
			return nil
		}
		if actual >= minValue && actual <= maxValue {
			return nil
		}
		return []DriftFinding{{
			Field:    field,
			RuleType: "range",
			Expected: fmt.Sprintf("%.4f~%.4f", minValue, maxValue),
			Actual:   fmt.Sprintf("%.4f", actual),
			Message:  field + " 超出允许范围",
		}}
	}
	return nil
}

func driftRuleNames(findings []DriftFinding) []string {
	result := make([]string, 0, len(findings))
	for _, finding := range findings {
		if strings.TrimSpace(finding.RuleType) == "" {
			continue
		}
		result = append(result, finding.RuleType)
	}
	return result
}

func toFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed, err == nil
	default:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(value)), 64)
		return parsed, err == nil
	}
}

func hasDomainPrefix(value string, domains []string) bool {
	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		if strings.Contains(value, domain) {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
