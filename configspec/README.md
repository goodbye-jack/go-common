# configspec

`configspec` 用于维护 `go-common` 的版本化配置元数据。

当前阶段目标：

- 让 `go-common` 的配置项从“散落在代码里的字符串”变成“可登记、可校验、可发布”的配置契约
- 为 `templates/releases/<version>/` 下的模板文件提供统一来源
- 为后续业务项目首次接入、版本升级、缺失项补齐提供标准数据基础

当前阶段边界：

- 只沉淀配置元数据、模板产物、文档与基础校验测试
- 不改运行时配置读取逻辑
- 不自动修改业务项目的真实 `config.yaml`

当前元数据约定：

- `modules/*.yaml`：按模块维护配置项
- `schema/config-spec.schema.json`：元数据结构约束
- 模板与文档当前以 `v1.3.1` 的实际运行能力为准

特别说明：

- 由于当前运行时仍使用 `service_name`、`addr` 读取服务信息，所以模板体系第一阶段仍以这两个字段作为运行兼容主结构
- `service.name / host / port` 与 `databases.*.*.enabled` 属于后续增强阶段，不在本轮模板产物中作为运行标准输出
