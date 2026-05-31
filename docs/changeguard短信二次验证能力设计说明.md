# changeguard 短信二次验证能力设计说明

## 1. 设计目标

这份设计只解决一个明确问题：

- 某些关键资源修改操作，在真正提交前，必须再做一次短信验证码确认

最终目标必须同时满足：

- 业务侧继续保持 `changeguard v2` 的极简接入方式
- 不新增业务专用验证码接口
- 不要求业务 handler 改请求结构
- 不要求业务侧再写二次验证胶水代码
- 继续沿用当前 `changeguard` 的 `runtime / providers / scenarios` 配置结构
- 这是在当前 `v2` 协议上做增量扩展，不再引入 `v3`

## 2. 最终结论

最终采用下面这套方案：

- 业务侧仍然只配 `config.yaml`
- 二次验证只作为 `changeguard v2` 的增强能力加入
- 只增加两类配置：
  - `providers.second_factor_sms`
  - `scenarios[].overrides.second_factor_*`
- 同一套二次验证能力支持两种确认方式：
  - `sms_code`：短信验证码输入
  - `sms_reply`：短信回复确认
- 也支持组合模式：
  - `sms_code_or_reply`
- 前端不新增业务专用“先发验证码、再提交”的接口
- 直接复用原来的“提交接口”
- 当模式为 `sms_code` 或 `sms_code_or_reply` 时：
  - 第一次提交时，`changeguard` 中间件拦截、发送验证码、返回“需要二次验证”
  - 第二次提交时，前端仍然调用同一个提交接口，只是在 Header 中带上 `challenge_id` 和 `sms_code`
- 当模式为 `sms_reply` 时：
  - 第一次提交时，`changeguard` 中间件拦截、发送“短信回复确认”消息
  - 用户直接回复短信，例如 `1#A7` / `0#A7`
  - 前端页面不需要输入验证码
  - 用户确认完成后，再点击一次原来的提交按钮即可

这样做的结果是：

- 业务侧代码仍然只保留 `BindChangeGuard(...)`
- 业务配置仍然只围绕 `scenario` 写
- 二次验证逻辑全部收口在 `go-common/changeguard`
- 短信验证码输入路径只需要前端统一请求层改一次
- 短信回复确认路径可以做到业务前端零改造

## 3. 为什么这样设计

### 3.1 不新增业务专用二次确认接口

如果每个业务项目都自己再写：

- `发送验证码接口`
- `校验验证码接口`
- `短信回复确认接口`
- `提交前校验接口`

最后一定会回到：

- 业务侧代码散
- 前后端协议不统一
- 每个项目都要重复造胶水

这和 `changeguard` 现在已经达成的“框架统一保护、业务只配场景”目标是冲突的。

所以最终必须由 `changeguard` 自己把：

- 发码
- 验码
- 发回复确认短信
- 接收上行短信回复
- 提交前拦截
- 提交成功后消费 challenge

这一整条链路做完。

### 3.2 不复用业务项目现有 C 端验证码能力

`pano-backend` 现在已有一套短信验证码能力，但它的定位是：

- C 端登录
- 注册
- 换绑手机号
- 重置密码

它带有明显的业务场景语义，不适合直接当成 `go-common/changeguard` 的通用二次验证底座。

所以这里的正确做法是：

- `changeguard` 自己维护二次验证 challenge 和验证码状态
- 短信发送层只复用 `go-common/notify/sms` 的“发文本短信”能力
- 不耦合 `pano-backend` 现有验证码 Redis key、场景枚举、业务频控策略

### 3.3 二次验证必须是 fail-closed

当前 `changeguard` 大部分能力遵守“增强能力不拖垮业务”的原则。

但是“提交前二次验证”不一样。

如果某个场景明确开启了短信二次验证，那么：

- 没拿到验证码
- 验证码校验失败
- 当前操作人没有可用手机号
- challenge 过期
- Redis 不可用
- 短信发送失败

这些情况都必须阻断本次提交。

否则这个保护能力在安全上就是无效的。

所以这里必须单独定义：

- 普通 `changeguard` 仍然保持当前 `fail_open` 思路
- 但 `second_factor_enabled=true` 的场景，二次验证链路固定使用 `fail_closed`

## 4. 业务侧最终接入方式

业务侧代码不变，仍然保持现在这层薄入口：

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

业务侧不需要新增：

- 发送验证码 handler
- 校验验证码 handler
- 前置校验 middleware
- challenge 持久化代码

