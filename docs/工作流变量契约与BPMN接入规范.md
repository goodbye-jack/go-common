# 工作流变量契约与 BPMN 接入规范

## 1. 目标

本规范用于约束：

- `go-common` 工作流标准接口输出什么变量
- Flowable BPMN 应该依赖哪些稳定变量
- `workflow.directory.provider=ldap/http` 切换时，如何做到尽量不改流程图

核心原则：

1. 固定的是**变量名与语义**
2. 不固定的是**变量值**
3. BPMN 不感知底层目录来源（LDAP / HTTP）
4. 业务项目优先通过配置接入，而不是写大量私有流程代码

---

## 2. 核心变量契约

### 2.1 必须长期稳定的核心变量

- `starterId`
  - 发起人唯一标识
- `managerId`
  - 当前办理人直属上级唯一标识
- `nextAssignee`
  - 下一节点明确办理人
- `nextCandidateUsers`
  - 下一节点候选用户列表
- `nextCandidateGroups`
  - 下一节点候选组列表（值应为 Flowable 最终识别的 groupId，而不是业务别名）

### 2.2 建议长期稳定的扩展变量

- `starterName`
- `startUserId`
- `tenantId`
- `systemCode`
- `managerName`
- `departmentId`
- `departmentName`

### 2.3 允许按行业扩展的变量

- `districtLeaderId`
- `cityLeaderId`
- `provinceLeaderId`

注意：

- 扩展变量可以有
- 但 BPMN 不建议强依赖过多扩展变量
- 优先依赖核心变量契约

---

## 3. BPMN 建模规范

### 3.1 推荐写法

#### 发起人回退类节点

- `flowable:assignee="${starterId}"`

#### 上级审批类节点

- `flowable:assignee="${managerId}"`

#### 通用动态分配节点

- `flowable:assignee="${nextAssignee}"`
- `flowable:candidateUsers="${nextCandidateUsers}"`
- `flowable:candidateGroups="${nextCandidateGroups}"`

### 3.2 允许写法

- 固定办理人
- 固定候选组
- 固定候选用户

例如：

- `flowable:assignee="admin"`
- `flowable:candidateGroups="role_ADMIN_ROLE"`

### 3.3 不推荐写法

- 直接依赖 LDAP 字段名
- 直接依赖 HTTP 返回字段名
- 直接依赖某个业务项目的私有变量名

例如不推荐：

- `${leaderDn}`
- `${httpManager}`
- `${districtOfficerFromBizA}`

---

## 4. LDAP / HTTP 等价边界

### 4.1 可做到稳定等价的部分

下面这些变量适合做成 LDAP / HTTP 等价：

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

### 4.2 需要谨慎处理的部分

- `nextCandidateGroups`

原因：

- LDAP 目录通常能稳定拿到“人、部门、直属上级”
- 但不一定天然能拿到“下一环节应该归属哪个业务角色组”
- 因此动态候选组如果依赖复杂行业规则，建议：
  1. 直接在 BPMN 里配置固定候选组
  2. 或在业务页面发起/办理前显式写入候选组变量

结论：

- 核心“动态办理人”变量可以做成稳定等价
- 动态“复杂候选组”变量需要按业务情况决定是否走自定义 HTTP assignment provider

---

## 5. 外部 BPMN 接入适配规范

不能保证任意外部 BPMN 零改导入即用。

但应保证接入时只需要做**有限标准化适配**，而不是改业务代码。

### 5.1 重点检查项

#### 办理人表达式

例如把：

- `${leader}`
- `${approver}`
- `${owner}`

改为：

- `${managerId}`
- `${nextAssignee}`
- `${starterId}`

#### 候选组编码

外部流程中的组编码，应统一为 Flowable 识别的最终 groupId。

例如如果系统配置：

- `workflow.flowable.role_prefix=role_`

那么候选组应写成：

- `role_ADMIN_ROLE`
- `role_LEGACY_ROLE_20`

而不是只写：

- `ADMIN_ROLE`
- `LEGACY_ROLE_20`

#### 固定办理人标识

固定 `assignee` 时，必须确认用户标识口径与系统当前 `user_id_strategy` 一致。

#### 分支条件变量

确认流程图中的条件变量在业务启动流程 / 提交任务时真实存在。

---

## 6. Provider 使用建议

### 6.1 通用场景

当业务只需要：

- 发起人
- 直属上级
- 部门
- 回退到发起人

建议优先使用目录驱动的 assignment provider。

### 6.2 复杂场景

当业务需要：

- 按区域层级分配
- 按行业角色分配
- 按多条件路由候选组
- 按业务数据动态选审批人

建议使用：

- `workflow.assignment.provider=directory`

由业务系统显式返回：

- `assignee`
- `candidateUsers`
- `candidateGroups`
- `variables`

---

## 7. 发版建议

`go-common` 发 tag 时，建议至少同步发布以下文档：

1. `工作流配置说明.md`
2. `工作流变量契约与BPMN接入规范.md`
3. 业务接入指南
4. 最小配置示例
5. 常见问题与排查说明
