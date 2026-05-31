package changeguard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	goodhttp "github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/log"
	"github.com/google/uuid"
)

type Engine struct {
	opts              EngineOptions
	policies          map[string]PolicyProfile
	resources         map[string]ResourceProfile
	bindings          []RouteBinding
	providers         *providerRegistry
	sink              EventSink
	versionStore      VersionStore
	driftStore        DriftReportStore
	notifiers         map[string]Notifier
	notifierHub       *NotifierHub
	dispatcher        *Dispatcher
	driftRunner       *DriftRunner
	recipientResolver RecipientResolver
	templateRenderer  TemplateRenderer
	baselineResolver  BaselineResolver
	secondFactor      *secondFactorService
	workers           *workerManager
}

func NewEngine(opts EngineOptions) *Engine {
	opts = opts.normalize()
	return &Engine{
		opts:              opts,
		policies:          map[string]PolicyProfile{},
		resources:         map[string]ResourceProfile{},
		bindings:          []RouteBinding{},
		providers:         newProviderRegistry(),
		sink:              &NoopSink{},
		versionStore:      &NoopVersionStore{},
		driftStore:        &NoopDriftReportStore{},
		notifiers:         map[string]Notifier{},
		notifierHub:       NewNotifierHub(),
		recipientResolver: &StaticRecipientResolver{},
		templateRenderer:  &DefaultTemplateRenderer{},
		baselineResolver:  &VersionBaselineResolver{},
	}
}

func (e *Engine) SetSink(sink EventSink) {
	if sink == nil {
		return
	}
	e.sink = sink
}

func (e *Engine) SetVersionStore(store VersionStore) {
	if store == nil {
		return
	}
	e.versionStore = store
}

func (e *Engine) SetDriftReportStore(store DriftReportStore) {
	if store == nil {
		return
	}
	e.driftStore = store
}

func (e *Engine) RegisterPolicies(values ...PolicyProfile) {
	for _, value := range values {
		if value.Name == "" {
			continue
		}
		if value.RiskLevel == "" {
			value.RiskLevel = e.opts.DefaultRiskLevel
		}
		if value.FailMode == "" {
			value.FailMode = e.opts.FailMode
		}
		if value.MaxDiffChanges <= 0 {
			value.MaxDiffChanges = e.opts.DefaultMaxDiffChanges
		}
		if value.SummaryFieldLimit <= 0 {
			value.SummaryFieldLimit = e.opts.DefaultSummaryFieldLimit
		}
		if value.SummaryValueLimit <= 0 {
			value.SummaryValueLimit = e.opts.DefaultSummaryValueLimit
		}
		if !value.VersioningEnabled && e.opts.DefaultVersionEnabled {
			value.VersioningEnabled = true
		}
		if !value.RollbackEnabled && e.opts.DefaultRollbackEnabled {
			value.RollbackEnabled = true
		}
		if !value.DriftCheckEnabled && e.opts.DefaultDriftEnabled {
			value.DriftCheckEnabled = true
		}
		if len(value.NotifyChannels) == 0 {
			value.NotifyChannels = append([]string{}, e.opts.DefaultNotifyChannels...)
		}
		e.policies[value.Name] = value
	}
}

func (e *Engine) RegisterResources(values ...ResourceProfile) {
	for _, value := range values {
		if value.Key == "" {
			continue
		}
		if value.ResourceType == "" {
			value.ResourceType = value.Key
		}
		e.resources[value.Key] = value
	}
}

func (e *Engine) RegisterBindings(values ...RouteBinding) {
	for _, value := range values {
		if value.Path == "" || value.ResourceKey == "" {
			continue
		}
		if len(value.Methods) == 0 {
			value.Methods = []string{"POST"}
		}
		if value.Action == "" {
			value.Action = ActionUpdate
		}
		e.bindings = append(e.bindings, value)
	}
}

func (e *Engine) RegisterSingletonFetcher(name string, fetcher SingletonFetcher) {
	if name == "" || fetcher == nil {
		return
	}
	e.providers.singletonFetchers[name] = fetcher
}