## 5. 最终配置协议

## 5.1 最小示例

```yaml
changeguard:
  enabled: true
  providers:
    sms:
      enabled: true
      config_prefix: "notifications.sms"
    second_factor_sms:
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
        - path: "/api/v1/admin/config/payment/publish"
          action: "publish"
      overrides:
        second_factor_enabled: true
        second_factor_mode: "sms_code_or_reply"
        second_factor_on_actions: ["save", "publish"]
```

上面这份配置已经表达了完整意图：

- `payment_config_guard` 继续是一个普通 `scenario`
- 只有这个场景开启了短信二次验证
- 这个场景同时支持“验证码输入”和“短信回复确认”
- 只有 `save / publish` 这些动作需要验证码
- 业务侧没有新增任何额外代码

## 5.2 新增配置一：`providers.second_factor_sms`

建议在 `providers` 下新增：

```yaml
providers:
  second_factor_sms:
    enabled: true
    config_prefix: "notifications.sms"
    template: "【关键资源保护】您正在执行{{resource_name}}{{action_name}}操作，验证码：{{code}}，{{ttl_minutes}}分钟内有效。"
    reply_template: "【关键资源保护】您正在执行{{resource_name}}{{action_name}}操作，摘要：{{summary_text}}。同意请回复 1#{{reply_token}}，拒绝请回复 0#{{reply_token}}。"
    code_ttl: "5m"
    resend_interval: "60s"
    verified_ttl: "10m"
    max_verify_attempts: 5
    challenge_header: "X-ChangeGuard-Challenge-Id"
    code_header: "X-ChangeGuard-Verify-Code"
    reply_callback_path: "/internal/changeguard/second-factor/sms-reply"
    reply_token_size: 2
    reply_approve_prefix: "1#"
    reply_reject_prefix: "0#"
    user_id_field: "id"
    phone_field: "phone"
    tenant_field: "tenant_code"
    user_type_field: "user_type"
    status_field: "status"
    required_user_type: "admin"
    required_status: "normal"
```

字段中文含义：

- `providers.second_factor_sms`
  - `changeguard` 短信二次验证提供方配置
  - 这里只负责“提交前验证”的验证码能力
  - 不等同于普通变更通知短信

- `enabled`
  - 是否启用短信二次验证提供方
  - 为 `false` 时，所有开启二次验证的场景都无法正常工作
  - 如果某个场景配置了 `second_factor_enabled=true`，但这里未启用，应直接阻断提交并告警

- `config_prefix`
  - 从哪一组短信发送配置读取供应商信息
  - 默认建议复用 `notifications.sms`
  - 含义是二次验证短信也走当前通用短信通道发出

- `template`
  - 二次验证短信模板
  - 用于拼接最终短信内容
  - 推荐由 `go-common` 直接渲染变量，不再依赖业务项目模板系统
  - 建议支持的模板变量：
    - `{{code}}`
    - `{{resource_name}}`
    - `{{action_name}}`
    - `{{ttl_minutes}}`
    - `{{operator_name}}`

- `reply_template`
  - 短信回复确认模式使用的短信模板
  - 短信里直接告诉用户本次变更的大致内容，并要求按约定格式回复
  - 推荐格式：
    - 同意：`1#{{reply_token}}`
    - 拒绝：`0#{{reply_token}}`
  - 建议支持的模板变量：
    - `{{resource_name}}`
    - `{{action_name}}`
    - `{{summary_text}}`
    - `{{reply_token}}`
    - `{{operator_name}}`

- `code_ttl`
  - 验证码有效期
  - 超过这个时间后，验证码失效，必须重新发码
  - 使用 Go duration 格式，例如 `5m`

- `resend_interval`
  - 同一个 challenge 的最短重发间隔
  - 用于避免重复点击导致连续发码
  - 使用 Go duration 格式，例如 `60s`

- `verified_ttl`
  - 验证通过后的临时可信窗口
  - 主要用于“验证码已通过，但业务提交失败后再次重试”的场景
  - 在这个时间窗口内，前端可带同一个 `challenge_id` 继续重试相同请求
  - 使用 Go duration 格式，例如 `10m`

- `max_verify_attempts`
  - 单个 challenge 最多允许输错验证码多少次
  - 超过后 challenge 直接作废，必须重新发码

- `challenge_header`
  - 前端第二次提交时，用哪个 Header 传 challenge ID
  - 推荐默认值：`X-ChangeGuard-Challenge-Id`

