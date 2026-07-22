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
- `formKey`、回调变量、路由变量分别承担什么职责

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

### 0.4 3 分钟速读

如果你现在只想先知道“流程图到底应该怎么画，哪些写法会把项目越做越乱”，先记下面 7 条：

1. 新项目标准主路线：
   - Flowable UI 建模
   - Flowable 平台接 LDAP
   - BPMN 优先写 `candidateGroups`
2. 办理人配置优先级固定：
   - `candidateGroups`
   - `candidateUsers`
   - `assignee`
   - `${nextAssignee}` / `${nextCandidateUsers}` / `${nextCandidateGroups}` 只作历史兼容或特殊动态场景
3. 不要把业务私有责任人字段直接写进 BPMN 长期使用。
4. 路由变量和办理人变量分开：
   - `result / needExpert` 决定流程往哪走
   - `next*` 变量只是在兼容模式下决定谁来办
5. `formKey` 只负责表单引用，不负责替代业务表单和业务校验。
6. `formKey` 不等于必须配置 `workflow.formref.db_instance`。
7. 如果流程图里的人、组、角色，Flowable UI 当前连着的 LDAP 根本查不到，那这张图即使能发布，运行期也迟早出问题。

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
5. 新项目接入时尽量做到少配置、少心智负担、拿来即用
6. 流程设计人员在 `flowable-ui` 中看到的配置，应尽量就是运行时真实生效的配置
7. 身份、人、组、角色来源要稳定清晰，不能一边在 UI 配，一边又在业务系统里另外算一套

一句话概括：

- 标准主路线下，Flowable 平台结合 LDAP 负责“节点由谁处理”
- 业务系统负责业务数据、业务状态、业务校验、业务与组织关系同步
- BPMN 负责表达流程结构、节点责任主体、分支条件和表单引用

这里先把口径说死：

- `nextAssignee / nextCandidateUsers / nextCandidateGroups`
  - 不是删除
  - 仍然保留
  - 但已经降级为历史兼容路线或特殊动态场景补充能力
- 新项目默认不再把“业务系统运行时临时算审批人”作为第一选择

---

## 3. 分层原则

### 3.1 Flowable 平台层负责什么

- 提供 `flowable-ui`
  - 让流程设计人员建模、发布、配置节点办理人
- 提供 `flowable-rest`
  - 让业务系统运行时发起流程、查询任务、完成任务、查询轨迹
- 接入 LDAP
  - 让 Flowable 平台在建模和运行时都能读取统一的用户、组、角色
- 提供 `go-common/workflow`
  - 给业务系统暴露统一的工作流 API、callback、progress、timeline、task-records、form-ref 等能力

### 3.2 业务系统层负责什么

- 业务规则
- 业务字段校验
- 业务主表和业务状态
- 业务详情查询与业务表单提交
- 把本系统真实的人岗角色关系同步到 LDAP

说明：

- 业务系统当然仍然可以在少数场景下决定“下一步给谁”
- 但这不再是公司级标准主路线
- 标准主路线是：
  - 先在 LDAP 中把组织、人、角色、组准备好
  - 再在 `flowable-ui` 中直接把节点配置成候选组、候选用户或固定办理人

### 3.3 BPMN 层只负责什么

- 节点定义
- 路由判断
- 候选人 / 候选组 / 办理人表达
- 表单引用
- 父子流程结构关系

也就是说，BPMN 要表达的是：

- 这个节点是什么职责
- 这个节点应该由哪个组或哪些人处理
- 满足什么条件流向哪条分支

而不是：

- 去理解业务数据库字段细节
- 去承载业务系统自己定义的一套非标准身份规则

### 3.4 禁止倒挂

禁止以下做法：

- BPMN 直接长期依赖业务私有变量名
- 平台代码直接识别某个业务系统的私有责任人字段
- 前端默认值直接替代流程规则
- 为了省事，在业务系统里重新造一套与 Flowable UI 脱节的办理人解析逻辑

### 3.5 标准主路线与历史兼容路线

当前统一口径如下：

1. 标准主路线
   - Flowable UI 建模
   - Flowable 平台接 LDAP
   - BPMN 优先配置 `candidateGroups`
   - 其次配置 `candidateUsers`
   - 最后才是 `assignee`
