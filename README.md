<font color="red">the project is under development</font>

# go-common

go-common is a library for some common functions such as logging, configuration, orm, http client, http server, http routes, http rbac middleware etc.

## 文档

- 工作流v1.3.0对外发布通知：`docs/工作流v1.3.0对外发布通知.md`
- 工作流v1.3.0正式发布说明：`docs/工作流v1.3.0正式发布说明.md`
- 工作流正式版发布检查结果：`docs/工作流正式版发布检查结果.md`
- 工作流正式版发布执行步骤：`docs/工作流正式版发布执行步骤.md`
- 工作流版本变更说明：`docs/工作流版本变更说明.md`
- 工作流配置说明：`docs/工作流配置说明.md`
- 工作流平台标准接口说明：`docs/工作流平台标准接口说明.md`
- 工作流接口收口说明：`docs/工作流接口收口说明.md`
- 工作流业务接入指南：`docs/工作流业务接入指南.md`
- 工作流后端接入交付模板：`docs/工作流后端接入交付模板.md`
- 工作流前端接入交付模板：`docs/工作流前端接入交付模板.md`
- 工作流测试验收模板：`docs/工作流测试验收模板.md`

非正式/过程性资料统一放在：

- `docs/非正式资料/`

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
	addr := config.GetConfigString("addr")
	serviceName := config.GetConfigString("service_name")

	server := myhttp.NewHTTPServer(serviceName)

	module, err := workflowapi.NewDefaultModuleFromConfig()
	if err != nil {
		panic(err)
	}
	module.Register(server)

	server.Prepare()
	server.Run(addr)
}
```

相关配置项：

- `workflow.flowable.base_url`
- `workflow.flowable.username`
- `workflow.flowable.password`
- `workflow.flowable.timeout_seconds`
- `workflow.flowable.group_prefix`
- `workflow.flowable.role_prefix`
- `workflow.api.prefix`
- `workflow.api.sso_enabled`
- `workflow.api.roles`
- `workflow.context.user_name_header`
- `workflow.context.system_code_header`
- `workflow.context.groups_header`
- `workflow.context.roles_header`

# 基础快速开始

## 安装

go get github.com/goodbye-jack/go-common


## 配置示例

$cat  /opt/config.yml

```
server_name: go-common
addr: ":8080"
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
	addr := config.GetConfigString("addr")
	service_name := config.GetConfigString("service_name")
	server := http.NewHTTPServer(service_name)
	server.Run(addr)
}
```

$ go run main.go

## Redis 密码配置（RBAC）

RBAC 默认读取以下配置键连接 Redis：

- `redis_addr`
- `redis_password`（可选，为空字符串表示无密码）

示例：

```yaml
redis_addr: "127.0.0.1:6379"
redis_password: ""
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
