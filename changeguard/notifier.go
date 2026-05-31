package changeguard

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/goodbye-jack/go-common/orm"
)

type Message struct {
	Channel    string
	Template   string
	Recipients []string
	Subject    string
	Content    string
	Variables  map[string]string
	EventID    string
	RiskLevel  string
}

type Notifier interface {
	Name() string
	Notify(ctx context.Context, msg Message) error
}

type RecipientResolver interface {
	Resolve(ctx context.Context, event ChangeEvent, channel string) ([]string, error)
}

type TemplateRenderer interface {
	Render(template string, event ChangeEvent, channel string) (Message, error)
}

type StaticRecipientResolver struct{}

func (r *StaticRecipientResolver) Resolve(context.Context, ChangeEvent, string) ([]string, error) {
	return nil, nil
}

type DefaultTemplateRenderer struct{}

// Render 提供一个通用兜底模板。
// 业务侧不接自定义模板渲染器时，也能直接得到可投递的通知消息。
func (r *DefaultTemplateRenderer) Render(template string, event ChangeEvent, channel string) (Message, error) {
	subject := fmt.Sprintf("[%s] %s", strings.ToUpper(chooseNonEmpty(event.RiskLevel, RiskLevelMedium)), chooseNonEmpty(event.ResourceName, event.ResourceKey))
	content := buildDefaultNotificationContent(&event)
	variables := map[string]string{
		"event_id":      event.EventID,
		"service_name":  event.ServiceName,
		"policy_name":   event.PolicyName,
		"resource_key":  event.ResourceKey,
		"resource_name": event.ResourceName,
		"resource_id":   event.ResourceID,
		"action":        event.Action,
		"risk_level":    event.RiskLevel,
		"operator_id":   event.Principal.UserID,
		"operator_name": event.Principal.UserName,
		"tenant_code":   event.Principal.TenantCode,
		"path":          event.Path,
		"method":        event.Method,
	}
	return Message{
		Channel:   channel,
		Template:  template,
		Subject:   subject,
		Content:   content,
		Variables: variables,
		EventID:   event.EventID,
		RiskLevel: event.RiskLevel,
	}, nil
}

type NoopNotifier struct {
	name string
}

func NewNoopNotifier(name string) *NoopNotifier {
	return &NoopNotifier{name: chooseNonEmpty(name, "noop")}
}

func (n *NoopNotifier) Name() string {
	return n.name
}

func (n *NoopNotifier) Notify(context.Context, Message) error {
	return nil
}

type RetryPolicy struct {
	Delays []time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		Delays: []time.Duration{
			time.Minute,
			5 * time.Minute,
			15 * time.Minute,
			time.Hour,
			4 * time.Hour,
		},
	}
}

type NotifierHub struct {
	notifiers map[string]Notifier
}

func NewNotifierHub() *NotifierHub {
	return &NotifierHub{notifiers: map[string]Notifier{}}
}

func (h *NotifierHub) Register(values ...Notifier) {
	for _, value := range values {
		if value == nil || value.Name() == "" {
			continue
		}
		h.notifiers[value.Name()] = value
	}
}

func (h *NotifierHub) Dispatch(ctx context.Context, msg Message) error {
	notifier := h.notifiers[msg.Channel]
	if notifier == nil {
		return nil
	}
	return notifier.Notify(ctx, msg)
}

type Dispatcher struct {
	hub               *NotifierHub
	recipientResolver RecipientResolver
	templateRenderer  TemplateRenderer
	retry             RetryPolicy
}

// NewDispatcher 组装通知分发主链路：
// 事件重建 -> 渲染消息 -> 解析收件人 -> 调用具体 notifier。
func NewDispatcher(hub *NotifierHub, resolver RecipientResolver, renderer TemplateRenderer, retry RetryPolicy) *Dispatcher {
	if hub == nil {
		hub = NewNotifierHub()
	}
	if resolver == nil {
		resolver = &StaticRecipientResolver{}
	}
	if renderer == nil {
		renderer = &DefaultTemplateRenderer{}
	}
	if len(retry.Delays) == 0 {
		retry = DefaultRetryPolicy()
	}
	return &Dispatcher{
		hub:               hub,
		recipientResolver: resolver,
		templateRenderer:  renderer,
		retry:             retry,
	}
}