2. 历史兼容路线
   - 业务系统运行时计算 `nextAssignee / nextCandidateUsers / nextCandidateGroups`
   - BPMN 用 `${nextAssignee}` 等表达式消费这些变量
3. 可选增强能力
   - `formref` 的 DB fallback
   - `task-records`
   - `action-timeline`

如果你是新项目：

- 请按标准主路线做
- 默认不要先开 `${nextAssignee}` / `${nextCandidateUsers}` / `${nextCandidateGroups}` 这套兼容路线

如果你是老项目迁移中：

- 可以暂时保留动态变量方式
- 但文档、命名、流程设计都要逐步往标准主路线收敛

---

## 4. 平台标准变量

平台变量要分成两类理解：

1. 标准上下文变量
2. 历史兼容动态分配变量

### 4.1 标准上下文变量

以下变量定义为平台标准稳定变量：

- `starterId`
- `starterName`
- `startUserId`
- `tenantId`
- `systemCode`
- `bizId`
- `bizType`
- `title`
- `name`
- `managerId`
- `managerName`
- `departmentId`
- `departmentName`
- `callbackUrl`
- `callbackEvents`

这些变量的作用分别是：

- `starterId / starterName / startUserId`
  - 表示流程发起人上下文
- `tenantId / systemCode`
  - 表示租户和系统上下文
- `bizId / bizType / title / name`
  - 表示业务关联主键和流程展示标题
- `managerId / managerName / departmentId / departmentName`
  - 可作为路由辅助上下文，但不应成为新项目主办理人分配契约
- `callbackUrl / callbackEvents`
  - 用于 Flowable 回调业务系统

### 4.2 历史兼容动态分配变量

以下变量继续保留，但定位已经变成历史兼容或特殊动态场景能力：

- `nextAssignee`
- `nextCandidateUsers`
- `nextCandidateGroups`

含义分别是：

- `nextAssignee`
  - 下一节点唯一办理人
- `nextCandidateUsers`
  - 下一节点候选用户列表
- `nextCandidateGroups`
  - 下一节点候选组列表

这 3 个变量什么时候用：

1. 老流程已经大量依赖 `${nextAssignee}` 等表达式
2. 某些节点必须由业务规则实时算出具体用户或组
3. 暂时还没有把角色关系沉淀进 LDAP 和 Flowable UI 配置

什么时候不应该优先用：

1. 新项目首次接入
2. 可以直接在 `flowable-ui` 中配置候选组的场景
3. 明明是稳定组织职责，却仍然让业务系统每次启动流程都临时传人

---

## 5. 允许进入 BPMN 的写法

本章只说明“BPMN 里允许出现什么写法”。

但允许，不等于同等推荐。

当前推荐优先级固定为：

1. `candidateGroups`
2. `candidateUsers`
3. `assignee`
4. `${nextAssignee}` / `${nextCandidateUsers}` / `${nextCandidateGroups}` 历史兼容写法

### 5.1 首选写法：固定候选组

```xml
flowable:candidateGroups="role_CITY_REVIEWER"
```

适用场景：

- 某个节点属于明确职责岗位
- 一组人都可以处理
- 希望流程图本身就能表达“这个环节归哪个角色池负责”

这是新项目最推荐的主路线写法。

原因：

1. 设计人员在 `flowable-ui` 中能直接看懂
2. 运行时不依赖业务系统额外算审批人
3. 和 LDAP 角色、候选组机制天然一致
4. 更符合 Flowable 官方常规建模方式

#### 5.1.1 一个节点配置多个 `candidateGroups` 到底是什么意思

示例：

```xml
flowable:candidateGroups="role_city_reviewer,role_province_reviewer"
```

这表示：

1. 这是一个共享候选池
2. `role_city_reviewer` 组成员可以处理
3. `role_province_reviewer` 组成员也可以处理
4. 任意一个组里的任意一个人处理一次，这个节点就结束

它的真实语义是：

- OR
- 任一组可处理

它不是：

- 组 A 先审，组 B 后审
- 组 A 和组 B 都必须各处理一次
- 会自动变成两级审批

所以只有当业务语义本身就是“这几个组里任意一组处理即可”时，才应该把多个组直接写在同一个节点上。

#### 5.1.2 多个组到底该怎么建模

开发最容易误解的是：

