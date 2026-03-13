# 工作流业务接入指南

## 1. 文档目标

这份文档用于定义业务系统接入 `go-common/workflow` 的标准方式。

目标架构是固定的：

- `go-common/workflow` 提供可复用的工作流通用能力
- 各业务系统保留自己的业务表、业务接口和业务页面
- Flowable 表单模型只作为节点字段参考，不承担业务页面渲染
- 业务前端始终使用自己的页面，只消费工作流接口返回的数据

这样可以保证工作流能力和业务存储、业务页面完全解耦，后续别的业务系统也能直接复用。

## 2. 职责边界

### 2.1 属于 `go-common/workflow` 的能力

- 当前工作流用户解析
- 基于 LDAP 的当前用户、上级、部门、岗位查询
- 发起流程
- 我的待办
- 我的已办
- 任务上下文
- 节点表单引用解析
- 流程进度视图
- 流程进度时间线
- 按流程实例查询进度
- 按业务单号查询进度
- BPMN XML 获取

### 2.2 属于业务系统自己的能力

- 业务表设计
- 业务数据入库
- 业务列表页
- 业务创建页
- 业务处理页
- 业务字段校验
- 附件存储
- 业务状态定义

## 3. 标准业务流转方式

推荐业务系统统一采用下面这条主链路：

1. 业务用户登录业务系统
2. 业务系统把当前登录人解析成工作流用户上下文
3. 业务系统先把业务数据写入自己的业务表
4. 业务系统使用 `bizId`、`bizType`、`title` 发起 Flowable 流程
5. 业务系统展示：
   - 我的申请：来自业务表
   - 我的待办：来自工作流接口
   - 我的已办：来自工作流接口
6. 业务系统通过任务上下文接口加载任务详情
7. 业务系统通过进度接口加载流程图和时间线
8. 业务系统调用工作流完成任务接口
9. 业务系统按自己的规则更新业务状态

## 4. 发起流程时必须带的字段

每个业务系统发起流程时，建议最少传入以下字段：

- `bizId`
- `bizType`
- `title`
- `payloadRef`

推荐同时传入：

- `tenantId`
- `systemCode`
- `callbackUrl`
- `callbackEvents`
- 业务判断变量，例如 `needExpert`

当前 `go-common/workflow` 已经支持在 `StartProcessRequest` 中自动补齐：

- `bizId`
- `bizType`
- `title`

也就是说，只要业务系统在发起请求中带了这些值，后续待办、任务上下文、进度接口都能稳定读到。

## 5. 标准接口使用方式

### 5.1 发起流程

后端调用：

- `POST /api/process/start`

业务系统职责：

- 先落业务表
- 生成稳定的 `bizId`
- 把业务标题传入流程
- 保存返回的 `processInstanceId`

### 5.2 我的申请

“我的申请”不是工作流自带列表，而是业务系统自己的列表。

推荐数据来源：

- 业务表按当前发起人查询

然后对每一条业务记录补充工作流进度：

- `GET /api/biz/{bizId}/progress-view`
- `GET /api/biz/{bizId}/progress-timeline`

### 5.3 我的待办

后端调用：

- `GET /api/me/tasks/todo`

适用场景：

- 查询当前登录人的待办任务
- 展示当前节点
- 展示当前办理人
- 展示当前候选人、候选组

### 5.4 我的已办

后端调用：

- `GET /api/me/tasks/done`

适用场景：

- 查询当前登录人已处理过的任务
- 展示完成时间
- 展示已办关联的流程进度

### 5.5 任务详情

后端调用：

- `GET /api/tasks/{taskId}/context`

适用场景：

- 获取任务本身信息
- 获取业务标识
- 获取流程变量
- 获取节点表单引用

### 5.6 流程进度

后端调用：

- `GET /api/process-instances/{id}/progress-view`
- `GET /api/process-instances/{id}/progress-timeline`
- `GET /api/biz/{bizId}/progress-view`
- `GET /api/biz/{bizId}/progress-timeline`
- `GET /api/process/instance/{id}/definition-xml`

