<font color="red">the project is under development</font>

# go-common

go-common is a library for some common functions such as logging, configuration, orm, http client, http server, http routes, http rbac middleware etc.

## 文档

- 工作流迁移设计：`docs/workflow-migration-design.md`
- 工作流第五阶段验证：`docs/workflow-phase5-verification.md`
- 工作流联调回归验证记录：`docs/工作流联调回归验证记录.md`
- 工作流版本变更说明：`docs/工作流版本变更说明.md`
- 工作流正式版达标路线：`docs/工作流正式版达标路线.md`
- 工作流业务接入指南：`docs/工作流业务接入指南.md`
- 工作流配置说明：`docs/工作流配置说明.md`
- 工作流v1.3.0正式发布说明：`docs/工作流v1.3.0正式发布说明.md`
- 工作流接口收口说明：`docs/工作流接口收口说明.md`
- 工作流平台标准接口说明：`docs/工作流平台标准接口说明.md`
- 工作流待办已办性能优化设计：`docs/工作流待办已办性能优化设计.md`
- 工作流待办已办优化实施路线：`docs/工作流待办已办优化实施路线.md`
- 工作流发布前检查清单：`docs/工作流发布前检查清单.md`
- 工作流正式版发布检查结果：`docs/工作流正式版发布检查结果.md`
- 工作流正式版发布执行步骤：`docs/工作流正式版发布执行步骤.md`
- 工作流标准接入检查清单：`docs/工作流标准接入检查清单.md`
- 工作流后端接入交付模板：`docs/工作流后端接入交付模板.md`
- 工作流前端接入交付模板：`docs/工作流前端接入交付模板.md`
- 工作流测试验收模板：`docs/工作流测试验收模板.md`
- 新业务系统接入工作流详细演示：`docs/新业务系统接入工作流详细演示.md`
- 工作流对外分发交付包说明：`docs/工作流对外分发交付包说明.md`
- 工作流项目推进交付模板：`docs/工作流项目推进交付模板.md`

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


# 测试命令

```

GOCACHE=/tmp go test ./ldap
GOCACHE=/tmp OPENLDAP_TEST=1 go test ./ldap -run TestOpenLDAPIntegration

```