- `code_header`
  - 前端第二次提交时，用哪个 Header 传短信验证码
  - 推荐默认值：`X-ChangeGuard-Verify-Code`

- `reply_callback_path`
  - 短信供应商上行回复推送到本系统的回调路径
  - 这个路径由 `go-common/changeguard` 提供统一接收 handler
  - 业务项目只需要把完整 URL 配到短信供应商后台
  - 例如 Luosimao 控制台中的“上行回复推送”

- `reply_token_size`
  - 回复短标识长度
  - 例如 `A7` 的长度是 `2`
  - 建议保持较短，方便用户直接回复

- `reply_approve_prefix`
  - 短信回复“同意”的前缀
  - 推荐默认值：`1#`

- `reply_reject_prefix`
  - 短信回复“拒绝”的前缀
  - 推荐默认值：`0#`

- `user_id_field`
  - 当前操作人用户模型里的主键字段名
  - 默认建议 `id`

- `phone_field`
  - 当前操作人用户模型里的手机号字段名
  - 默认建议 `phone`

- `tenant_field`
  - 当前操作人用户模型里的租户字段名
  - 默认建议 `tenant_code`

- `user_type_field`
  - 当前操作人用户模型里的用户类型字段名
  - 默认建议 `user_type`

- `status_field`
  - 当前操作人用户模型里的状态字段名
  - 默认建议 `status`

- `required_user_type`
  - 允许通过二次验证的用户类型
  - 默认建议 `admin`

- `required_status`
  - 允许通过二次验证的用户状态
  - 默认建议 `normal`

### 5.2.1 业务侧最小推荐写法

绝大多数项目只需要写：

```yaml
providers:
  second_factor_sms:
    enabled: true
    config_prefix: "notifications.sms"
```

其余字段都走默认值。

这才符合“业务侧简单易配”的目标。

## 5.3 新增配置二：`scenarios[].overrides.second_factor_*`

建议在 `ScenarioOverrideConfig` 里新增：

```yaml
overrides:
  second_factor_enabled: true
  second_factor_mode: "sms_code_or_reply"
  second_factor_on_actions: ["save", "publish"]
```

字段中文含义：

- `second_factor_enabled`
  - 当前场景是否开启短信二次验证
  - 为 `true` 时，这个场景从“审计/通知型保护”升级成“提交前拦截型保护”

- `second_factor_mode`
  - 当前场景使用哪种二次确认方式
  - 当前支持：
    - `sms_code`
    - `sms_reply`
    - `sms_code_or_reply`
  - 中文含义：
    - `sms_code`：短信验证码输入
    - `sms_reply`：短信回复确认
    - `sms_code_or_reply`：两种方式都可用，任意一种完成即可

- `second_factor_on_actions`
  - 当前场景中，哪些动作需要短信二次验证
  - 例如：
    - `save`
    - `publish`
    - `enable`
    - `disable`
    - `toggle`
    - `rollback`
  - 如果不写，建议默认表示“该场景下所有已绑定动作都需要二次验证”

这三个字段已经足够表达当前版本需要的能力。

这里仍然不建议现在就对外开放：

- `email_second_factor`
- `totp_second_factor`
原因很简单：

- 当前目标只有“短信二次验证”
- 过早抽象会把配置重新做复杂
- 这和本轮“先把短信方案做扎实”的目标冲突

所以本次设计仍然保持收敛：

- 外部配置只支持短信二次验证
- 但允许短信二次验证内部有两种确认模式
- 内部实现继续使用 `PolicyProfile.RequireSecondFactor` / `SecondFactorMode`

## 6. 中间件执行流程

## 6.1 第一次提交

当请求命中某个已开启 `second_factor_enabled=true` 的 `scenario` 时：

1. `changeguard` 中间件在业务 handler 前执行
2. 判断当前 `binding.action` 是否在 `second_factor_on_actions` 范围内
3. 解析当前操作人信息
4. 基于 `AppSpec.UserModel` 查询当前操作人的手机号
5. 读取请求原始 body，计算请求摘要 `request_digest`
6. 创建或复用一个 challenge
7. 按 `second_factor_mode` 发送对应短信：
   - `sms_code`：发送验证码短信
   - `sms_reply`：发送回复确认短信
   - `sms_code_or_reply`：建议发送一条组合说明短信，或优先发送可同时覆盖两种确认方式的短信
8. 直接拦截本次请求，不进入业务 handler
9. 返回“需要短信二次验证”的响应

这一步的关键点是：