func (e *Engine) RegisterCustomProvider(name string, provider CustomProvider) {
	if name == "" || provider == nil {
		return
	}
	e.providers.customProviders[name] = provider
}

func (e *Engine) RegisterNotifier(values ...Notifier) {
	for _, value := range values {
		if value == nil || value.Name() == "" {
			continue
		}
		e.notifiers[value.Name()] = value
		e.notifierHub.Register(value)
	}
}

func (e *Engine) SetRecipientResolver(resolver RecipientResolver) {
	if resolver == nil {
		return
	}
	e.recipientResolver = resolver
}

func (e *Engine) SetTemplateRenderer(renderer TemplateRenderer) {
	if renderer == nil {
		return
	}
	e.templateRenderer = renderer
}

func (e *Engine) SetBaselineResolver(resolver BaselineResolver) {
	if resolver == nil {
		return
	}
	e.baselineResolver = resolver
}

func (e *Engine) SetSecondFactor(service *secondFactorService) {
	if service == nil {
		return
	}
	e.secondFactor = service
}

func (e *Engine) Bind(server *goodhttp.HTTPServer) error {
	if !e.opts.Enabled || server == nil {
		return nil
	}
	if err := e.prepareStorage(); err != nil {
		e.handleRuntimeError(err)
	}
	if e.dispatcher == nil {
		e.dispatcher = NewDispatcher(e.notifierHub, e.recipientResolver, e.templateRenderer, DefaultRetryPolicy())
	}
	if e.driftRunner == nil {
		e.driftRunner = NewDriftRunner(e, e.driftStore, e.baselineResolver)
	}
	if e.opts.AutoStartWorkers {
		e.ensureWorkersStarted()
	}
	applied := 0
	for _, route := range server.GetRoutes() {
		for _, binding := range e.bindings {
			if !binding.Enabled || route.Url != binding.Path || !methodMatched(route.Methods, binding.Methods) {
				continue
			}
			route.AddMiddleware(e.buildMiddleware(binding))
			applied++
		}
	}
	log.Infof("changeguard bind complete, service=%s, bindings=%d, applied=%d", e.opts.ServiceName, len(e.bindings), applied)
	return nil
}

// prepareStorage 在 worker 启动前统一准备 changeguard 相关表结构。
// 这样可以避免后台 worker 先查询，再因为懒建表而持续报“表不存在”警告。
func (e *Engine) prepareStorage() error {
	if e == nil {
		return nil
	}
	return autoMigrateChangeguardTables()
}