- “一个节点写两个组”不等于“两级审批”
- “一个节点写两个组”也不等于“两个组都必须处理”

正确判断方式如下：

1. 如果你的意思是“组 A 或组 B 任意一组处理即可”
   - 用一个节点
   - 在同一个 `candidateGroups` 里配置多个组
2. 如果你的意思是“必须先组 A，再组 B”
   - 用两个串行 `userTask`
   - 不要把两个组堆到一个节点里
3. 如果你的意思是“组 A 和组 B 都要各处理一次”
   - 用并行网关拆成两个节点
   - 或者使用更明确的会签/多实例方案

对照示例如下：

场景 1：任一组处理即可

```xml
<userTask id="reviewTask"
          name="审核"
          flowable:candidateGroups="role_city_reviewer,role_province_reviewer" />
```

场景 2：组 A 后组 B

```xml
<userTask id="cityReview"
          name="市级审核"
          flowable:candidateGroups="role_city_reviewer" />

<userTask id="provinceReview"
          name="省级审核"
          flowable:candidateGroups="role_province_reviewer" />
```

场景 3：组 A 和组 B 都要处理

```xml
开始
  -> 并行网关
     -> 组A审核节点
     -> 组B审核节点
  -> 汇聚
  -> 后续节点
```

### 5.2 次选写法：固定候选用户

```xml
flowable:candidateUsers="zhangsan,lisi"
```

适用场景：

- 人员池比较小且稳定
- 暂时没有合适的候选组
- 演示流程或短期试点流程

注意：

- 长期来看，只要是职责型节点，仍然更推荐候选组

### 5.3 再次选写法：固定办理人

```xml
flowable:assignee="zhangsan"
```

适用场景：

- 永久固定技术节点
- 极少数明确永远由固定账号处理的节点

不建议滥用的原因：

- 一旦人员变化，就必须改模型重新发布

### 5.4 历史兼容写法：动态办理人

```xml
flowable:assignee="${nextAssignee}"
```

### 5.5 历史兼容写法：动态候选用户

```xml
flowable:candidateUsers="${nextCandidateUsers}"
```

### 5.6 历史兼容写法：动态候选组

```xml
flowable:candidateGroups="${nextCandidateGroups}"
```

这 3 个写法当前仍然允许，但定位已经调整为：

- 历史兼容
- 过渡迁移
- 少量必须运行时决定办理人的特殊场景

不是新项目的首推建模方式。

#### 5.6.1 动态多个组的真实语义

如果 BPMN 中这样写：

```xml
flowable:candidateGroups="${nextCandidateGroups}"
```

而业务系统运行时传入：

```json
{
  "nextCandidateGroups": [
    "role_city_reviewer",
    "role_province_reviewer"
  ]
}
```

那么它的语义和固定多个组完全一致：

1. 这是一个共享候选池
2. `role_city_reviewer` 可以处理
3. `role_province_reviewer` 也可以处理
4. 任意一组成员处理后，节点结束

它仍然不是：

- 顺序审批
- 双方都必须处理
- 自动会签

也就是说：

- 动态变量只是在“谁来处理”这件事上运行时算值
- 不会改变 Flowable 对单个 `userTask` 的基本语义

#### 5.6.2 动态变量生效的两个前提

动态变量是否生效，只看两个条件是否同时满足：

1. BPMN 里明确写了变量表达式
   - `${nextAssignee}`
   - `${nextCandidateUsers}`
   - `${nextCandidateGroups}`
2. 业务系统在启动流程或提交当前节点时，真的传了对应变量

少一个都不行。

错误理解示例：

1. BPMN 写死了 `flowable:candidateGroups="role_city_reviewer"`，但业务系统又传 `nextCandidateGroups`
   - 这不会把固定组自动替换成动态组
2. BPMN 写死了 `flowable:assignee="qxtest"`，但业务系统又传 `nextAssignee="lisi"`
   - 这不会把 `qxtest` 自动替换成 `lisi`
3. BPMN 根本没写 `${nextCandidateGroups}`，但开发以为只要后端传了变量就会生效
   - 也不会生效

结论必须记清楚：

- 是否动态，先由 BPMN 决定
- 具体传谁，再由业务系统决定

---

## 6. 明确禁止的高耦合写法

下面这些写法是反例，用于说明什么不应该继续作为平台标准。
它们不是推荐配置，也不是兼容示例模板：

