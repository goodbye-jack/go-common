# go-common 工作流文档入口

这个目录当前正式保留 4 份文档，但定位不一样。

如果你是第一次接入，请不要 4 份一起乱读，按下面顺序进入。

---

## 1. 先记住这 4 份文档分别干什么

### 1.1 `Flowable UI与REST本地启动说明.md`

只回答一件事：

- Flowable 平台怎么启动、怎么验证、地址怎么区分

适合谁先看：

- 第一次本地联调的人
- 负责搭 Flowable UI / REST / MySQL 的人
- 要先确认平台到底通没通的人

它负责说明：

- Flowable UI 怎么起
- Flowable REST 怎么起
- MySQL 怎么配
- UI 地址、REST 地址、业务系统地址、callback 外部地址怎么区分
- 业务系统最小配置先配哪些

它不负责说明：

- BPMN 节点到底该怎么画
- 前端页面具体该调用哪个工作流接口

### 1.2 `工作流BPMN建模与变量规范.md`

只回答两件事：

- 流程图应该怎么建
- BPMN 变量应该怎么约束

适合谁看：

- 流程设计人员
- 平台架构人员
- 负责流程接入的后端开发

它负责说明：

- `candidateGroups / candidateUsers / assignee` 应该怎么选
- `nextAssignee / nextCandidateUsers / nextCandidateGroups` 的兼容定位
- 哪些业务私有字段不应该直接进 BPMN
- `formKey` 的职责是什么

它不负责说明：

- 业务系统 `config.yaml` 里具体每个工作流配置项怎么配
- 前端待办、已办、进度页到底该调哪个接口

### 1.3 `工作流平台接入总说明.md`

这是业务系统接入主文档。

适合谁重点看：

- 业务后端开发
- 业务前端开发
- 负责联调和排障的人

它负责说明：

- 业务系统最小配置怎么写
- `workflow.flowable.* / workflow.callback.*` 各自是什么意思
- 标准启动流程接口怎么用
- `claim / unclaim / delegate / resolve / transfer / complete` 各自干什么
- `todo / done / task-records / progress-view / progress-timeline / action-timeline` 分别该用于什么页面
- callback 怎么接、怎么传、怎么扩展业务状态回写

这份文档是新项目接入时最核心的一份。

### 1.4 `go-common-v1.3.8-工作流发版回归清单.md`

这份不是接入手册，是发版回归清单。

适合谁看：

- 发版前验收的人
- 回归测试的人
- 想确认“这版到底测没测全”的人

它负责说明：

- 发版前哪些链路必须逐条验证
- callback、formref、records、demo、真实项目等回归点有哪些

它不负责替代前 3 份正式接入文档。

---

## 2. 第一次接入的标准阅读顺序

如果你是新项目第一次接入，固定按这个顺序看：

1. 先看 `Flowable UI与REST本地启动说明.md`
   - 先把 Flowable 平台跑通
2. 再看 `工作流BPMN建模与变量规范.md`
   - 先把流程图画对
3. 最后看 `工作流平台接入总说明.md`
   - 再把业务系统后端、前端和 callback 联调跑通

不要反过来：

- 不要先看接口文档就开始怀疑 Flowable 平台
- 不要流程图还没定下来就先大规模写业务接入代码
- 不要还没跑通最小链路，就先把所有兼容增强配置全打开

---

## 3. 新项目最小主路线

新项目第一次接入，建议默认按下面这条主路线理解：

1. Flowable UI 建模
2. Flowable 平台接 LDAP
3. BPMN 优先写 `candidateGroups`
4. 业务系统通过 `go-common` 调标准 workflow API
5. 业务系统显式传 `callbackUrl / callbackEvents`
6. 前端按标准接口拆：
   - 待办：`todo`
   - 已办：`done`
   - 流程步骤主链路：`progress-timeline`
   - 步骤补充动作：`task-records`

第一次接入默认先只关注：

- `workflow.flowable.*`
- `workflow.callback.*`

不要一上来就默认全配：

- `workflow.identity.*`
- `workflow.directory.*`
- `workflow.assignment.*`
- `workflow.formref.db_instance`
- `workflow.records.db_instance`

这些都属于：

- 历史兼容能力
- 按需增强能力
- 特殊迁移或排障能力

---

## 4. 开发最容易混掉的 6 组概念

### 4.1 Flowable UI 地址 和 Flowable REST 地址

- UI 地址
  - 给人登录、建模、发布流程定义
- REST 地址
  - 给业务系统运行时调接口

### 4.2 `workflow.callback.path` 和 `workflow.callback.public_base_url`

- `workflow.callback.path`
  - 业务系统把 callback 路由暴露在哪
- `workflow.callback.public_base_url`
  - Flowable 实际回调业务系统时使用哪个外部可达基地址

### 4.3 `formKey` 和 `workflow.formref.db_instance`

- `formKey`
  - BPMN 建模字段
- `workflow.formref.db_instance`
  - 业务系统 `form-ref` 解析 fallback 配置

### 4.4 `done` 和 `task-records`

- `done`
  - 我真正办结过的任务
- `task-records`
  - 我参与过的动作记录

### 4.5 `progress-view` 和 `progress-timeline`

- `progress-view`
  - 结构视图
- `progress-timeline`
  - 用户真正能读懂的流程步骤主链路

### 4.6 callback 注册成功 和 业务状态自动更新

- callback 注册成功
  - 只代表 go-common 标准 callback 路由存在并能接收事件
- 业务状态自动更新
  - 仍然要看业务系统有没有实现自己的 callback 扩展处理

---

## 5. 按角色推荐读法

### 5.1 后端开发

建议顺序：

1. `Flowable UI与REST本地启动说明.md`
2. `工作流平台接入总说明.md`
3. `工作流BPMN建模与变量规范.md`

重点章节：

- 最小配置
- 流程启动
- 任务动作接口
- callback
- formref / records 边界

### 5.2 前端开发

建议顺序：

1. `工作流平台接入总说明.md`
2. `工作流BPMN建模与变量规范.md`

重点章节：

- 待办 / 已办 / task-records
- progress-view / progress-timeline / action-timeline
- 表单引用和页面映射边界

### 5.3 流程设计人员

建议顺序：

1. `Flowable UI与REST本地启动说明.md`
2. `工作流BPMN建模与变量规范.md`

重点章节：

- LDAP 主路线
- 候选组建模规范
- 路由变量规范
- `formKey` 命名规范

### 5.4 发版验收人员

建议顺序：

1. 先快速回看 `工作流平台接入总说明.md`
2. 再执行 `go-common-v1.3.8-工作流发版回归清单.md`

不要直接只看回归清单，不理解接口职责边界就做回归，容易测偏。

---

## 6. 最后结论

如果你只想记最短版本，就记下面 3 句：

1. 平台启动看 `Flowable UI与REST本地启动说明.md`
2. 流程图建模看 `工作流BPMN建模与变量规范.md`
3. 业务系统接入和前后端联调看 `工作流平台接入总说明.md`

发版前最后再看：

- `go-common-v1.3.8-工作流发版回归清单.md`
