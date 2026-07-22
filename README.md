<font color="red">the project is under development</font>

# go-common

go-common is a library for some common functions such as logging, configuration, orm, http client, http server, http routes, http rbac middleware etc.

## 文档

正式工作流接入只看这 3 份：

- `docs/flowable使用/工作流平台接入总说明.md`
- `docs/flowable使用/工作流BPMN建模与变量规范.md`
- `docs/flowable使用/Flowable UI与REST本地启动说明.md`

其他仍在维护的通用文档：

- `docs/配置加载与环境覆盖规则.md`
- `docs/配置分层与环境覆盖规范.md`
- `docs/配置模板系统设计说明.md`
- `docs/配置模板使用说明.md`
- `docs/配置模板同步器说明.md`
- `docs/配置项登记规范.md`
- `docs/目录同步模块说明.md`
- `docs/changeguard关键资源保护框架接入说明.md`
- `docs/changeguard短信二次验证能力设计说明.md`

当前推荐阅读顺序：

1. `docs/flowable使用/Flowable UI与REST本地启动说明.md`
2. `docs/flowable使用/工作流BPMN建模与变量规范.md`
3. `docs/flowable使用/工作流平台接入总说明.md`
4. `docs/配置模板系统设计说明.md`
5. `docs/配置加载与环境覆盖规则.md`

## v1.3.8 升级提醒

如果你准备从 `v1.3.7` 升到 `v1.3.8`，先看这 5 点：

1. 标准 callback 路径统一为 `/api/v1/workflow/callback`。
2. 工作流标准接口默认前缀统一为 `/api/v1/workflow`；如果你仍想保留旧前缀，必须显式配置 `workflow.api.prefix`。
3. `workflow.formref.db_instance` 不再隐式回退到 `orm.DB`。
4. `workflow.formref.db_instance`、`workflow.records.db_instance` 一旦配置错误，启动阶段会直接报错。
5. `http` 包的 RBAC 初始化已经改成延迟触发，减少 import 副作用。

完整升级说明看：

- `CHANGELOG.md`
- `docs/flowable使用/工作流平台接入总说明.md`
- `docs/flowable使用/go-common-v1.3.8-工作流发版回归清单.md`

这份回归清单不是第 4 份正式接入文档，它只用于发版前验收、联调前核对和回归留痕。

版本化配置模板产物位于：

- `templates/releases/v1.3.8/`
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

- Flowable REST client 初始化
- 表单引用服务初始化
- 工作流标准路由注册

新项目最小接入时，优先只关心：

- `workflow.api.*`
- `workflow.flowable.*`
- `workflow.context.*`

下面这些能力按需开启即可，不是新项目默认前置项：

- `workflow.formref.*`
- `workflow.records.*`
- `workflow.identity.*`
- `workflow.assignment.*`

`workflow.directory.*` 是否启用，取决于业务系统是否还要通过 go-common 提供目录查询能力：

- 如果只需要调 Flowable REST 跑流程，不一定要配
- 如果还要启用 `/api/v1/workflow/me`、`/api/v1/workflow/org/**` 这类目录接口，就应按需配置

标准 callback 路径推荐固定为：

- `/api/v1/workflow/callback`

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
- `workflow.context.user_name_header`
- `workflow.context.system_code_header`
- `workflow.context.groups_header`
- `workflow.context.roles_header`

工作流标准接口鉴权规则：

- `workflow.callback.path` 对应的 Flowable 回调接口是公开入口，不依赖登录态；生产环境应配置 `workflow.callback.secret` 做回调验签。
- 其他 `/workflow/**` 标准接口要求已登录用户，并要求业务系统在 `Principal.CapabilityCodes` 中提供 `workflow_api_access` 能力。
- `workflow.api.roles` 已移除，不再通过配置文件维护可访问角色列表；业务项目应在自己的角色/能力表中把 `workflow_api_access` 绑定给默认登录角色。

按需增强配置：

- `workflow.formref.db_instance`
- `workflow.records.db_instance`

这里再强调一次：

- `workflow.formref.db_instance`
  - 指向 Flowable 元数据库实例名
  - 只用于表单引用解析的 DB fallback
- `workflow.records.db_instance`
  - 指向 `workflow_task_records` 的落库实例名
  - 这是 go-common 自己的记录表，不是 Flowable 引擎表
  - 不配置时默认走业务系统关系型 `default` 实例

`go-common/config` 在源码项目目录下会先尝试安全自动同步配置模板，再读取真实配置：

- 缺失时初始化 `config.yaml`
- 刷新 `go-common-rules.vX.X.X.md`
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
