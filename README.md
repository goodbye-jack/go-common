<font color="red">the project is under development</font>

# go-common

go-common is a library for some common functions such as logging, configuration, orm, http client, http server, http routes, http rbac middleware etc.

## 文档

- 工作流v1.3.1正式发布说明：`docs/工作流v1.3.1正式发布说明.md`
- 工作流v1.3.1业务项目最小接入手册：`docs/工作流v1.3.1业务项目最小接入手册.md`
- 工作流v1.3.0对外发布通知（历史归档）：`docs/工作流v1.3.0对外发布通知.md`
- 工作流v1.3.0正式发布说明（历史归档）：`docs/工作流v1.3.0正式发布说明.md`
- 工作流正式版发布检查结果（v1.3.0 基线归档）：`docs/工作流正式版发布检查结果.md`
- 工作流正式版发布执行步骤：`docs/工作流正式版发布执行步骤.md`
- 工作流版本变更说明：`docs/工作流版本变更说明.md`
- 工作流配置说明：`docs/工作流配置说明.md`
- 配置加载与环境覆盖规则：`docs/配置加载与环境覆盖规则.md`
- 工作流身份组契约说明：`docs/工作流身份组契约说明.md`
- 配置模板系统设计说明：`docs/配置模板系统设计说明.md`
- 配置模板使用说明：`docs/配置模板使用说明.md`
- 配置模板同步器说明：`docs/配置模板同步器说明.md`
- 配置项登记规范：`docs/配置项登记规范.md`
- 工作流平台标准接口说明：`docs/工作流平台标准接口说明.md`
- 工作流接口收口说明：`docs/工作流接口收口说明.md`
- 工作流业务接入指南：`docs/工作流业务接入指南.md`
- 工作流后端接入交付模板：`docs/工作流后端接入交付模板.md`
- 工作流前端接入交付模板：`docs/工作流前端接入交付模板.md`
- 工作流测试验收模板：`docs/工作流测试验收模板.md`

非正式/过程性资料统一放在：

- `docs/非正式资料/`

当前推荐阅读顺序：

1. `docs/工作流v1.3.1业务项目最小接入手册.md`
2. `docs/工作流v1.3.1正式发布说明.md`
3. `docs/工作流配置说明.md`
4. `docs/工作流版本变更说明.md`
5. `docs/配置模板系统设计说明.md`
6. `docs/配置加载与环境覆盖规则.md`

版本化配置模板产物位于：

- `templates/releases/v1.3.3/`
- `templates/diff/`
- `configspec/modules/`

## 工作流快速开始

把工作流模块注册到业务服务：

```go
package main

import (
	"github.com/goodbye-jack/go-common/config"
	myhttp "github.com/goodbye-jack/go-common/http"
	workflowapi "github.com/goodbye-jack/go-common/workflow/api"
)

func main() {
	addr := config.GetServerAddr()
	serviceName := config.GetAppName()

	server := myhttp.NewHTTPServer(serviceName)
	workflowapi.MustRegisterFromConfig(server)

	server.Prepare()
	server.Run(addr)
}
```

`v1.3.1` 推荐业务项目直接使用这一行注册方式：

- `workflowapi.MustRegisterFromConfig(server)`

这样业务侧不需要自己关心：

- 目录服务 provider 选择
- 自动分配 provider 选择
- Flowable REST client 初始化
- 表单引用服务初始化
- 工作流标准路由注册

相关配置项：

- `workflow.flowable.base_url`
- `workflow.flowable.username`
- `workflow.flowable.password`
- `workflow.flowable.timeout_seconds`
- `workflow.flowable.group_prefix`
- `workflow.flowable.role_prefix`
- `workflow.api.enabled`
- `workflow.api.prefix`
- `workflow.api.sso_enabled`
- `workflow.api.roles`
- `workflow.context.user_name_header`
- `workflow.context.system_code_header`
- `workflow.context.groups_header`
- `workflow.context.roles_header`

`go-common/config` 在源码项目目录下会先尝试安全自动同步配置模板，再读取真实配置：

- 缺失时初始化 `config.yaml`
- 刷新 `config.latest.yaml`
- 刷新 `config.todo.yaml`
- 刷新 `config.rules.md`
- 刷新 `.go-common-config-meta.yaml`

如果是生产环境，建议显式设置：

```bash
export GO_COMMON_CONFIG_SYNC=off
```

# 基础快速开始

## 安装

go get github.com/goodbye-jack/go-common


## 配置示例

$cat  /opt/config.yml

```yaml
app:
  name: go-common-demo
  env: local

server:
  addr: ":8080"

security:
  cookie:
    name: good_token
    expired_seconds: 36000

storage:
  local:
    base_path: ""
    relative_path_prefix: ""
    cleanup_after_upload: false
```

## 示例代码

$cat main.go

```
package main

import (
	"github.com/goodbye-jack/go-common/http"
	"github.com/goodbye-jack/go-common/config"
)

func main() {
	addr := config.GetServerAddr()
	serviceName := config.GetAppName()
	server := http.NewHTTPServer(serviceName)
	server.Run(addr)
}
```

$ go run main.go

## Redis / 数据源配置

从 `v1.2.0+` 开始，业务项目推荐统一使用 `databases.*` 配置数据库与 Redis，例如：

```yaml
databases:
  mysql:
    default:
      db_name: default_mysql
      mode: single
      host: 127.0.0.1
      port: 3306
      user: root
      password: your_password
      database: your_biz_db
      charset: utf8mb4
      parse_time: true
      loc: Local
      max_open_conn: 100
      max_idle_conn: 10
      conn_max_life_time: 300s
      slow_time: 5
      log_mode: info
  redis:
    default:
      db_name: default_redis
      mode: single
      host: 127.0.0.1
      port: 6379
      password:
      database: 0
      timeout: 5
      max_pool_size: 100
      min_pool_size: 10
```

说明：

- MySQL 可直接通过 `charset`、`parse_time`、`loc`、`params` 生成完整 DSN
- Redis 同时支持“无密码”和“有密码”两种模式
- PostgreSQL / KingBase / DM / Mongo 也统一走 `databases.*` 结构

历史项目如果仍保留以下 Redis 旧 key：

- `redis_addr`
- `redis_password`（可选，为空字符串表示无密码）

请迁移到：

```yaml
databases:
  redis:
    default:
      host: 127.0.0.1
      port: 6379
      password: ""
      database: 0
```

## Tag 发布与推送

基于当前分支提交打 tag 并推送：

```bash
# 1) 确认当前分支
git branch --show-current

# 2) 在当前提交创建注释 tag
git tag -a v0.2.21 -m "release: v0.2.21"

# 3) 推送分支（建议）
git push -u origin <your-branch>

# 4) 推送指定 tag
git push origin v0.2.21
```

查看远程 tag：

```bash
git ls-remote --tags origin
```


# 测试命令

```

GOCACHE=/tmp go test ./ldap
GOCACHE=/tmp OPENLDAP_TEST=1 go test ./ldap -run TestOpenLDAPIntegration

```