func (e *Engine) buildMiddleware(binding RouteBinding) gin.HandlerFunc {
	return func(c *gin.Context) {
		resource, ok := e.resources[binding.ResourceKey]
		if !ok {
			e.handleRuntimeError(fmt.Errorf("changeguard resource not found: %s", binding.ResourceKey))
			c.Next()
			return
		}
		policy, ok := e.policies[resource.PolicyName]
		if !ok {
			e.handleRuntimeError(fmt.Errorf("changeguard policy not found: %s", resource.PolicyName))
			c.Next()
			return
		}
		provider, err := e.providers.resolve(resource)
		if err != nil {
			e.handleRuntimeError(err)
			c.Next()
			return
		}
		session := &Session{
			RequestID: uuid.NewString(),
			StartedAt: time.Now(),
			Context:   c,
			Principal: ResolvePrincipal(c),
			Binding:   binding,
			Resource:  resource,
			Policy:    clonePolicy(policy),
			Action:    binding.Action,
			Store:     map[string]any{},
			RequestMeta: RequestMeta{
				RequestID:   firstString(c.GetHeader(e.opts.RequestIDHeader), uuid.NewString()),
				Path:        c.FullPath(),
				Method:      c.Request.Method,
				QueryString: c.Request.URL.RawQuery,
			},
		}
		if rawBody, err := GetCachedRequestBody(c); err == nil {
			session.RequestMeta.RawBody = rawBody
		}
		if e.secondFactor != nil && e.secondFactor.shouldProtect(session.Policy, session.Action) {
			result := e.secondFactor.ensureVerified(c, session)
			if result.Responded && !result.Allowed {
				c.AbortWithStatusJSON(result.HTTPStatus, gin.H{
					"code":    result.ResponseCode,
					"message": result.ResponseMessage,
					"data":    result.ResponseData,
				})
				return
			}
		}
		before, err := provider.Before(session)
		if err != nil {
			e.handleRuntimeError(err)
		}
		c.Next()
		session.ResponseMeta = ResponseMeta{StatusCode: c.Writer.Status()}
		if challengeID := strings.TrimSpace(fmt.Sprint(session.Store["second_factor_challenge_id"])); challengeID != "" && session.isSuccess() && e.secondFactor != nil {
			e.secondFactor.consumeChallenge(context.Background(), challengeID)
		}
		if !session.isSuccess() && policy.NotifyOnlyOnSuccess {
			return
		}
		after, err := provider.After(session)
		if err != nil {
			e.handleRuntimeError(err)
		}
		beforeValue := map[string]any{}
		afterValue := map[string]any{}
		resourceID := ""
		resourceName := chooseNonEmpty(resource.Name, resource.Key)
		if before != nil {
			beforeValue = before.Value
			resourceID = before.ResourceID
			if before.ResourceName != "" {
				resourceName = before.ResourceName
			}
		}
		if after != nil {
			afterValue = after.Value
			if resourceID == "" {
				resourceID = after.ResourceID
			}
			if after.ResourceName != "" {
				resourceName = after.ResourceName
			}
		}
		diffResult, err := Compare(beforeValue, afterValue, policy)
		if err != nil {
			e.handleRuntimeError(err)
			return
		}
		if !diffResult.Changed {
			return
		}
		event := ChangeEvent{
			EventID:      uuid.NewString(),
			ServiceName:  chooseNonEmpty(e.opts.ServiceName, c.FullPath()),
			PolicyName:   policy.Name,
			ResourceKey:  resource.Key,
			ResourceType: resource.ResourceType,
			ResourceID:   resourceID,
			ResourceName: resourceName,
			Action:       binding.Action,
			RiskLevel:    chooseNonEmpty(binding.OverrideRisk, policy.RiskLevel, e.opts.DefaultRiskLevel),
			Path:         c.FullPath(),
			Method:       c.Request.Method,
			Success:      session.isSuccess(),
			OccurredAt:   time.Now(),
			Principal:    session.Principal,
			BeforeMasked: diffResult.BeforeMasked,
			AfterMasked:  diffResult.AfterMasked,
			Changes:      diffResult.Changes,
			Summary:      diffResult.Summary,
			RequestID:    session.RequestMeta.RequestID,
			NotifyStatus: "pending",
			Metadata:     e.buildEventMetadata(binding, policy),
		}
		if policy.VersioningEnabled {
			versionID, err := e.versionStore.Save(context.Background(), SaveVersionRequest{
				ServiceName:  event.ServiceName,
				ResourceKey:  event.ResourceKey,
				ResourceType: event.ResourceType,
				ResourceID:   event.ResourceID,
				ResourceName: event.ResourceName,
				Action:       event.Action,
				RiskLevel:    event.RiskLevel,
				Snapshot:     cloneAnyMap(afterValue),
				EventID:      event.EventID,
				Operator:     event.Principal,
				Tags:         cloneStringMap(policy.Tags),
			})
			if err != nil {
				e.handleRuntimeError(err)
			} else if versionID != "" {
				event.VersionID = versionID
			}
		}
		if err := e.sink.Emit(context.Background(), event); err != nil {
			e.handleRuntimeError(err)
			return
		}
		// 变更事件落库成功后，立即异步尝试分发一次通知。
		// 这样不必完全等待后台轮询周期，用户能更快收到关键资源变更提醒；
		// 后台 worker 仍保留，继续承担失败重试和兜底职责。
		if e.dispatcher != nil && e.opts.DispatcherEnabled {
			go func() {
				if err := e.ProcessPendingNotifications(context.Background(), 1); err != nil {
					e.handleRuntimeError(err)
				}
			}()
		}
	}
}

