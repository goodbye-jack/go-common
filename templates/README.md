# templates

这里保存 `go-common` 的版本化配置模板产物。

当前阶段规则：

- `releases/<version>/config.initial.yaml`
  - 新业务项目首次接入时可作为真实 `config.yaml` 的初始化模板
  - 当前主结构统一使用 `app / server / security / storage`
  - 默认生成 `MySQL + Redis + workflow 基础骨架`
- `releases/<version>/config.latest.yaml`
  - 当前版本完整标准模板快照
  - 用于展示当前 tag 支持的完整配置面
  - 这是内部结构化模板源，业务侧主说明会从这里提取模块示例
- `releases/<version>/config.full.yaml`
  - 比 `config.latest.yaml` 更适合做交付包总览
- `releases/<version>/config.workflow.yaml`
  - 仅工作流模块模板
- `releases/<version>/config.databases.yaml`
  - 仅数据源模块模板
- `releases/<version>/config.compatibility.yaml`
  - 历史兼容配置模板，仅用于旧项目迁移时对照新旧 key
- `releases/<version>/config.layering.yaml`
  - 环境覆盖规则的机器可读策略文件，供 `go-common` 运行时校验使用
- `diff/<from>_to_<to>.yaml`
  - 版本间新增 / 变更 / 废弃项说明

当前阶段说明：

- 模板体系从 `v1.3.3` 起与运行时新配置结构保持一致
- 运行时不再直接兼容 `service_name / addr / cookie_token` 等旧 key
- 如项目仍包含旧 key，请参考自动生成的 `go-common-rules.vX.X.X.md` 中“这次需要处理的配置项”和 `config.compatibility.yaml` 迁移
- `config.latest.yaml` 继续作为内部模板真源，允许注释更详细
- `config.layering.yaml` 作为 go-common 内置校验规则，不再默认同步到业务项目
- 业务项目只保留 `go-common-rules.vX.X.X.md` 一个开发者主说明入口
