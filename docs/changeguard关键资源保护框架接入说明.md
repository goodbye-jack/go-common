# changeguard 关键资源保护框架接入说明

## 1. 目标

`changeguard` 是 `go-common` 提供的关键资源保护框架，当前正式协议只保留一套：

- `runtime`
- `providers`
- `scenarios`

目标是把关键资源保护抽成统一底座，而不是继续散落在各业务项目里重复实现。

它主要解决：

- 关键配置被误改后没有审计
- 敏感字段变更没有脱敏留痕
- 价格、密钥、短信、支付、微信配置变更后没有统一通知
- 关键资源缺少版本快照
- 配置被绕过前端直接改动后缺少 drift 检查

设计原则：

- 轻接入：业务侧尽量只配 YAML 和最小注册信息
- 厚能力：通知、版本、drift、worker、持久化由 `go-common` 统一承担
- 强容错：默认 fail-open，不因为保护能力异常影响主业务启动
- 默认可用：启用后自动启动 worker，业务侧不需要再写定时任务

## 2. 推荐接入模式

当前推荐的最终接入方式是：

- 业务项目在 `config.yaml` 中声明 `changeguard`
- 业务项目只提供 `AppSpec`
- `go-common` 读取 `runtime / providers / scenarios` 并自动完成 engine 初始化、sink/version/drift store 注册、notifier 注册、场景展开、路由绑定和 worker 启动

推荐入口：

```go
func BindChangeGuard(server *goodhttp.HTTPServer) {
	_ = changeguard.BindFromConfig(server, changeguard.AppSpec{
		ServiceName: config.GetAppName(),
		UserModel:   &systemModel.SysUser{},
		Fetchers: map[string]any{
			"payment_config": tradeBiz.GetPaymentCenterConfig,
			"sms_config":     tradeBiz.GetSMSConfig,
			"wechat_config":  accountBiz.GetWechatConfig,
		},
		Models: map[string]any{
			"membership_plan": &membershipModel.Plan{},
			"token_package":   &tradeModel.TokenPackage{},
			"storage_package": &tradeModel.StoragePackage{},
		},
	})
}
```

说明：

- `Fetchers` 允许直接传 `func(ctx) (*T, error)`，`go-common` 会自动适配
- `Models` 只负责告诉框架某个场景底层对应哪个 GORM 模型
- `CustomProviders` 和 `RecipientResolvers` 只用于高级扩展
- 业务侧不再直接声明 `policies/resources/bindings`

## 3. BindFromConfig 做了什么

`BindFromConfig(server, spec)` 当前会自动完成：

- 读取 `changeguard` 配置
- 构建 `EngineOptions`
- 创建 `Engine`
- 注册 `GormSink`
- 注册 `GormVersionStore`
- 注册 `GormDriftReportStore`
- 注册短信 notifier
- 注册收件人 profile resolver
- 注册单例 fetcher
- 注册 custom provider
- 根据 `scenarios` 展开 policy/resource/binding
- 绑定到 `HTTPServer`
- 自动启动通知 worker 和 drift worker

如果 `changeguard` 配置不存在，或者配置解析失败：

- 默认只告警
- 不影响业务正常启动

## 4. YAML 结构

当前 `changeguard` 顶层结构固定为：

```yaml
changeguard:
  enabled: true
  runtime: {}
  providers: {}
  scenarios: []
```

### 4.1 `enabled`

- 是否启用 `changeguard`
- 未配置或为 `false` 时，框架直接跳过

### 4.2 `runtime`

用于配置框架级默认行为。

支持字段：

- `strict`
- `fail_mode`
- `default_risk_level`
- `default_notify_channels`
- `default_max_diff_changes`
- `default_summary_field_limit`
- `default_summary_value_limit`
- `default_version_enabled`
- `default_rollback_enabled`
- `default_retention_days`
- `default_drift_enabled`
- `dispatcher_enabled`
- `drift_runner_enabled`
- `retention_enabled`
- `allow_no_sink`
- `allow_no_version_store`
- `allow_no_notifier`
- `request_id_header`
- `auto_start_workers`
- `notification_worker_enabled`
- `drift_worker_enabled`
- `notification_poll_interval`
- `drift_poll_interval`

推荐最小配置：

```yaml
runtime:
  auto_start_workers: true
  notification_worker_enabled: true
  drift_worker_enabled: true
  notification_poll_interval: "1m"
  drift_poll_interval: "10m"
  fail_mode: "fail_open"
  default_risk_level: "medium"
  default_notify_channels: ["sms"]
  default_version_enabled: true
  default_rollback_enabled: true
  default_drift_enabled: true
```