```xml
flowable:assignee="${bizSpecificApproverId}"
flowable:assignee="${bizSpecificOwnerUserId}"
flowable:assignee="${bizSpecificDispatchUserId}"
flowable:candidateGroups="${bizSpecificDynamicGroupCode}"
```

原因：

1. 这些名字是业务私有字段，不是平台标准字段
2. 新业务系统看不懂，也无法复用
3. BPMN 一旦依赖这类字段，平台就必须理解业务领域
4. 文档、前端、后端、流程设计人员容易出现理解分裂

这些写法的问题不是 Flowable 不能运行，而是：

- 可以运行
- 但不适合作为公司级公共工程规范

### 6.1 标准主路线正确做法

1. 先把业务职责关系同步到 LDAP
2. 在 `flowable-ui` 中把节点配置成：
   - 固定候选组
   - 固定候选用户
   - 固定办理人
3. 让 Flowable 平台自己根据节点配置去决定谁能处理

### 6.2 历史兼容路线正确做法

1. 业务系统先根据自己的字段算出真正的下一办理结果
2. 平台统一把结果回写到：
   - `nextAssignee`
   - `nextCandidateUsers`
   - `nextCandidateGroups`
3. BPMN 只读取这 3 个标准变量

### 6.3 正式流程里禁止写死测试账号

下面这类写法，在演示环境短期自测时可能能跑，但不能作为正式流程模型发布：

```xml
flowable:assignee="qxtest"
flowable:assignee="test"
flowable:assignee="admin"
```

禁止原因：

1. 这类账号往往只是测试账号，不代表真实岗位职责
2. 一旦流程发到正式环境，任务会被直接派到错误的人
3. 开发、测试、实施、流程设计人员都会误以为“这个节点就该给这个人”
4. 后续即使业务系统传了动态变量，也不一定能覆盖这个固定写法

正式流程应该改成下面两类之一：

1. 固定职责节点
   - 用 `candidateGroups`
2. 确实必须运行时决定
   - 用 `${nextAssignee}` / `${nextCandidateUsers}` / `${nextCandidateGroups}`

### 6.4 禁止固定配置与动态变量混用

一个节点要么是固定节点，要么是动态节点，不要混成“半固定半动态”。

错误示例 1：

```xml
flowable:candidateGroups="role_city_reviewer"
```

业务系统又传：

```json
{
  "nextCandidateGroups": ["role_province_reviewer"]
}
```

问题：

- 开发容易误以为后端传参会覆盖固定组
- 实际排障时很难一眼判断到底谁生效

错误示例 2：

```xml
flowable:assignee="qxtest"
```

业务系统又传：

```json
{
  "nextAssignee": "lisi"
}
```

问题：

- 流程图上看是固定给 `qxtest`
- 业务代码里又以为自己传的是 `lisi`
- 两边理解会分裂

错误示例 3：

```xml
flowable:assignee="${nextAssignee}"
flowable:candidateGroups="role_city_reviewer"
```

问题：

1. 节点责任边界不清晰
2. 后续谁来接任务难以解释
3. 出问题时很难判断是 BPMN 配置错还是业务变量错

平台建议的原则只有一句话：

- 固定职责就固定配置
- 运行时计算就纯动态变量
- 不要把两套模式揉在同一个节点里

第一次接入时，可以直接记成一句话：

- 业务私有责任人字段可以存在于业务表、业务规则、页面参数里
- 但它们不应直接成为 BPMN 的长期契约
- 新项目 BPMN 应优先直接消费 Flowable 标准的人、组配置能力
- 老项目兼容时，才通过 `next*` 变量做平台标准映射

---

## 7. 业务字段与平台变量的正确关系

例如业务系统里可能有这些字段，或者你过去在流程里见过类似字段：

- `bizSpecificApproverId`
- `bizSpecificOwnerUserId`
- `bizSpecificDispatchUserId`
- `reviewerPoolCode`

它们可以作为：

- 业务主表字段
- 业务规则输入
- 页面展示字段

但它们不应该直接成为 BPMN 变量契约。

### 7.1 标准主路线正确链路

1. 业务系统把用户、角色、岗位、组织关系同步到 LDAP
2. Flowable UI 里把节点配置成候选组或候选用户
3. Flowable 平台运行时按标准身份体系分配任务