// ProcessPending 扫描待发送/待重试事件，逐条投递。
// 这里保持 fail-open：通知失败只进入重试，不反向影响业务主流程。
func (d *Dispatcher) ProcessPending(ctx context.Context, limit int) error {
	if orm.DB == nil {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	records := make([]EventRecord, 0, limit)
	err := orm.DB.GetDB().WithContext(ctx).
		Model(&EventRecord{}).
		Where("notify_status IN ?", []string{"pending", "failed"}).
		Order("id ASC").
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return err
	}
	for _, record := range records {
		if err := d.processOne(ctx, &record); err != nil {
			return err
		}
	}
	return nil
}

// processOne 负责单条事件的完整异步通知链路。
func (d *Dispatcher) processOne(ctx context.Context, record *EventRecord) error {
	if record == nil {
		return nil
	}
	event, err := d.toEvent(record)
	if err != nil {
		return d.markRetry(ctx, record, err)
	}
	metadata := cloneStringMap(event.Metadata)
	channels := resolveNotifyChannels(metadata)
	if !shouldNotifyForEvent(event, metadata) {
		return d.markSuccess(ctx, record, "skipped")
	}
	for _, channel := range channels {
		msg, err := d.templateRenderer.Render(metadata["notify_template"], event, channel)
		if err != nil {
			return d.markRetry(ctx, record, err)
		}
		recipients, err := d.recipientResolver.Resolve(ctx, event, channel)
		if err != nil {
			return d.markRetry(ctx, record, err)
		}
		msg.Channel = channel
		msg.Template = chooseNonEmpty(msg.Template, metadata["notify_template"])
		msg.Recipients = compactStrings(append(msg.Recipients, recipients...))
		msg.EventID = event.EventID
		msg.RiskLevel = event.RiskLevel
		if err := d.hub.Dispatch(ctx, msg); err != nil {
			return d.markRetry(ctx, record, err)
		}
	}
	return d.markSuccess(ctx, record, "notified")
}

// toEvent 将落库事件重新还原成 ChangeEvent，避免 dispatcher 依赖业务侧运行时上下文。
func (d *Dispatcher) toEvent(record *EventRecord) (ChangeEvent, error) {
	event := ChangeEvent{
		EventID:      record.EventID,
		ServiceName:  record.ServiceName,
		ResourceKey:  record.ResourceKey,
		ResourceType: record.ResourceType,
		ResourceID:   record.ResourceID,
		ResourceName: record.ResourceName,
		Action:       record.Action,
		RiskLevel:    record.RiskLevel,
		Path:         record.Path,
		Method:       record.Method,
		Success:      record.Success,
		RequestID:    record.RequestID,
		NotifyStatus: record.NotifyStatus,
		OccurredAt:   time.Unix(record.OccurredAtUnix, 0),
		Principal: Principal{
			UserID:      record.OperatorID,
			UserName:    record.OperatorName,
			UserAccount: record.OperatorAccount,
			TenantCode:  record.TenantCode,
			ClientIP:    record.ClientIP,
			ClientType:  record.ClientType,
		},
	}
	if record.MetadataJSON != "" {
		if err := json.Unmarshal([]byte(record.MetadataJSON), &event.Metadata); err != nil {
			return ChangeEvent{}, err
		}
		event.PolicyName = event.Metadata["policy_name"]
	}
	if record.ChangesJSON != "" {
		if err := json.Unmarshal([]byte(record.ChangesJSON), &event.Changes); err != nil {
			return ChangeEvent{}, err
		}
	}
	if record.SummaryJSON != "" {
		if err := json.Unmarshal([]byte(record.SummaryJSON), &event.Summary); err != nil {
			return ChangeEvent{}, err
		}
	}
	return event, nil
}

func (d *Dispatcher) markSuccess(ctx context.Context, record *EventRecord, status string) error {
	return orm.DB.GetDB().WithContext(ctx).
		Model(&EventRecord{}).
		Where("id = ?", record.ID).
		Updates(map[string]any{
			"notify_status":     "success",
			"process_status":    status,
			"last_notify_error": "",
		}).Error
}