### 4.3 `providers`

用于声明通知通道和收件人来源。

当前支持：

- `providers.sms`
- `providers.recipient_profiles`

`providers.sms` 支持字段：

- `enabled`
- `config_prefix`

`providers.recipient_profiles` 当前支持模式：

- `admin_users`
- `fixed_phones`
- `fixed_emails`
- `custom`

### 4.4 `scenarios`

`scenarios` 是当前唯一业务主入口。

每个 `scenario` 会自动展开成内部：

- 1 个 `PolicyProfile`
- 1 个 `ResourceProfile`
- N 个 `RouteBinding`

当前支持字段：

- `key`
- `enabled`
- `kind`
- `name`
- `source`
- `routes`
- `notify`
- `overrides`

## 5. `kind` 列表

当前内置：

- `critical_config`
- `price_resource`
- `status_toggle_resource`
- `secret_config`
- `custom`

说明：

- `critical_config` 适合支付、短信、微信等关键配置
- `price_resource` 适合会员套餐、Token 包、容量包等价格资源
- `status_toggle_resource` 适合启停/开关类资源
- `secret_config` 适合密钥/令牌类资源
- `custom` 作为高级兜底

## 6. 最小接入示例

### 6.1 业务侧代码

```go
func BindChangeGuard(server *goodhttp.HTTPServer) {
	_ = changeguard.BindFromConfig(server, changeguard.AppSpec{
		ServiceName: config.GetAppName(),
		UserModel:   &systemModel.SysUser{},
		Fetchers: map[string]any{
			"payment_config": tradeBiz.GetPaymentCenterConfig,
		},
		Models: map[string]any{
			"membership_plan": &membershipModel.Plan{},
		},
	})
}
```

### 6.2 业务侧 YAML

```yaml
notifications:
  sms:
    enabled: true
    provider: "luosimao"
    default_sign: "琢跃数智"
    default_template: "{{content}}"
    providers:
      mock:
        enabled: false
      luosimao:
        enabled: true
        endpoint: "https://sms-api.luosimao.com/v1/send.json"
        api_key: "your-api-key"
        sign_name: "琢跃数智"

changeguard:
  enabled: true
  runtime:
    auto_start_workers: true
    notification_poll_interval: "1m"
    drift_poll_interval: "10m"
  providers:
    sms:
      enabled: true
      config_prefix: "notifications.sms"
    recipient_profiles:
      default_admins:
        mode: "admin_users"
        user_type: "admin"
        status: "normal"
        phone_field: "phone"
        tenant_field: "tenant_code"
  scenarios:
    - key: "payment_config_guard"
      kind: "critical_config"
      name: "支付配置"
      source:
        type: "singleton"
        fetcher: "payment_config"
      notify:
        recipient_profile: "default_admins"
      routes:
        - path: "/api/v1/admin/config/payment/save"
          action: "save"
```

## 7. 自动 worker 机制

启用 `BindFromConfig` 后，默认行为是：

- 存在通知策略时，自动启动通知 worker
- 存在 drift 策略时，自动启动 drift worker

业务侧不需要自己写：

- `ProcessPendingNotifications(ctx, limit)` 的定时任务
- `RunDriftChecks(ctx)` 的定时任务

这部分调度逻辑已经由 `go-common` 承担。

## 8. 容错原则

`changeguard` 当前遵守“增强能力不拖垮主业务”的原则：

- `changeguard` 节点不存在：直接跳过
- 配置解析失败：告警并跳过
- fetcher 签名不支持：告警并跳过该 fetcher
- model 未注册：告警并跳过该 scenario
- kind 与 source.type 不兼容：告警并跳过该 scenario
- recipient_profile 无效：告警并跳过该 scenario
- route 未匹配：告警但不阻断启动

## 9. 后续扩展建议

建议继续保持下面这条边界：

- `go-common` 负责框架骨架、通知通道、worker、存储、容错
- 业务项目只负责提供 YAML、fetcher、model、用户模型

不建议重新回到：

- 业务项目自己手写 `RegisterPolicies`
- 业务项目自己手写 `RegisterResources`
- 业务项目自己手写 `RegisterBindings`
- 业务项目自己写通知重试任务
- 业务项目自己拼短信发送能力

这样才能真正保持“轻接入、厚能力、强容错”的目标不被破坏。