### 7.2 历史兼容路线正确链路

1. 业务系统读取自己的业务责任人字段
2. 业务系统决定“下一步给谁”
3. 业务系统或 assignment provider 输出 `nextAssignee=test3`
4. BPMN 用 `${nextAssignee}`

---

## 8. 历史兼容动态分配输出规范

本章不是新项目主路线，只用于：

- 老项目兼容
- 迁移过渡
- 特殊动态分配场景

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

这里再次强调：

- 这套输出规范的意义是兼容标准化
- 不是要求所有新项目都必须走这套运行时分配机制

如果你的节点本来就能在 `flowable-ui` 中直接配置候选组：

- 请直接配候选组
- 不要额外再绕一圈把结果塞进 `nextCandidateGroups`

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

推荐原因：

1. 业务系统、LDAP、JWT、前端展示更容易统一
2. 流程设计人员看到组编码时不容易误解
3. 后续做权限映射、候选组展示、待办解释时更稳定

### 9.2 Flowable 平台 LDAP 是主来源

公司级标准主路线下：

- 流程节点实际能选谁、哪个组能处理，优先以 Flowable 平台接入的 LDAP 为准
- 业务系统不再作为默认主身份源

也就是说：

1. 流程设计人员在 `flowable-ui` 中能选到哪些用户和组
2. 流程运行时任务到底落到哪些候选用户和组

这两件事应该尽量来源一致。

### 9.3 `identity alias` 的定位

因为实际系统里角色来源可能不同：

- LDAP 角色码
- JWT 角色码
- 业务库角色码

所以 `workflow.identity.role_aliases` 的定位是：

- 辅助归一
- 历史桥接
- 前后端展示和权限兼容

不是新项目第一优先配置项。

示例：

```yaml
workflow:
  identity:
    role_aliases:
      LDAP_ROLE_CITY_REVIEWER: CITY_REVIEWER
      LEGACY_ROLE_17: CITY_REVIEWER
```

这样做的收益是：

- BPMN 只需要写统一后的组编码
- 旧系统角色名和新系统角色名可以逐步收敛

但再次强调：

- 别把 `identity alias` 当成替代 LDAP 主路线的核心方案

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

### 10.3 路由变量和办理人变量要分开

这是很多项目容易混淆的点。

路由变量用于决定：

- 流程往哪条分支走
- 是否进入会签、补正、驳回、归档、子流程

办理人变量用于决定：

- 这个节点谁能处理

因此：

- `result`
- `needExpert`
- `needArchive`
- `needNotify`

这类变量属于路由或业务状态变量。

- `nextAssignee`
- `nextCandidateUsers`
- `nextCandidateGroups`

这类变量才是兼容模式下的办理人变量。

不要把两类变量混用。

---

## 11. 表单模型使用边界

表单引用一直是容易误解的点，这里要明确三层边界。

### 11.1 Flowable 表单模型负责什么

Flowable 表单模型只负责：

- 给节点挂一个 `formKey`
- 给平台提供字段参考
- 让流程设计人员在流程节点层面知道“这个节点大致对应什么表单”

它不负责：

- 替代业务表单
- 替代业务详情页
- 直接作为最终前端渲染协议

### 11.2 go-common 的 `form-ref` 负责什么

`go-common` 的 `GET /tasks/{id}/form-ref` 负责：

- 把节点上的 `formKey`
- 以及必要时从 Flowable REST / Flowable DB 解析出的表单引用信息
- 以统一 JSON 结构返回给业务前端

它的定位是：

- 参考信息
- 元信息输出
- 节点和前端页面之间的关联辅助

它不负责：

- 替业务前端自动生成最终业务表单
- 替业务后端做字段校验
- 保证页面一定能完全按 Flowable 表单模型自动渲染

### 11.3 业务系统前后端负责什么

业务后端负责：

- 业务表真实存储
- 业务字段合法性校验
- 业务状态流转

业务前端负责：

- 真实业务页面渲染
- 表单输入体验
- 提交参数组装
- 错误提示和交互

因此推荐做法是：

1. 业务系统自己维护业务页面
2. `GET /tasks/{id}/form-ref` 只作为参考能力
3. `formKey` 用来帮助前端决定展示哪个页面或哪个表单组件
4. 真正的业务字段校验仍在业务系统完成

