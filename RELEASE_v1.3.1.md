# go-common workflow v1.3.1 发布摘要

## 1. 版本信息

- 版本号：`v1.3.1`
- 发布时间：`2026-04-15`
- 版本定位：在 `v1.3.0` 正式稳定版基础上的易用性增强版

## 2. 这次版本主要解决什么问题

这次版本重点不是新增一大批全新接口，而是让业务项目接入 `go-common/workflow` 更简单、更明确、更容易排障。

主要解决：

- 业务项目如何用最少代码注册工作流模块
- LDAP / 本地登录态如何尽量共用一套 BPMN
- 自动分配如何通过配置直接启用
- 数据库 / Redis 如何统一使用 `databases.*` 配置
- 开发者拿到版本后先看什么、怎么排障

## 3. 本次版本核心变化

### 3.1 业务项目推荐一行注册

业务项目现在推荐直接使用：

```go
workflowapi.MustRegisterFromConfig(server)
```

这样业务侧不需要自己手动初始化：

- 目录服务
- 自动分配服务
- Flowable REST client
- 表单引用服务
- 工作流标准路由

### 3.2 目录服务来源统一

当前统一支持：

- `workflow.directory.provider=none`
- `workflow.directory.provider=ldap`

同时兼容历史 `ldap_*` 配置。

### 3.3 自动分配 provider 统一

当前统一支持：

- `workflow.assignment.provider=none`
- `workflow.assignment.provider=directory`
- `workflow.assignment.provider=ldap`

标准变量名建议固定使用：

- `nextAssignee`
- `nextCandidateUsers`
- `nextCandidateGroups`

### 3.4 身份归一能力增强

推荐使用：

- `workflow.identity.role_aliases`
- `workflow.identity.group_aliases`

作用：

- 统一 LDAP / JWT / 业务库中的角色与组编码
- 降低登录来源切换时改 BPMN 的概率

### 3.5 数据源配置更推荐 `databases.*`

业务项目现在更推荐使用结构化配置：

- `databases.mysql.*`
- `databases.postgres.*`
- `databases.kingbase.*`
- `databases.dm.*`
- `databases.mongo.*`
- `databases.redis.*`

MySQL 现已支持通过 YAML 直接表达：

- `charset`
- `parse_time`
- `loc`
- `params`

Redis 同时兼容：

- 无密码
- 有密码

## 4. 升级建议

如果业务项目从 `v1.3.0` 升到 `v1.3.1`，建议按下面顺序处理：

1. 工作流注册统一改成 `workflowapi.MustRegisterFromConfig(server)`
2. 显式配置 `workflow.directory.provider`
3. 如需自动分配，显式配置 `workflow.assignment.provider`
4. 如需 LDAP / 本地登录态共用 BPMN，补齐 `role_aliases` / `group_aliases`
5. 数据库与 Redis 尽量迁移到 `databases.*`

## 5. 推荐开发者先看什么

推荐阅读顺序：

1. `docs/工作流v1.3.1业务项目最小接入手册.md`
2. `docs/工作流v1.3.1正式发布说明.md`
3. `docs/工作流配置说明.md`
4. `docs/工作流版本变更说明.md`

## 6. 兼容说明

当前仍兼容：

- `workflowapi.NewDefaultModuleFromConfig()` + `module.Register(server)`
- 历史 LDAP 全局配置 `ldap_*`
- 历史 Redis 配置 `redis_addr` / `redis_password`

因此，`v1.3.1` 的目标是“升级更清晰”，不是“强制重写老项目”。

## 7. 建议发版命令

```bash
git status
git add README.md CHANGELOG.md docs/ RELEASE_v1.3.1.md
git commit -m "release: workflow v1.3.1"
git tag -a v1.3.1 -m "go-common workflow v1.3.1"
git push origin <your-branch>
git push origin v1.3.1
```

## 8. 可直接转发的简版通知

各业务开发同学：

`go-common/workflow v1.3.1` 已准备发布。当前版本重点提升业务项目接入易用性，推荐统一使用 `workflowapi.MustRegisterFromConfig(server)` 注册工作流模块，并通过 `workflow.directory.provider`、`workflow.assignment.provider`、`workflow.identity.role_aliases/group_aliases` 完成目录、分配和身份归一配置。数据库与 Redis 推荐统一迁移到 `databases.*` 配置结构。新接入请优先阅读 `docs/工作流v1.3.1业务项目最小接入手册.md` 和 `docs/工作流v1.3.1正式发布说明.md`。
