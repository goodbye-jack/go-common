# 配置分层与环境覆盖规范

这份文件由 `go-common` 同步到业务项目，用来约束 `config.yaml` 与环境覆盖文件的写法。

## 1. 环境选择规则

- 永远先读取 `config.yaml`
- 如果设置了 `CONFIG_FILE`，则读取 `config.yaml + CONFIG_FILE`
- 如果同时设置了 `CONFIG_FILE` 和 `CONFIG_ENV`，以 `CONFIG_FILE` 为准，`CONFIG_ENV` 仅记录在启动日志中并被忽略
- 如果未设置 `CONFIG_FILE`，但设置了 `CONFIG_ENV=dev|test|prod`，则读取 `config.yaml + config.<env>.yaml`
- 如果两者都未设置，则只读取 `config.yaml`

## 2. 文件职责

`config.yaml`

- 放公共默认值
- 放所有环境都通用的配置
- 不建议放明显只属于某个环境的差异值

`config.dev.yaml / config.test.yaml / config.prod.yaml`

- 只放差异项
- 不要复制 `config.yaml` 的全量内容
- 优先覆盖网络地址、端口、数据库、对象存储、超时、调试开关等环境差异字段

## 3. app.env 语义

- `app.env` 是运行时展示值
- 它不再承担“选择环境”的职责
- 当 `CONFIG_ENV` 生效时，运行时会自动把 `app.env` 修正为当前环境
- 开发时不要通过修改 `app.env` 来切换 dev/test/prod

## 4. 覆盖文件禁止项

以下 key 不允许出现在环境覆盖文件中：

- `app.name`
- `app.env`

原因：

- `app.name` 属于应用标识，不应该按环境漂移
- `app.env` 不负责选环境，写在覆盖文件里容易和 `CONFIG_ENV` 打架

## 5. 推荐的覆盖范围

常见推荐放进环境覆盖文件的前缀：

- `server.*`
- `security.*`
- `storage.*`
- `databases.*`
- `workflow.*`
- 业务自定义的第三方接入、容量、媒体处理等差异项

## 6. 开发建议

- 本地开发优先使用 `CONFIG_ENV=dev`
- 测试环境优先使用 `CONFIG_ENV=test`
- 生产部署优先使用 `CONFIG_ENV=prod`
- Docker / K8s / 外部挂载配置优先使用 `CONFIG_FILE`

## 7. 违规处理

- 如果环境覆盖文件里出现禁止项，服务启动时会直接拒绝启动
- 如果项目升级了 `go-common` 模板版本，请同时查看：
  - `config.latest.yaml`
  - `config.missing.yaml`
  - `config.deprecated.yaml`
  - `config.layering.yaml`
