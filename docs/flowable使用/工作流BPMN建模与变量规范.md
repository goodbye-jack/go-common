# 工作流BPMN建模与变量规范

## 0. 文档入口

当前正式工作流文档统一放在：

```text
go-common/docs/flowable使用/
```

正式只保留 3 份：

1. `工作流平台接入总说明.md`
2. `工作流BPMN建模与变量规范.md`
3. `Flowable UI与REST本地启动说明.md`

### 0.1 本文适合谁看

- 流程设计人员
- 平台架构人员
- 负责流程接入的后端开发
- 需要和流程设计人员对齐规范的前端开发

### 0.2 本文解决什么问题

本文只解决“流程图应该怎么建”和“流程变量应该怎么约束”的问题，重点包括：

- Flowable 用户任务节点怎么配置
- 哪些办理人写法是平台允许的
- 哪些变量可以进入 BPMN
- 哪些变量属于高耦合、禁止长期使用

本文不负责说明：

- `flowable-ui` / `flowable-rest` 怎么部署
- 业务系统代码怎么注册工作流接口
- 前端应该按什么顺序调用工作流接口

这些内容分别去看：

- 平台部署和启动：`Flowable UI与REST本地启动说明.md`
- 业务系统接入和接口使用：`工作流平台接入总说明.md`

### 0.3 建议阅读顺序

如果你是第一次接入，建议先确认部署已经可用，再回来读本文：

1. 先看 `Flowable UI与REST本地启动说明.md`
2. 再看本文，完成 BPMN 建模和变量约束
3. 最后看 `工作流平台接入总说明.md`，完成业务系统接入

## 1. 文档定位

本文只回答两类问题：

1. Flowable BPMN 应该怎么建，哪些写法是平台标准
2. 流程变量应该怎么命名，哪些字段可以进 BPMN，哪些字段不应该直接耦合进 BPMN

这份文档是平台规范，不是某个业务项目的个性化说明。

---

## 2. 设计目标

平台规范必须满足以下目标：

1. 多个业务系统可以复用同一套 `go-common/workflow`
2. BPMN 不直接绑定某个业务系统的私有字段名
3. Flowable UI 建模人员、后端开发、前端开发使用同一套术语
4. 平台升级时不要求每个业务系统跟着改一遍 BPMN

一句话概括：

- 业务系统负责算“下一步给谁”
- 平台负责把结果标准化
- BPMN 只消费平台标准变量

---

## 3. 分层原则

### 3.1 平台层负责什么

- 标准工作流接口
- 目录服务
- 身份归一
- 自动分配输出标准化

### 3.2 业务层负责什么

- 业务规则
- 业务责任人计算
- 业务字段校验
- 业务表和业务状态

### 3.3 BPMN 层只负责什么

- 节点定义
- 路由判断
- 候选人/候选组/办理人表达

### 3.4 禁止倒挂

禁止以下做法：

- BPMN 直接长期依赖业务私有变量名
- 平台代码直接识别某个业务系统的私有责任人字段
- 前端默认值直接替代流程规则

---

## 4. 平台标准变量

以下变量定义为平台标准稳定变量：

- `starterId`
- `starterName`
- `startUserId`
- `tenantId`
- `systemCode`
- `managerId`
- `managerName`
- `departmentId`
- `departmentName`
- `nextAssignee`
- `nextCandidateUsers`
- `nextCandidateGroups`

其中最关键的是最后 3 个：

- `nextAssignee`
  - 下一节点唯一办理人
- `nextCandidateUsers`
  - 下一节点候选用户列表
- `nextCandidateGroups`
  - 下一节点候选组列表

---

## 5. 允许进入 BPMN 的写法

### 5.1 固定办理人

```xml
flowable:assignee="zhangsan"
```

适用场景：

- 永久固定技术节点
- 极少数明确永远由固定账号处理的节点

### 5.2 固定候选用户

```xml
flowable:candidateUsers="zhangsan,lisi"
```

适用场景：

- 少量固定人池

### 5.3 固定候选组

```xml
flowable:candidateGroups="role_ADMIN_ROLE"
```

适用场景：

- 固定角色池办理

### 5.4 标准动态办理人

```xml
flowable:assignee="${nextAssignee}"
```

### 5.5 标准动态候选用户

```xml
flowable:candidateUsers="${nextCandidateUsers}"
```

### 5.6 标准动态候选组

```xml
flowable:candidateGroups="${nextCandidateGroups}"
```

这 3 个是平台最推荐的动态分配写法。

---

## 6. 明确禁止的高耦合写法

下面这些写法不再推荐作为平台标准：

```xml
flowable:assignee="${customApproverId}"
flowable:assignee="${recordOwnerUserId}"
flowable:assignee="${dispatchTargetUserId}"
flowable:candidateGroups="${customDynamicGroupCode}"
```

原因：

