# Repository Guidelines

本仓库是一个 Go 模块（`go 1.22.3`），提供共享工具库（HTTP 服务/客户端、ORM、日志、RBAC、LDAP、JWT 工具等）。请参考以下说明以保持贡献一致性。

## 项目结构与模块组织
- 根目录为模块入口（`go.mod`、`go.sum`）。
- 按功能划分包目录：`approval/`、`config/`、`http/`、`ldap/`、`log/`、`model/`、`orm/`、`rbac/`、`utils/`。
- `example/` 目录包含可运行示例（`example/main.go`）与配置样例（`example/config.yaml`）。
- 测试文件与实现同目录，命名为 `*_test.go`。

## 构建、测试与开发命令
- `go test ./...` 运行全部测试。
- `go test ./http -run TestName` 运行指定包的单个测试。
- `go vet ./...` 检查常见问题。
- `go build ./...` 验证模块是否可编译。
- `go run ./example` 运行示例程序（需要配置文件）。

## 编码风格与命名约定
- 使用 `gofmt` 格式化（`gofmt -w path/to/file.go`）。
- 缩进使用 tab（由 gofmt 统一）。
- 导出标识符使用 `CamelCase`，未导出使用 `lowerCamel`。
- 文件与包名使用小写、简短且语义明确。

## 测试规范
- 使用 Go 标准库 `testing`，测试与实现同目录。
- 新功能优先使用表驱动测试。
- 测试应保持确定性；避免真实外部网络调用（必要时使用 mock）。

## 提交与 PR 指南
- 此处无 Git 历史；提交信息建议简短、命令式（如 `orm: add mysql config defaults`）。
- PR 需包含：简要说明、变更原因、执行过的命令（如 `go test ./...`）。
- 行为变更需说明配置影响，并提供必要示例。

## 安全与配置提示
- 配置由 YAML/Viper 驱动；避免提交敏感信息（如数据库密码、JWT 密钥）。
- 新增配置项时，请同步更新 `example/config.yaml` 或 README 使用说明。