适用场景：

- 页面内嵌 BPMN 流程图
- 展示当前节点
- 展示当前办理人
- 展示当前候选人和候选组
- 展示从开始到结束的完整流转时间线

### 5.7 处理任务

后端调用：

- `POST /api/tasks/{taskId}/complete`

业务系统职责：

- 提交业务页面上的字段
- 把业务判断结果映射成流程变量
- 按业务规则更新业务表状态

## 6. 当前进度接口中人员字段的语义

当前标准进度接口已经支持以下人员字段：

- `summary.currentAssignees`
- `summary.currentCandidateUsers`
- `summary.currentCandidateGroups`
- `steps[].assignee`
- `steps[].owner`
- `steps[].candidateUsers`
- `steps[].candidateGroups`
- `timeline.items[].assignee`
- `timeline.items[].owner`
- `timeline.items[].candidateUsers`
- `timeline.items[].candidateGroups`

前端推荐展示规则：

- 优先显示 `assignee`
- 如果没有签收人，则显示候选人和候选组，并标识为“待认领”
- 只有真正的系统节点才显示“系统”

## 7. 表单接入原则

Flowable 表单模型只作为节点参考，不作为业务页面渲染来源。

推荐原则如下：

- 节点有 Flowable 表单模型时，业务页面可以把它当作字段参考
- 节点没有 Flowable 表单模型时，业务页面也必须正常工作
- 不依赖 Flowable 官方表单渲染器去做业务前端

这也是当前整体架构已经确认的方向。

## 8. 业务服务中最小注册方式

业务服务只需要把工作流模块注册到自己的 HTTP 服务里：

```go
module, err := workflowapi.NewDefaultModuleFromConfig()
if err != nil {
	panic(err)
}
module.Register(server)
```

最少需要的配置组：

- `workflow.flowable.*`
- `workflow.api.*`
- `workflow.context.*`
- LDAP 相关配置

如果开启表单引用解析，并且 Flowable 表单元数据不在业务库里，还需要配置：

- `workflow.formref.db_instance`

## 9. 推荐的业务系统契约

为了让工作流和业务系统长期稳定联动，建议所有业务系统都遵守下面这个契约：

- 业务表必须有稳定的 `biz_id`
- 发起流程时始终使用同一个 `biz_id`
- 业务表里保存 `process_instance_id`
- 业务详情页能根据 `biz_id` 查到业务记录

这样可以得到两条稳定的查询路径：

- 业务系统通过 `bizId` 查工作流
- 工作流通过 `payloadRef` 或 `bizId` 反查业务

## 10. 标准接入检查项

新接一个业务系统时，最少检查以下内容：

- 业务系统已经有稳定登录态
- 请求里能解析出 `userId`
- 请求里能解析出 `tenantId`
- 业务表已经建好
- 业务表里有 `bizId`
- 发起流程时携带 `bizId`、`bizType`、`title`
- 待办接口对当前人可用
- 已办接口对当前人可用
- 任务上下文能返回业务标识
- 进度视图能返回当前节点和人员
- 时间线能返回真实流转过程
- BPMN XML 能在前端正确渲染
- 完成任务后流程能继续往下流转

## 11. 后续复用建议

以后新的业务系统接入时，建议直接复用 `go-common/workflow`，只重写下面几类内容：

- 业务创建接口
- 业务列表接口
- 业务详情接口
- 任务处理页字段

不建议每个系统都重复实现下面这些能力：

- 工作流进度接口
- 待办和已办接口
- 表单引用解析
- LDAP 上级和部门查询
- BPMN XML 获取

这正是当前这套架构最大的复用价值。

## 12. 可直接发给业务组的接入要求

如果这份文档要直接发给其他业务组，建议你把下面这段作为统一要求一起发出：

### 12.1 必须遵守的接入规则