1. 这些名字是业务私有字段，不是平台标准字段
2. 新业务系统看不懂，也无法复用
3. BPMN 一旦依赖这类字段，平台就必须理解业务领域
4. 文档、前端、后端、流程设计人员容易出现理解分裂

正确做法是：

1. 业务系统先根据自己的字段算出真正的下一办理结果
2. 平台统一把结果回写到：
   - `nextAssignee`
   - `nextCandidateUsers`
   - `nextCandidateGroups`
3. BPMN 只读取这 3 个标准变量

---

## 7. 业务字段与平台变量的正确关系

例如业务系统里可能有这些字段：

- `customApproverId`
- `recordOwnerUserId`
- `dispatchTargetUserId`
- `reviewerPoolCode`

它们可以作为：

- 业务主表字段
- 业务规则输入
- 页面展示字段

但它们不应该直接成为 BPMN 变量契约。

正确链路：

1. 业务系统读取自己的业务责任人字段
2. 业务系统决定“下一步给谁”
3. 业务系统或 assignment provider 输出 `nextAssignee=test3`
4. BPMN 用 `${nextAssignee}`

---

## 8. 自动分配输出规范

无论分配规则来自哪里：

- LDAP
- 业务数据库
- HTTP 规则服务
- 前端手工指定

最终都必须统一输出到下面 3 个平台结果：

- `assignee`
- `candidateUsers`
- `candidateGroups`

平台再统一写入流程变量：

- `nextAssignee`
- `nextCandidateUsers`
- `nextCandidateGroups`

---

## 9. 身份与候选组规范

### 9.1 候选组推荐统一格式

角色型候选组推荐统一为：

```text
role_<ROLE_CODE>
```

示例：

- `role_ADMIN_ROLE`
- `role_CITY_REVIEWER`
- `role_LEGACY_ROLE_17`

### 9.2 为什么要做 identity alias

因为实际系统里角色来源可能不同：

- LDAP 角色码
- JWT 角色码
- 业务库角色码

推荐在 `workflow.identity.role_aliases` 中做归一：

```yaml
workflow:
  identity:
    role_aliases:
      LDAP_ROLE_CITY_REVIEWER: CITY_REVIEWER
      LEGACY_ROLE_17: CITY_REVIEWER
```

这样 BPMN 只需要写统一后的组编码。

---

## 10. BPMN 路由规范

### 10.1 退回、补正、重审怎么做

统一通过：

- `complete`
- 结果变量
- 网关条件表达式

例如：

- `result=APPROVED`
- `result=REWORK`
- `needExpert=true`

而不是新增：

- 任意跳转接口
- 指定节点回退接口

### 10.2 推荐的变量控制方式

```xml
${result == 'APPROVED'}
${result == 'REWORK'}
${needExpert == true}
```

---

## 11. 表单模型使用边界

Flowable 表单模型只负责：

- 给节点挂一个 `formKey`
- 给平台提供字段参考

不负责：

- 替代业务表单
- 替代业务详情页
- 直接作为最终前端渲染协议

因此推荐做法是：

1. 业务系统自己维护业务页面
2. `GET /tasks/{id}/form-ref` 只作为参考能力

---

## 12. 前端展示边界

前端展示必须把两个概念拆开：

### 12.1 流程进度

使用：

- `progress-timeline`
- `progress-view`
- 必要时再配合 `task-records`

表示：

- 主链路步骤到底走到了哪里
- 当前真正处于哪个节点
- 每个主步骤下发生过什么动作

### 12.2 任务动作

使用：

- `action-timeline`

表示：

- 谁认领了
- 谁委派了
- 谁转办了
- 谁解决了委派

不要把这两个概念混成一个时间线。

补充说明：

- `progress-view`
  - 适合做流程图高亮、步骤结构分析、顶部摘要
  - 不建议直接原样渲染成审批进度主列表
- `progress-timeline`
  - 适合做审批进度主列表
- `task-records`
  - 适合补充主步骤下的转办、委派、解决委派、完成等动作

---

## 13. 建模检查清单

发布 BPMN 前至少检查下面 10 项：

1. 是否优先使用 `${nextAssignee}` / `${nextCandidateUsers}` / `${nextCandidateGroups}`
2. 是否避免直接写业务私有责任人变量
3. 是否没有引入任意跳转设计
4. 网关条件是否只依赖稳定业务结果变量
5. 候选组编码是否统一
6. 固定账号是否真的是长期固定节点
7. 租户、系统编码、发起人变量是否能正确注入
8. 表单 key 是否只作为参考，不承担业务主数据职责
9. 父子流程中 `callActivity` 的 calledElement 是否稳定
10. 流程图里使用的用户和组是否真实存在于当前身份体系

---

## 14. 最后结论

如果只记一句话，请记这句：

- BPMN 只消费平台标准变量，业务私有字段先在业务层算完，再映射成平台标准结果。