func (d *Dispatcher) markRetry(ctx context.Context, record *EventRecord, sourceErr error) error {
	attempts := record.NotifyAttempts + 1
	status := "failed"
	nextAtUnix := int64(0)
	if attempts < len(d.retry.Delays)+1 {
		status = "pending"
		nextAtUnix = time.Now().Add(d.retry.Delays[attempts-1]).Unix()
	}
	metadata := map[string]any{}
	if record.MetadataJSON != "" {
		_ = json.Unmarshal([]byte(record.MetadataJSON), &metadata)
	}
	if nextAtUnix > 0 {
		metadata["notify_retry_at_unix"] = nextAtUnix
	}
	metadataJSON, _ := json.Marshal(metadata)
	return orm.DB.GetDB().WithContext(ctx).
		Model(&EventRecord{}).
		Where("id = ?", record.ID).
		Updates(map[string]any{
			"notify_status":     status,
			"notify_attempts":   attempts,
			"last_notify_error": truncateString(sourceErr.Error(), 1024),
			"process_status":    "new",
			"metadata_json":     string(metadataJSON),
		}).Error
}

// shouldNotifyForEvent 支持按 action / risk level 做策略过滤，
// 让业务侧尽量只配策略，不再手写 if/else。
func shouldNotifyForEvent(event ChangeEvent, metadata map[string]string) bool {
	actions := splitCSV(metadata["notify_on_actions"])
	if len(actions) > 0 && !containsString(actions, event.Action) {
		return false
	}
	riskLevels := splitCSV(metadata["notify_on_risk_levels"])
	if len(riskLevels) > 0 && !containsString(riskLevels, event.RiskLevel) {
		return false
	}
	return true
}

func resolveNotifyChannels(metadata map[string]string) []string {
	channels := splitCSV(metadata["notify_channels"])
	if len(channels) == 0 {
		return []string{"sms"}
	}
	return channels
}

func buildDefaultNotificationContent(event *ChangeEvent) string {
	if event == nil {
		return ""
	}
	resourceName := chooseNonEmpty(event.ResourceName, event.ResourceKey)
	operator := chooseNonEmpty(event.Principal.UserName, event.Principal.UserAccount, event.Principal.UserID, "未知操作人")
	actionText := actionDisplayName(event.Action)
	businessSummary := buildEventBusinessSummary(event)
	summary := summarizeFieldChanges(event.Changes, 2)
	switch {
	case businessSummary != "" && summary != "":
		return fmt.Sprintf("关键资源通知：%s已%s，摘要：%s，变更内容：%s，操作人：%s。", resourceName, actionText, businessSummary, summary, operator)
	case businessSummary != "":
		return fmt.Sprintf("关键资源通知：%s已%s，摘要：%s，操作人：%s。", resourceName, actionText, businessSummary, operator)
	case summary != "":
		return fmt.Sprintf("关键资源通知：%s已%s，变更内容：%s，操作人：%s。", resourceName, actionText, summary, operator)
	}
	return fmt.Sprintf("关键资源通知：%s已%s，操作人：%s。", resourceName, actionText, operator)
}

func actionDisplayName(action string) string {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "save":
		return "更新"
	case "update":
		return "更新"
	case "publish":
		return "发布"
	case "toggle":
		return "切换"
	case "enable":
		return "启用"
	case "disable":
		return "停用"
	case "rollback":
		return "回滚"
	default:
		if strings.TrimSpace(action) == "" {
			return "变更"
		}
		return strings.TrimSpace(action)
	}
}

func summarizeFieldChanges(changes []FieldChange, limit int) string {
	if len(changes) == 0 {
		return ""
	}
	if limit <= 0 {
		limit = 2
	}
	items := make([]string, 0, limit)
	for _, change := range changes {
		label := chooseNonEmpty(change.DisplayName, change.Path)
		switch change.ChangeType {
		case "added":
			items = append(items, fmt.Sprintf("%s新增为%s", label, formatNotificationValue(change.After)))
		case "removed":
			items = append(items, fmt.Sprintf("%s已移除", label))
		default:
			items = append(items, fmt.Sprintf("%s调整为%s", label, formatNotificationValue(change.After)))
		}
		if len(items) >= limit {
			break
		}
	}
	return strings.Join(items, "；")
}

func formatNotificationValue(value any) string {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return "-"
	}
	if len(text) > 48 {
		return text[:48] + "..."
	}
	return text
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	return compactStrings(parts)
}

func joinCSV(values []string) string {
	return strings.Join(compactStrings(values), ",")
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(target) {
			return true
		}
	}
	return false
}

func truncateString(value string, maxLen int) string {
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}