- 业务系统必须保留自己的业务表
- 业务系统必须保留自己的业务页面
- `bizId` 必须是业务与流程之间的主关联键
- 发起流程时必须传 `bizId`、`bizType`、`title`
- “我的申请”必须查业务表，不能查工作流已办代替
- Flowable 表单只做字段参考，不做业务页面渲染

### 12.2 业务组必须提供的内容

- 一张业务表
- 一个业务创建接口
- 一个“我的申请”接口
- 一个任务处理页
- 一个业务详情页
- 一套业务字段到流程变量的映射规则

## 13. 可直接复用的配置模板

下面这份配置模板可以直接给业务组改值后使用：

```yaml
workflow:
  flowable:
    base_url: "http://localhost:8087/flowable-rest/service"
    username: "rest-admin"
    password: "test"
    timeout_seconds: 15
    group_prefix: ""
    role_prefix: "role_"
  formref:
    db_instance: "flowable_meta"

ldap_addr: "ldap://127.0.0.1:8389"
ldap_bind_dn: "cn=admin,dc=example,dc=com"
ldap_bind_password: "password"
ldap_base_dn: "dc=example,dc=com"
ldap_people_ou: "ou=people"
ldap_department_ou: "ou=departments"
ldap_position_ou: "ou=positions"
ldap_use_tls: false
```

说明：

- `workflow.flowable.*` 是 Flowable REST 连接
- `workflow.formref.db_instance` 只在启用表单引用解析时需要
- LDAP 配置用于当前人、上级、部门能力

## 14. 可直接复用的发起流程模板

业务组发起流程时，可以直接参考下面这份请求结构：

```json
{
  "processDefinitionKey": "artifact_appraise",
  "bizId": "APPRAISE-20260311-0001",
  "bizType": "artifact_appraise",
  "title": "馆藏文物鉴定申请单-0001",
  "name": "馆藏文物鉴定申请单-0001",
  "variables": {
    "payloadRef": "APPRAISE-20260311-0001",
    "tenantId": "test-demo",
    "systemCode": "sysA",
    "callbackUrl": "http://localhost:9080/flowable/callback",
    "callbackEvents": "PROCESS_STARTED,NODE_STARTED,NODE_ENDED,PROCESS_ENDED",
    "needExpert": true
  }
}
```

强制要求：

- `bizId` 不能为空
- `title` 不能为空
- `payloadRef` 建议与 `bizId` 保持一致

## 15. 可直接复用的页面职责模板

为了避免业务组做乱，建议统一按下面的页面职责拆分：

### 15.1 创建页

- 只负责填写业务字段
- 只负责调用业务创建和发起接口

### 15.2 我的申请页

- 只查业务表列表
- 叠加流程进度
- 支持查看流程图和时间线

### 15.3 我的待办页

- 只展示当前登录人的待办
- 支持进入任务处理页

### 15.4 我的已办页

- 只展示当前登录人的已办
- 支持查看流程进度

### 15.5 任务处理页

- 展示任务上下文
- 展示业务数据
- 提交处理结果

## 16. 可直接复用的联调顺序

建议业务组严格按下面顺序联调：

1. 先验证发起流程
2. 再验证“我的申请”
3. 再验证“我的待办”
4. 再验证任务详情
5. 再验证任务处理
6. 再验证“我的已办”
7. 最后验证流程图和时间线

这样排查问题的成本最低。
- todo list works for current user
- done list works for current user
- task context returns business identity
- progress view returns current node and people
- progress timeline returns real handling sequence
- definition XML renders correctly on frontend
- complete-task request can drive real process flow

## 11. Recommended Reuse Strategy

For future business systems, reuse `go-common/workflow` directly and only rewrite:

- business create API
- business list API
- business detail API
- task handling page fields

Do not rewrite:

- workflow progress APIs
- todo and done APIs
- form reference parsing
- LDAP manager and department lookup
- BPMN definition XML access

That is the main decoupling value of the current architecture.
