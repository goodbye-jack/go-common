# Flowable Callback 接口 3 分钟速读版

这份文档只回答一个问题：

- `POST /flowable/callback` 到底怎么用

如果你只想快速搞明白这个接口，不想先看完整平台文档，就看这份。

---

## 1. 先记住一句话

`POST /flowable/callback` 不是前端调用的接口，而是 Flowable 在流程运行过程中自动回调到业务系统的内部接口。

---

## 2. 谁调用它

正常调用方只有一个：

- Flowable

不是这些人调用：

- 前端页面
- 业务按钮
- 普通后端业务接口
- 测试人员日常手工点接口

所以你不要在前端写“调用 `flowable/callback`”这种代码。

---

## 3. 它什么时候会被调用

只要你在启动流程时配置了回调地址和回调事件，Flowable 就会在对应时机自动回调。

最常见的 4 类事件：

- `PROCESS_STARTED`
  - 流程刚启动
- `NODE_STARTED`
  - 某个节点刚进入运行态
- `NODE_ENDED`
  - 某个节点刚处理完成
- `PROCESS_ENDED`
  - 整个流程结束

---

## 4. 你真正要做的不是调它，而是“配置它”

你真正要做的是：

1. 业务保存成功
2. 调用 `POST /api/v1/workflow/process/start`
3. 在启动参数里传：
   - `callbackUrl`
   - `callbackEvents`

示例：

```json
{
  "processDefinitionKey": "asset_approval_process",
  "bizId": "BIZ-20260610-0001",
  "bizType": "asset_approval",
  "title": "资产审批申请示例",
  "variables": {
    "callbackUrl": "http://127.0.0.1:9080/flowable/callback",
    "callbackEvents": "PROCESS_STARTED,NODE_STARTED,NODE_ENDED,PROCESS_ENDED"
  }
}
```

字段含义：

- `callbackUrl`
  - Flowable 后面要把事件回调到哪个地址
- `callbackEvents`
  - 哪些事件发生时要回调

如果你没传这两个字段，通常就不会有这条回调链路。

---

## 5. 一条完整链路到底长什么样

### 第 1 步：业务系统启动流程

```http
POST /api/v1/workflow/process/start
```

请求体里传：

```json
{
  "processDefinitionKey": "asset_approval_process",
  "bizId": "BIZ-20260610-0001",
  "bizType": "asset_approval",
  "title": "资产审批申请示例",
  "variables": {
    "callbackUrl": "http://127.0.0.1:9080/flowable/callback",
    "callbackEvents": "PROCESS_STARTED,NODE_STARTED,NODE_ENDED,PROCESS_ENDED"
  }
}
```

### 第 2 步：Flowable 自动回调

```http
POST /flowable/callback
Content-Type: application/json
```

示例请求体：

```json
{
  "eventId": "evt-001",
  "eventType": "NODE_ENDED",
  "eventTime": "2026-06-10T10:00:00+08:00",
  "processInstanceId": "process-001",
  "processDefinitionId": "asset_approval_process:3:abc",
  "activityId": "task_review",
  "taskId": "task-001",
  "bizId": "BIZ-20260610-0001",
  "variables": {
    "bizType": "asset_approval",
    "title": "资产审批申请示例",
    "result": "APPROVED"
  }
}
```

### 第 3 步：业务系统返回成功

```json
{
  "data": {
    "accepted": true
  },
  "message": "success"
}
```

---

## 6. 请求体字段一眼看懂

- `eventId`
  - 这次回调事件的唯一标识
- `eventType`
  - 事件类型
- `eventTime`
  - 事件发生时间
- `processInstanceId`
  - 流程实例 ID
- `processDefinitionId`
  - 流程定义 ID
- `activityId`
  - 当前节点 ID，例如 `task_review`
- `taskId`
  - 当前任务 ID
- `bizId`
  - 业务主键
- `variables`
  - 这次事件发生时的流程变量快照

最常用的几个字段是：

- `eventType`
- `bizId`
- `activityId`
- `taskId`
- `variables`

---

## 7. 当前 `go-common` 收到回调后会做什么

当前标准实现只做工作流侧同步，不直接改你的业务主表。

它会做：

- 刷新流程摘要
- 同步运行中待办
- 同步已办
- 清理已完成节点的运行态数据

简单说就是：

- 保证“我的待办、我的已办、流程进度”这些工作流查询结果及时刷新

---

## 8. 它不会自动做什么

当前标准实现不会自动帮你做这些事：

- 不会自动更新业务主表状态
- 不会自动更新业务审核字段
- 不会自动写业务审核日志
- 不会自动发通知

所以不要误解成：

- “我只要配置了 `callbackUrl`，业务表状态就会自动变”

不是这样。

---

## 9. 那怎么用它更新业务表

答案是：

- 基于这个回调事件，做你自己的业务扩展处理

最常见做法：

- `PROCESS_STARTED`
  - 把 `processInstanceId` 回写到业务表
- `NODE_STARTED`
  - 更新当前处理节点
- `NODE_ENDED`
  - 更新本节点处理结果
  - 记录办理日志
- `PROCESS_ENDED`
  - 更新业务流程总状态
  - 记录完成时间

例如收到 `NODE_ENDED` 时，你可以这样理解：

```text
根据 bizId 找到业务单
根据 activityId 判断是哪一个节点完成
根据 variables.result 更新处理结果
根据 eventTime 更新最后处理时间
插入一条业务日志
```

---

## 10. 需要签名吗

看配置：

- 没配置 `workflow.api.callback_secret`
  - 不验签
- 配置了 `workflow.api.callback_secret`
  - 必须验签

需要的请求头：

- `X-Timestamp`
- `X-Nonce`
- `X-Signature`

签名原文：

```text
timestamp + "\n" + nonce + "\n" + body
```

算法：

```text
HMAC-SHA256
```

如果配了密钥但没带这些头，会返回：

```text
401 Unauthorized
```

---

## 11. 联调时怎么手工测

正式环境不应该手工调用，但联调时可以模拟一条：

```bash
curl -X POST http://127.0.0.1:9080/flowable/callback \
  -H "Content-Type: application/json" \
  -d '{
    "eventId":"evt-001",
    "eventType":"NODE_ENDED",
    "eventTime":"2026-06-10T10:00:00+08:00",
    "processInstanceId":"process-001",
    "processDefinitionId":"asset_approval_process:3:abc",
    "activityId":"task_review",
    "taskId":"task-001",
    "bizId":"BIZ-20260610-0001",
    "variables":{
      "bizType":"asset_approval",
      "result":"APPROVED"
    }
  }'
```

如果你配置了 `callback_secret`，记得补签名头，否则这条联调请求会失败。

---

## 12. 最后 30 秒总结

只记下面 4 条就够了：

1. `flowable/callback` 不是前端调的，是 Flowable 自动回调的。
2. 你要使用它，关键是启动流程时传 `callbackUrl` 和 `callbackEvents`。
3. 当前 `go-common` 默认只刷新工作流侧数据，不自动更新业务表。
4. 如果你要根据节点完成去更新业务表，要基于这个回调事件自己扩展。