- 真正的数据修改还没有发生
- handler 根本不会执行
- 所以不会出现“先改成功了，再补验证码”的问题

## 6.2 第二次提交：`sms_code`

前端拿到 challenge 后，弹出验证码输入框。

用户输入短信验证码后，前端仍然调用原来的提交接口，只是在 Header 中加：

- `X-ChangeGuard-Challenge-Id`
- `X-ChangeGuard-Verify-Code`

然后 `changeguard` 中间件执行：

1. 读取 challenge ID 和验证码
2. 查 challenge 是否存在、是否过期
3. 校验 challenge 是否属于当前用户
4. 校验当前请求的 `request_digest` 是否和第一次发码时一致
5. 校验短信验证码是否正确
6. 校验成功后放行业务 handler
7. handler 执行完成后，再根据响应结果决定是否消费 challenge

## 6.3 短信回复确认：`sms_reply`

当模式是 `sms_reply` 时，不走验证码输入，而是走上行短信确认。

用户收到短信后，短信内容里会带一个很短的回复标识，例如：

- 同意：`1#A7`
- 拒绝：`0#A7`

这里的 `A7` 是 `changeguard` 为当前 challenge 生成的短标识。

服务端处理流程：

1. `changeguard` 创建 challenge 时，同时生成 `reply_token`
2. 把 `reply_token` 写入短信内容
3. 用户回复 `1#A7` 或 `0#A7`
4. 短信供应商把上行内容推送到 `reply_callback_path`
5. `changeguard` 统一回调接口解析：
   - 手机号
   - 回复内容
6. 根据 `reply_token` 找到对应 challenge
7. 如果回复是 `1#A7`，标记为 `approved`
8. 如果回复是 `0#A7`，标记为 `rejected`
9. 用户再次点击原提交按钮时：
   - `approved` 则放行业务请求
   - `rejected` 则直接拦截并返回“已拒绝本次变更”

## 6.4 成功后 challenge 如何处理

challenge 不应该在“校验通过那一刻”立即销毁。

否则会出现一个糟糕问题：

- 验证码输入正确
- 但业务 handler 因普通表单校验失败返回 400
- 用户改一个无关字段再提交
- 却被迫重新收验证码

所以最终规则应当是：

- 验证码校验通过后，challenge 进入 `verified` 状态
- 如果本次业务请求最终成功，再消费 challenge
- 如果本次业务请求最终失败，则保留 `verified` 状态直到 `verified_ttl` 过期
- 如果是 `sms_reply` 模式，则保留 `approved` 状态直到 `verified_ttl` 过期
- 但 challenge 仍然必须绑定同一个 `request_digest`
- 只要请求内容变化，就必须重新发码

这样可以同时兼顾：

- 安全性
- 可用性
- 前端体验

## 7. 请求摘要 `request_digest` 规则

短信二次验证不能只校验“人”和“验证码”，还必须校验“这次提交的内容是不是同一份”。

否则会出现风险：

- 用户先针对 A 内容拿到了验证码
- 然后把请求体改成 B 内容
- 再直接复用原验证码提交

这是不能接受的。

所以 `changeguard` 必须在 challenge 中保存本次请求摘要。

建议摘要至少包含：

- 服务名
- 路径
- HTTP 方法
- 当前场景 key
- 当前 action
- 当前用户 ID
- 当前租户
- 原始请求 body
- query string

实现上建议：

- 对原始请求字节做 hash
- 不需要业务 handler 感知
- 只要第二次提交的请求内容发生变化，就判定 challenge 不可复用

## 8. 操作人手机号解析规则

短信二次验证发给谁，不应该走 `notify.recipient_profile`。

因为：

- `notify.recipient_profile` 是“谁来接收变更通知”
- 二次验证需要的是“当前操作人本人来确认”

所以这里必须单独定义：

- 二次验证短信固定发送给当前登录操作人
- 通过 `AppSpec.UserModel` 查询用户手机号
- 默认按下列条件查当前用户：
  - 用户主键等于当前 principal 的 `user_id`
  - 租户等于当前 principal 的 `tenant_code`
  - `user_type=admin`
  - `status=normal`

如果查不到可用手机号：

- 本次提交直接阻断
- 返回明确错误信息
- 不允许自动降级放行

## 9. HTTP 交互协议

## 9.1 第一次提交返回

建议返回 HTTP `428 Precondition Required`。

返回体建议固定为：

