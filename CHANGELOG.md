## v1.3.1（2026-04-15）
### 变更
- **工作流业务接入进一步简化**：业务项目推荐直接使用 `workflowapi.MustRegisterFromConfig(server)` 完成标准模块注册。
- **目录服务 provider 标准化**：统一支持 `workflow.directory.provider=none|ldap`，并兼容老的全局 `ldap_*` 配置自动识别。
- **自动分配 provider 标准化**：统一支持 `workflow.assignment.provider=none|directory|ldap`，可直接回写 `nextAssignee`、`nextCandidateUsers`、`nextCandidateGroups`。
- **身份归一能力增强**：新增 / 强化 `workflow.identity.role_aliases`、`workflow.identity.group_aliases`，支持 LDAP 与本地登录态切换时尽量不改 BPMN。
- **启动摘要日志增强**：工作流模块注册时输出 provider、assignment provider、alias 数量等关键信息，方便开发排障。
- **数据库统一配置能力增强**：`databases.*` 配置继续完善，支持 MySQL `charset/parse_time/loc/params`，并兼容 PostgreSQL、KingBase、DM、Mongo、Redis 多实例初始化。
- **Redis 密码兼容保留**：历史 RBAC 读取 `redis_addr`、`redis_password` 的方式继续兼容，避免影响旧项目。
### 升级指引
1. 业务项目工作流注册统一改为 `workflowapi.MustRegisterFromConfig(server)`
2. 新项目优先配置 `workflow.directory.provider`，不再依赖隐式目录来源判断
3. 如需 LDAP / 本地登录态两种来源共用同一套 BPMN，请补齐 `workflow.identity.role_aliases` / `group_aliases`
4. 业务数据库与 Redis 推荐统一迁移到 `databases.*` 结构
5. 如启用自动分配，优先显式配置 `workflow.assignment.provider`
### 兼容说明
- 老的 `workflowapi.NewDefaultModuleFromConfig()` + `module.Register(server)` 仍可继续使用
- 老的全局 LDAP 配置 `ldap_*` 仍兼容
- 历史 RBAC Redis 配置 `redis_addr` / `redis_password` 仍兼容
- 新版本推荐文档与接入方式以 `v1.3.1` 发布说明和最小接入手册为准

## v1.3.0（2026-03-13）
### 变更
- **工作流标准接口正式收口**：固定正式标准接口清单，明确兼容接口清单，稳定对外契约
- **统一流程图入口**：新增 `diagram-view` 统一流程图视图接口，自动判断单流程和父子复合流程
- **流程发起分层**：明确 `POST /api/process/start` 为工作流标准启动接口，推荐业务系统自行封装发起业务单接口
- **待办与已办稳定化**：修复人员隔离问题，统一待办、已办过滤口径，列表默认轻量返回
- **任务上下文与表单引用稳定化**：固定 `task`、`business`、`variables`、`formRef` 四段结构，表单模型仅作节点字段参考
- **进度能力正式化**：稳定 `progress-view`、`progress-timeline` 按流程实例和按业务单号两条查询路径
- **业务联调达标**：完成文物鉴定流程、文物告警流程以及包含 `callActivity` 的父子流程图场景联调
- **正式版配置与发布文档补齐**：新增工作流配置说明、正式版发布检查结果、正式发布执行步骤和正式发布说明正文
### 升级指引
1. 新系统统一改用 `GET /api/process-instances/{id}/diagram-view`
2. 旧系统如仍依赖 `definition-xml` 或 `composite-diagram`，可先保持兼容调用，再逐步迁移
3. 业务系统发起流程时，建议统一通过业务侧发起接口封装工作流启动，而不是前端直接裸调 `POST /api/process/start`
### 兼容说明
- `GET /api/process/instance/{id}/definition-xml` 继续保留兼容
- `GET /api/process-instances/{id}/composite-diagram` 继续保留兼容
- 新接入业务系统不再推荐继续依赖以上两条兼容接口

## v1.2.0（2026-01-20）
### 变更
- **关系型DB初始化**：如果业务系统中配置了yaml的DB数据库配置,那么go-common将会自动加载NewOrm方法,业务系统中直接调用orm.DB即可使用各类方法 
- **DB中具体配置如下:**
- databases:
  - mysql: (此处mysql可以替换成dm、kingbase等)
    - default:
      - db_name: default_mysql
        mode: single (此处值见DBMode属性)
        host: 127.0.0.1
        port: 3306
        user: root
        password: 12345678
        database: warmcity
        max_open_conn: 100
        max_idle_conn: 10
        conn_max_life_time: 300s
        slow_time: 5 # 慢查询阈值5秒（可选，默认3秒）
        log_mode: info
- **Redis初始化**：NewRedis方法新增cfgFromYaml参数，initNoSQLDB调用时需传入解析后的配置（必填）
- **MongoDB初始化**：NewRedis方法新增cfgFromYaml参数，initNoSQLDB调用时需传入解析后的配置（必填）
- **RBAC客户端**：NewRbacClient不再依赖GetAddr/GetPassword方法，直接读取GetConfig()原始参数
### 升级指引
1. 修改initNoSQLDB中调用NewRedis的逻辑：redis.NewRedis(dsn, dbType, timeout, cfg)
2. 业务代码中NewRbacClient无需适配，直接升级依赖即可
### 兼容说明
- 旧版本调用方式（无cfg参数）会导致Config缺失Host/Port，建议立即升级

## v1.1.0（2026-01-19）
### 新增
- Redis通用DSN生成逻辑：genRedisDSN方法支持单机/集群模式
