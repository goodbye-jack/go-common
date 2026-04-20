# templates

这里保存 `go-common` 的版本化配置模板产物。

当前阶段规则：

- `releases/<version>/config.initial.yaml`
  - 新业务项目首次接入时可作为真实 `config.yaml` 的初始化模板
  - 当前运行兼容结构仍使用 `service_name`、`addr`
  - 默认生成 `MySQL + Redis + workflow 基础骨架`
- `releases/<version>/config.latest.yaml`
  - 当前版本完整标准模板快照
  - 用于展示当前 tag 支持的完整配置面
  - 必须尽量保留模块级、字段级注释，作为开发者理解配置项的首选参考文件
- `releases/<version>/config.full.yaml`
  - 比 `config.latest.yaml` 更适合做交付包总览
- `releases/<version>/config.workflow.yaml`
  - 仅工作流模块模板
- `releases/<version>/config.databases.yaml`
  - 仅数据源模块模板
- `releases/<version>/config.compatibility.yaml`
  - 历史兼容配置模板，不建议新项目直接使用
- `diff/<from>_to_<to>.yaml`
  - 版本间新增 / 变更 / 废弃项说明

当前阶段说明：

- 模板体系第一轮只沉淀配置契约与模板产物，不改运行时读取逻辑
- 因此模板内容必须与当前 `v1.3.1` 运行能力保持兼容
- `config.latest.yaml` 面向“读懂配置”，允许注释更详细
- `config.missing.yaml` 面向“缺失项提示和人工合并”，建议保持简洁，不强行塞入大量解释性注释