```json
{
  "code": "CHANGEGUARD_SECOND_FACTOR_REQUIRED",
  "message": "当前操作需要短信二次验证",
  "data": {
    "required": true,
    "mode": "sms_code_or_reply",
    "challenge_id": "cg_2fa_xxx",
    "expire_in_seconds": 300,
    "resend_after_seconds": 60,
    "masked_phone": "188****9210",
    "resource_name": "支付配置",
    "action": "save",
    "reply_token_hint": "A7"
  }
}
```

字段中文含义：

- `code`
  - 前端识别“这是 changeguard 二次验证响应”的稳定错误码

- `message`
  - 直接给用户看的提示文案

- `required`
  - 是否必须二次验证

- `mode`
  - 当前验证方式
  - 当前可能是：
    - `sms_code`
    - `sms_reply`
    - `sms_code_or_reply`

- `challenge_id`
  - 当前二次验证挑战 ID
  - 前端第二次提交时必须带回

- `expire_in_seconds`
  - 验证码剩余有效期

- `resend_after_seconds`
  - 还要多久才能再次发码

- `masked_phone`
  - 脱敏后的目标手机号
  - 用于告诉操作人验证码发到哪里去了

- `resource_name`
  - 当前被保护资源中文名

- `action`
  - 当前被保护动作

- `reply_token_hint`
  - 当前短信回复确认使用的短标识提示
  - 仅在 `sms_reply` 或 `sms_code_or_reply` 模式下返回
  - 主要用于前端提示“你也可以直接回复短信 `1#A7` 完成确认”

## 9.2 第二次提交方式

前端再次请求同一个接口时：

- URL 不变
- body 不变
- 只额外带 Header

推荐 Header：

- `X-ChangeGuard-Challenge-Id: cg_2fa_xxx`
- `X-ChangeGuard-Verify-Code: 123456`

这样做的好处是：

- 不污染业务 JSON body
- 不需要每个 handler DTO 都新增验证码字段
- 对现有业务接口兼容性最好

## 9.3 短信回复确认方式

当模式是 `sms_reply` 或 `sms_code_or_reply` 时，短信内容中会明确提示：

- 同意：`1#A7`
- 拒绝：`0#A7`

这里的设计意图是：

- `1` / `0` 直接表达“同意 / 不同意”
- `#A7` 这样的短标识用于唯一定位 challenge
- 比只回复 `1` 或 `0` 更安全，避免多次待确认操作串单

推荐规则：

- 回复内容大小写不敏感
- 首尾空格自动忽略
- 只有完全匹配：
  - `1#<token>`
  - `0#<token>`
  才视为有效回复
- 其他内容统一记为无效回复，不改变 challenge 状态

## 9.4 Luosimao 上行回复回调约定

根据当前 Luosimao 控制台能力，已支持配置“上行回复推送”地址。

当前建议按 Luosimao 的回调形式接入：

- 使用 HTTP/HTTPS 推送
- 供应商以 GET 方式回调
- 至少带两个参数：
  - `mobile`
  - `message`

字段中文含义：

- `mobile`
  - 发送号码
  - 即回复短信的手机号

- `message`
  - 回复内容
  - 例如：
    - `1#A7`
    - `0#A7`

`changeguard` 统一回调 handler 的职责是：

1. 读取 `mobile`
2. 读取 `message`
3. 归一化回复内容
4. 解析出：
   - 回复动作：同意 / 拒绝
   - `reply_token`
5. 按 `reply_token + mobile` 定位 challenge
6. 更新 challenge 状态：
   - `approved`
   - `rejected`
7. 返回 200，避免供应商重复推送

这里建议 challenge 匹配时同时校验：

- `reply_token`
- 手机号
- challenge 未过期
- challenge 当前仍处于 `pending` 状态

这样可以最大程度降低误匹配风险。

## 10. Redis 存储设计

建议 `changeguard` 自己维护二次验证 Redis 数据，不复用业务验证码 key。

建议 key 结构：

- `changeguard:2fa:challenge:{challenge_id}`
  - 保存 challenge 主记录

challenge 主记录建议包含：

- `challenge_id`
- `service_name`
- `scenario_key`
- `resource_key`
- `action`
- `principal_user_id`
- `principal_tenant_code`
- `phone`
- `masked_phone`
- `request_digest`
- `sms_code`
- `reply_token`
- `approval_status`
- `verify_attempts`
- `verified`
- `verified_at`
- `expires_at`
- `last_sent_at`

说明：