func (e *Engine) buildEventMetadata(binding RouteBinding, policy PolicyProfile) map[string]string {
	result := cloneStringMap(binding.Metadata)
	if result == nil {
		result = map[string]string{}
	}
	// 这里把策略层的通知配置一并落到事件元数据中，后续异步 dispatcher
	// 不需要再回查业务配置，也避免通知逻辑反向耦合业务代码。
	if len(policy.NotifyChannels) > 0 {
		result["notify_channels"] = joinCSV(policy.NotifyChannels)
	}
	if policy.NotifyTemplate != "" {
		result["notify_template"] = policy.NotifyTemplate
	}
	if len(policy.NotifyOnActions) > 0 {
		result["notify_on_actions"] = joinCSV(policy.NotifyOnActions)
	}
	if len(policy.NotifyOnRiskLevels) > 0 {
		result["notify_on_risk_levels"] = joinCSV(policy.NotifyOnRiskLevels)
	}
	result["policy_name"] = policy.Name
	return result
}

// ProcessPendingNotifications 提供给业务侧或内部 worker 的统一通知重试入口。
func (e *Engine) ProcessPendingNotifications(ctx context.Context, limit int) error {
	if e == nil || e.dispatcher == nil || !e.opts.DispatcherEnabled {
		return nil
	}
	return e.dispatcher.ProcessPending(ctx, limit)
}

// RunDriftChecks 提供给业务侧或内部 worker 的统一漂移检测入口。
func (e *Engine) RunDriftChecks(ctx context.Context) error {
	if e == nil || e.driftRunner == nil || !e.opts.DriftRunnerEnabled {
		return nil
	}
	return e.driftRunner.RunOnce(ctx)
}

func (e *Engine) StopWorkers() {
	if e == nil || e.workers == nil {
		return
	}
	e.workers.Stop()
}

func (e *Engine) ensureWorkersStarted() {
	if e == nil {
		return
	}
	if e.workers == nil {
		e.workers = newWorkerManager(e)
	}
	e.workers.Start()
}

// shouldRunNotificationWorker 通过已注册策略自动判断是否需要启动通知后台任务。
// 业务侧默认零配置即可生效，如确实不想启用，可通过 options 显式关闭。
func (e *Engine) shouldRunNotificationWorker() bool {
	if e == nil || !e.opts.DispatcherEnabled || !e.opts.NotificationWorkerEnabled {
		return false
	}
	for _, policy := range e.policies {
		if len(policy.NotifyChannels) > 0 {
			return true
		}
	}
	return false
}

// shouldRunDriftWorker 仅在存在启用 drift 的策略时启动，避免无意义空转。
func (e *Engine) shouldRunDriftWorker() bool {
	if e == nil || !e.opts.DriftRunnerEnabled || !e.opts.DriftWorkerEnabled {
		return false
	}
	for _, policy := range e.policies {
		if policy.DriftCheckEnabled {
			return true
		}
	}
	return false
}

func (s *Session) isSuccess() bool {
	if len(s.Policy.SuccessHTTPStatuses) == 0 {
		return s.ResponseMeta.StatusCode >= 200 && s.ResponseMeta.StatusCode < 400
	}
	for _, code := range s.Policy.SuccessHTTPStatuses {
		if code == s.ResponseMeta.StatusCode {
			return true
		}
	}
	return false
}

func methodMatched(routeMethods, bindingMethods []string) bool {
	for _, routeMethod := range routeMethods {
		for _, bindingMethod := range bindingMethods {
			if routeMethod == bindingMethod {
				return true
			}
		}
	}
	return false
}

func (e *Engine) handleRuntimeError(err error) {
	if err == nil {
		return
	}
	if e.opts.Strict || e.opts.FailMode == FailModeClose {
		log.Warnf("changeguard runtime warning: %v", err)
		return
	}
	log.Warnf("changeguard runtime warning: %v", err)
}