### 11.4 `formKey` 和 `workflow.formref.db_instance` 不是一回事

这个点必须单独说清楚，因为很多开发第一次接入时会把它们混成一件事。

`formKey` 属于 BPMN / Flowable 模型层概念：

- 流程设计人员在节点上配置
- 用来表达“这个节点大致关联哪个表单引用”
- 它跟流程图一起发布

`workflow.formref.db_instance` 属于业务系统接入层配置：

- 写在业务系统自己的 `config.yaml`
- 由 `go-common` 在调用 `GET /api/v1/workflow/tasks/{id}/form-ref` 时按需使用
- 它不是 BPMN 字段
- 也不是 Flowable UI 里的建模属性

可以直接把两者理解成：

1. `formKey`
   - 是“流程图里写了什么引用”
2. `workflow.formref.db_instance`
   - 是“业务系统在 REST 不够用时，到哪套 Flowable 元数据库做表单引用 fallback”

### 11.5 `workflow.formref.db_instance` 什么时候和 BPMN 有关，什么时候无关

只有在下面这个链路里，它才和你画的 BPMN 间接有关：

1. BPMN 节点上配了 `formKey`
2. 业务前端或后端会调用 `GET /tasks/{id}/form-ref`
3. 当前环境下单靠 Flowable REST 解析表单引用不稳定或不完整
4. 业务系统才需要补 `workflow.formref.db_instance`

反过来说，如果满足下面任一情况：

- 前端根本不调用 `form-ref`
- 前端只用固定页面映射，不依赖 Flowable 表单引用解析
- 当前 Flowable REST 已经足够稳定拿到表单引用

那就不要因为 BPMN 里写了 `formKey`，就默认再去多配一套 `workflow.formref.db_instance`。

### 11.6 建模人员与开发人员各自要关心什么

建模人员要关心：

- 节点 `formKey` 是否命名清晰、稳定、可读
- 同类节点是否使用统一命名规则

业务开发要关心：

- 页面是否真的要调用 `form-ref`
- 如果要调用，当前环境是 REST-only 够用，还是需要 REST + DB fallback
- 如果要配 `workflow.formref.db_instance`，它是否和 `workflow.flowable.base_url` 指向同一套 Flowable 环境

所以结论很简单：

- `formKey` 是建模问题
- `workflow.formref.db_instance` 是接入配置问题
- 二者有关联，但不要把它们当成同一层配置

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

前端如果要做“一个完整步骤卡片”式展示，推荐理解为：

1. `progress-timeline`
   - 负责主步骤骨架
2. `task-records`
   - 负责给每个步骤补充子动作明细
3. `action-timeline`
   - 适合做动作审计或辅助页

而不是反过来拿动作审计时间线硬拼主进度页。

---

## 13. 建模检查清单

发布 BPMN 前至少检查下面 14 项：

1. 是否优先使用 `candidateGroups`
2. 如果不用 `candidateGroups`，是否有充分理由改用 `candidateUsers`
3. 如果直接用 `assignee`，是否确认这是长期固定节点
4. 是否避免直接写业务私有责任人变量，如 `${bizSpecificApproverId}`
5. 如果使用 `${nextAssignee}` 等变量，是否明确这只是历史兼容或特殊动态场景
6. 流程图里使用的用户和组，是否真实存在于当前 Flowable 平台接入的 LDAP 中
7. 候选组编码是否统一，例如 `role_<ROLE_CODE>`
8. 网关条件是否只依赖稳定业务结果变量，而不是直接依赖人名、账号名等脆弱字段
9. 是否没有引入任意跳转设计
10. 表单 `formKey` 是否清晰稳定，且只承担引用职责
11. 是否已经明确前端页面和 `formKey` 的映射关系
12. 父子流程中 `callActivity` 的 `calledElement` 是否稳定
13. 启动流程时是否会传入 `callbackUrl` 和 `callbackEvents`
14. 流程中的回调、表单、办理人配置是否都能被新接手的开发人员一眼看懂

---

## 14. 最后结论

如果只记一句话，请记这句：

- 新项目按 `Flowable UI + LDAP` 直接建模，BPMN 优先写候选组；业务私有字段不直接进 BPMN，`next*` 动态分配变量只作为历史兼容和特殊场景补充能力。