- 原始验证码只保存在 Redis
- 不落数据库
- 日志里不打印原始验证码
- 短信日志里只保留脱敏手机号和挑战元数据
- `reply_token` 应尽量短，但必须在有效 challenge 范围内唯一
- `approval_status` 建议至少支持：
  - `pending`
  - `approved`
  - `rejected`

## 11. 与现有 changeguard 流程的关系

短信二次验证是“提交前拦截链路”，它发生在现有流程之前。

最终链路顺序应是：

1. 命中 `changeguard` route binding
2. 先执行二次验证检查
3. 通过后再执行现有的：
   - before 快照采集
   - `c.Next()`
   - after 快照采集
   - diff
   - event 落库
   - 通知分发
   - drift/version 逻辑

也就是说：

- 二次验证不是替代现有 `changeguard`
- 而是在现有保护链路最前面再加一道“提交门禁”

## 12. 需要补的代码点

本次实现建议只改 `go-common/changeguard`，业务侧尽量不动。

建议改动点：

- `config_binding_v2.go`
  - 新增 `providers.second_factor_sms` 配置结构
  - 新增 `ScenarioOverrideConfig.second_factor_enabled`
  - 新增 `ScenarioOverrideConfig.second_factor_mode`
  - 新增 `ScenarioOverrideConfig.second_factor_on_actions`

- `config_binding_v2_expand.go`
  - 把上述配置映射到 `PolicyProfile.RequireSecondFactor`
  - 映射到 `PolicyProfile.SecondFactorMode`
  - 映射到 `PolicyProfile.SecondFactorOnActions`

- `engine.go`
  - 在 `buildMiddleware(...)` 里，进入 `provider.Before(...)` 之前插入二次验证判断
  - 验证不通过时直接返回 428

- 新增二次验证 provider / service
  - challenge 创建
  - 发码
  - 验码
  - 生成 `reply_token`
  - 接收上行短信回复
  - 解析 `1#A7 / 0#A7`
  - challenge 状态更新

- 复用 `go-common/notify/sms`
  - 发送最终验证码文本
  - 发送回复确认文本

- 新增统一回调路由
  - 用于接收短信供应商的“上行回复推送”
  - Luosimao 当前已支持在控制台配置该推送地址
  - 第一阶段先按 Luosimao 的 `mobile + message` 回调格式接入

## 13. 前端对接规则

后台前端只需要统一处理一种响应：

- HTTP `428`
- `code=CHANGEGUARD_SECOND_FACTOR_REQUIRED`

前端行为建议固定为：

1. 拿到 428 响应
2. 读取 `challenge_id`、`masked_phone`、`expire_in_seconds`
3. 弹验证码输入框
4. 用户输入验证码
5. 按原请求再次提交
6. 在 Header 中带：
   - `X-ChangeGuard-Challenge-Id`
   - `X-ChangeGuard-Verify-Code`

这样前端也不需要为每个业务页面单独设计一套新流程。

如果场景模式是 `sms_reply`：

- 业务前端可以零改造
- 用户只需按短信内容回复，例如 `1#A7`
- 然后回到页面再次点击原按钮即可

如果场景模式是 `sms_code_or_reply`：

- 前端仍可按 `sms_code` 的统一弹窗流程工作
- 同时后端返回 `reply_token_hint`
- 用户也可以不输验证码，直接回复短信完成确认

## 14. 本方案的优点

- 业务侧配置仍然简单
- 业务侧代码不增加抽象和胶水
- 不新增业务专用验证码接口
- 不污染业务 handler DTO
- 不耦合已有 C 端验证码系统
- 同时兼容“前端统一弹验证码”和“短信直接回复确认”两种产品路径
- 可以真正形成“关键资源修改前门禁”
- 与现有 `changeguard v2` 协议完全兼容，只做增量扩展

## 15. 最终定版建议

本轮建议就按下面这版执行，不再发散：

- 只保留当前 `v2` 协议
- 二次验证只做短信方案
- 外部配置只新增：
  - `providers.second_factor_sms`
  - `overrides.second_factor_enabled`
  - `overrides.second_factor_mode`
  - `overrides.second_factor_on_actions`
- `sms_code` 走“同一路由二次提交 + Header 带 challenge/code”
- `sms_reply` 走“供应商上行回复回调 + 用户再次提交”
- 业务侧不写新接口、不写新中间件、不写新绑定代码

这版已经满足：

- 简单
- 直观
- 易配置
- 可实现
- 可发布

下一步就可以按照这份设计直接落代码。
