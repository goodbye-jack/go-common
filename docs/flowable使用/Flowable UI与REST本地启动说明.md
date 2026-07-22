# Flowable UI 与 REST 本地启动说明

## 0. 文档入口

当前正式工作流文档统一放在：

```text
go-common/docs/flowable使用/
```

正式只保留 3 份：

1. `工作流平台接入总说明.md`
2. `工作流BPMN建模与变量规范.md`
3. `Flowable UI与REST本地启动说明.md`

### 0.1 本文适合谁看

- 本地联调开发人员
- 测试环境部署人员
- 运维人员
- 需要自查 Flowable UI / REST / MySQL 是否正常的人

### 0.2 本文解决什么问题

本文只解决“平台怎么启动和验证”的问题，重点包括：

- `flowable-ui` 怎么启动
- `flowable-rest` 怎么启动
- Flowable MySQL 怎么持久化
- 本地账号模式和 LDAP 模式怎么切换
- 修改配置后为什么必须重新构建镜像

本文不负责说明：

- 业务系统代码怎么接入工作流
- BPMN 节点到底该怎么建模
- 前端应该调用哪些工作流接口

这些内容分别去看：

- 业务系统接入、接口使用：`工作流平台接入总说明.md`
- BPMN 画图规范、变量规范：`工作流BPMN建模与变量规范.md`

### 0.3 建议阅读顺序

第一次接入建议按下面顺序阅读：

1. 先看本文，把 `flowable-ui`、`flowable-rest`、MySQL 跑起来
2. 再看 `工作流BPMN建模与变量规范.md`，确认流程图怎么配
3. 最后看 `工作流平台接入总说明.md`，完成业务系统代码接入和前后端联调

### 0.4 3 分钟速读

如果你现在只想先把平台跑起来，并判断“到底是 Flowable 平台问题，还是业务系统接入问题”，先记下面 7 条：

1. 先区分 4 个地址：
   - Flowable UI 地址
   - Flowable REST 地址
   - 业务系统地址
   - callback 对外地址
2. 人登录、建模、发流程图，用 Flowable UI 地址。
3. 程序调流程运行时接口，用 Flowable REST 地址。
4. Flowable 回调业务系统，用 `workflow.callback.public_base_url` 对应的外部地址。
5. 业务系统第一次接入，最小只先配：
   - `workflow.flowable.*`
   - `workflow.callback.*`
6. 新项目不要一上来就默认全开：
   - `workflow.identity.*`
   - `workflow.directory.*`
   - `workflow.assignment.*`
   - `workflow.formref.db_instance`
   - `workflow.records.db_instance`
7. 修改 UI / REST 的 properties 后，如果当前 Dockerfile 是 build 时复制配置文件，必须重新 `docker compose up -d --build`，不能只 `restart`。

## 1. 文档目标

本文说明本地全量联调时，如何启动 Flowable 6.8.1 的三个依赖服务：

- Flowable MySQL
- Flowable UI
- Flowable REST

同时说明：

- UI 和 REST 使用本地账号登录时要改哪些配置
- 如果切换回 LDAP 要怎么改
- 为什么修改配置后必须重新 build 镜像
- Flowable 平台 LDAP 与业务系统侧配置的职责分界

一句话定位：

- 本文只讲 Flowable 平台侧
- 业务系统如何接 `go-common/workflow`，看《工作流平台接入总说明.md》

---

## 2. 推荐目录结构与完整示例文件

不要在正式接入文档里依赖某个人机器上的绝对路径。

推荐把 Flowable 本地联调目录组织成下面的结构：

```text
flowable-adapter/
├── compose.yaml
└── suite/
    └── deploy/
        ├── ui/
        │   ├── Dockerfile
        │   └── flowable-ui.properties
        └── rest/
            ├── Dockerfile
            └── flowable-rest.properties
```

其中：

- `compose.yaml`
  - 负责拉起 MySQL、Flowable UI、Flowable REST
- `deploy/ui/flowable-ui.properties`
  - 负责 UI 账号、LDAP、数据库连接等配置
- `deploy/rest/flowable-rest.properties`
  - 负责 REST 服务账号、数据库连接、历史级别等配置

这些都属于 Flowable 平台侧配置，不是业务系统自己的 `config.yaml`。

### 2.1 `compose.yaml` 完整示例

```yaml
services:
  mysql:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: flowable
      MYSQL_DATABASE: flowable_681_app
      MYSQL_USER: flowable_app
      MYSQL_PASSWORD: flowable
    ports:
      - "3307:3306"
    volumes:
      - flowable_681_mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost", "-pflowable"]
      interval: 5s
      timeout: 5s
      retries: 30

  flowable-ui:
    build:
      context: ./suite/deploy
      dockerfile: ui/Dockerfile
    depends_on:
      mysql:
        condition: service_healthy
    ports:
      - "8086:8080"

  flowable-rest:
    build:
      context: ./suite/deploy
      dockerfile: rest/Dockerfile
    depends_on:
      mysql:
        condition: service_healthy
    ports:
      - "8087:8080"

volumes:
  flowable_681_mysql_data:
```

### 2.2 `flowable-ui.properties` 完整示例

```properties
server.port=8080
server.servlet.context-path=/flowable-ui
spring.application.name=flowable-ui
spring.jmx.default-domain=${spring.application.name}
flowable.common.app.role-prefix=

# MySQL
spring.datasource.driver-class-name=com.mysql.cj.jdbc.Driver
spring.datasource.url=${SPRING_DATASOURCE_URL:jdbc:mysql://host.docker.internal:3307/flowable_681_app?characterEncoding=UTF-8&useSSL=false&allowPublicKeyRetrieval=true}
spring.datasource.username=${SPRING_DATASOURCE_USERNAME:flowable_app}
spring.datasource.password=${SPRING_DATASOURCE_PASSWORD:flowable}
spring.datasource.hikari.maximumPoolSize=${SPRING_DATASOURCE_HIKARI_MAXIMUM_POOL_SIZE:20}
flowable.database-schema=${FLOWABLE_DATABASE_SCHEMA:flowable_681_app}
flowable.use-lock-for-database-schema-update=true
flowable.database-schema-update=${FLOWABLE_UI_DATABASE_SCHEMA_UPDATE:false}

# Admin user bootstrap for UI apps
flowable.common.app.idm-admin.user=${FLOWABLE_COMMON_APP_IDM_ADMIN_USER:admin}
flowable.common.app.idm-admin.password=${FLOWABLE_COMMON_APP_IDM_ADMIN_PASSWORD:test}

# Engine / UI
flowable.process.servlet.path=/service
flowable.rest.app.authentication-mode=verify-privilege
flowable.app.create-demo-data=${FLOWABLE_APP_CREATE_DEMO_DATA:false}

# LDAP (UI only)
flowable.idm.ldap.enabled=${FLOWABLE_IDM_LDAP_ENABLED:false}
flowable.idm.ldap.server=${FLOWABLE_IDM_LDAP_SERVER:ldap://ldap.example.com}
flowable.idm.ldap.port=${FLOWABLE_IDM_LDAP_PORT:389}
flowable.idm.ldap.user=${FLOWABLE_IDM_LDAP_USER:cn=admin,dc=example,dc=com}
flowable.idm.ldap.password=${FLOWABLE_IDM_LDAP_PASSWORD:change_me}
flowable.idm.ldap.base-dn=${FLOWABLE_IDM_LDAP_BASE_DN:dc=example,dc=com}
flowable.idm.ldap.user-base-dn=${FLOWABLE_IDM_LDAP_USER_BASE_DN:ou=people,dc=example,dc=com}
flowable.idm.ldap.group-base-dn=${FLOWABLE_IDM_LDAP_GROUP_BASE_DN:ou=groups,dc=example,dc=com}
flowable.idm.ldap.query.user-by-id=${FLOWABLE_IDM_LDAP_QUERY_USER_BY_ID:(&(objectClass=inetOrgPerson)(uid={0}))}
flowable.idm.ldap.query.user-by-full-name-like=${FLOWABLE_IDM_LDAP_QUERY_USER_BY_FULL_NAME_LIKE:(&(objectClass=inetOrgPerson)(|(cn=*{0}*)(givenName=*{0}*)(sn=*{0}*)))}
flowable.idm.ldap.query.all-users=${FLOWABLE_IDM_LDAP_QUERY_ALL_USERS:(objectClass=inetOrgPerson)}
flowable.idm.ldap.query.groups-for-user=${FLOWABLE_IDM_LDAP_QUERY_GROUPS_FOR_USER:(&(objectClass=groupOfUniqueNames)(uniqueMember={0}))}
flowable.idm.ldap.query.all-groups=${FLOWABLE_IDM_LDAP_QUERY_ALL_GROUPS:(objectClass=groupOfUniqueNames)}
flowable.idm.ldap.query.group-by-id=${FLOWABLE_IDM_LDAP_QUERY_GROUP_BY_ID:(&(objectClass=groupOfUniqueNames)(cn={0}))}
flowable.idm.ldap.attribute.user-id=${FLOWABLE_IDM_LDAP_ATTRIBUTE_USER_ID:uid}
flowable.idm.ldap.attribute.first-name=${FLOWABLE_IDM_LDAP_ATTRIBUTE_FIRST_NAME:cn}
flowable.idm.ldap.attribute.last-name=${FLOWABLE_IDM_LDAP_ATTRIBUTE_LAST_NAME:sn}
flowable.idm.ldap.attribute.email=${FLOWABLE_IDM_LDAP_ATTRIBUTE_EMAIL:mail}
flowable.idm.ldap.attribute.group-id=${FLOWABLE_IDM_LDAP_ATTRIBUTE_GROUP_ID:cn}
flowable.idm.ldap.attribute.group-name=${FLOWABLE_IDM_LDAP_ATTRIBUTE_GROUP_NAME:cn}
```

上面这份 properties 是“可运行示例”，不是“生产默认值模板”。

这份文件的职责是：

- 配置 Flowable UI 自己使用的数据库
- 配置 Flowable UI 自己使用的 LDAP identity source
- 支撑在 UI 中选择用户、候选用户、候选组、固定办理人

也就是说，标准主路线下：

- Flowable 平台 LDAP 配在这里
- 不是要求每个业务系统默认再额外配一套运行时目录能力

第一次照着抄时，至少要主动确认下面几项：

1. `host.docker.internal`
   - 这个写法适合 Docker Desktop 场景
   - 如果你的 Docker 环境不支持 `host.docker.internal`，需要改成宿主机可达地址
2. `admin / test`
   - 只是本地演示示例，不应直接作为长期环境口令
3. `ldap.example.com / change_me`
   - 只是占位值，必须换成真实 LDAP 参数

### 2.3 `flowable-rest.properties` 完整示例

```properties
server.port=8080
server.servlet.context-path=/flowable-rest
spring.application.name=flowable-rest
spring.jmx.default-domain=${spring.application.name}
flowable.rest.app.role-prefix=

# MySQL
spring.datasource.driver-class-name=com.mysql.cj.jdbc.Driver
spring.datasource.url=${SPRING_DATASOURCE_URL:jdbc:mysql://host.docker.internal:3307/flowable_681_app?characterEncoding=UTF-8&useSSL=false&allowPublicKeyRetrieval=true}
spring.datasource.username=${SPRING_DATASOURCE_USERNAME:flowable_app}
spring.datasource.password=${SPRING_DATASOURCE_PASSWORD:flowable}
spring.datasource.hikari.maximumPoolSize=${SPRING_DATASOURCE_HIKARI_MAXIMUM_POOL_SIZE:20}
flowable.database-schema=${FLOWABLE_DATABASE_SCHEMA:flowable_681_app}
flowable.use-lock-for-database-schema-update=true

# Engine
flowable.process.servlet.path=/service
flowable.database-schema-update=${FLOWABLE_REST_DATABASE_SCHEMA_UPDATE:true}
flowable.async-executor-activate=${FLOWABLE_REST_ASYNC_EXECUTOR_ACTIVATE:true}
flowable.history-level=${FLOWABLE_REST_HISTORY_LEVEL:full}

# REST
flowable.rest.app.authentication-mode=verify-privilege
flowable.rest.app.swagger-docs-enabled=true
flowable.rest.app.create-demo-definitions=false

# REST WAR in this suite uses service account access.
# LDAP settings should be configured on Flowable UI; this REST WAR does not include
# the LDAP dependency module, so keeping LDAP config here would be misleading.
flowable.rest.app.admin.user-id=${FLOWABLE_REST_APP_ADMIN_USER_ID:rest-admin}
flowable.rest.app.admin.password=${FLOWABLE_REST_APP_ADMIN_PASSWORD:test}
flowable.rest.app.admin.firstname=${FLOWABLE_REST_APP_ADMIN_FIRSTNAME:Rest}
flowable.rest.app.admin.lastname=${FLOWABLE_REST_APP_ADMIN_LASTNAME:Admin}
```

同样提醒：

- `rest-admin / test`
  - 是本地演示示例账号
  - 不是生产推荐值
- `flowable.history-level=full`
  - 是当前 go-common 工作流能力推荐值
  - 因为待办、已办、步骤、办理记录、时间线等能力依赖完整历史信息

这份文件的职责是：

- 配置 Flowable REST 自己使用的数据库
- 暴露运行时流程接口给业务系统调用

业务系统最小接入时，通常只需要知道：

- `workflow.flowable.base_url`
- `workflow.callback.path`
- `workflow.callback.public_base_url`

不需要默认知道 Flowable 平台内部 LDAP 或数据库的具体实现细节。

---

## 3. 本地全量测试推荐模式

本地全量测试推荐：

- Flowable UI：LDAP 登录
- Flowable REST：`rest-admin/test`
- 业务系统：通过 `go-common/workflow` 调用标准 workflow API

标准主路线下，本地全量测试推荐分成两部分：

1. 平台侧验证
   - Flowable UI 能登录
   - Flowable UI 能看到 LDAP 用户/组
   - Flowable REST 能正常响应
2. 业务侧验证
   - 业务保存
   - 流程启动
   - 待办处理
   - callback 回写

建议实际联调顺序也按下面走：

1. 先确认 Flowable UI 能登录，且能看到 LDAP 用户和组
2. 再确认 Flowable REST 能正常响应
3. 再启动业务系统，通过 go-common 走标准接口发起和处理流程

不要一上来就先开业务系统侧 `directory / assignment`，否则很容易把“平台主路线问题”和“兼容路线问题”混在一起排查。

注意：

- Flowable UI 登录方式和业务系统 `workflow.directory.provider` 不是一回事
- Flowable UI 只负责画流程图、发布流程定义
- 业务系统最小接入默认不要求先启用 `workflow.directory.provider=ldap`
- 当前代码里与业务侧 LDAP / assignment 相关的能力仍然存在，但已经降级为历史兼容路线

为了防止第一次接入时概念混掉，这里再拆开说一次：

1. Flowable UI 用 LDAP 登录
   - 解决的是“谁能登录 Flowable UI、谁能在 UI 里选人选组”
2. Flowable REST 用 `rest-admin` 服务账号
   - 解决的是“业务系统调用引擎运行时接口时，用什么账号访问 REST”
3. 业务系统最小接入
   - 默认只需要通过 `go-common` 调用 Flowable REST
4. 业务系统 `workflow.directory.provider=ldap / workflow.assignment.provider=directory`
   - 属于历史兼容路线
   - 解决的是“业务系统运行时自己查 LDAP、自己算审批人”

这四件事是相关的，但不是同一个概念。

第一次本地启动时，最容易混掉的其实是下面 4 类地址：

1. Flowable UI 地址
   - 给人登录、建模、发布流程定义用
   - 例如：`http://127.0.0.1:8086/flowable-ui/`
2. Flowable REST 地址
   - 给业务系统或 go-common 调运行时接口用
   - 例如：`http://127.0.0.1:8087/flowable-rest/service`
3. 业务系统自己的服务地址
   - 给前端或其他业务接口调用用
   - 例如：`http://127.0.0.1:9080`
4. Flowable 回调业务系统时使用的外部地址
   - 对应业务系统里的 `workflow.callback.public_base_url`
   - 本地同机联调时可以和业务系统服务地址一样
   - 远端 Flowable 联调时，通常要写成公网、网关或 FRP 可达地址

直接记一条最实用的规则：

- 人登录 Flowable，用 UI 地址
- 程序调用 Flowable，用 REST 地址
- Flowable 回调业务系统，用 `workflow.callback.public_base_url`

不要把这三类地址混成一个字段，也不要把 UI 地址误填到 `workflow.flowable.base_url` 里。

### 3.1 业务系统最小配置示例

如果你的目标只是让业务系统通过 `go-common` 接入当前这套 Flowable 平台，最小配置通常先写成这样：

```yaml
workflow:
  api:
    enabled: true
    prefix: "/api/v1/workflow"
    sso_enabled: true
  callback:
    path: "/api/v1/workflow/callback"
    secret: ""
    public_base_url: "http://127.0.0.1:9080"
  flowable:
    base_url: "http://127.0.0.1:8087/flowable-rest/service"
    username: "rest-admin"
    password: "test"
```

这 3 组配置分别管什么：

- `workflow.flowable.base_url`
  - 业务系统调用 Flowable REST 的地址
  - 应该指向 `flowable-rest/service`
- `workflow.callback.path`
  - 业务系统自己暴露的标准 callback 路径
  - 推荐固定为 `/api/v1/workflow/callback`
- `workflow.callback.public_base_url`
  - Flowable 运行时回调业务系统时使用的外部可达基地址
  - 本地同机联调时，可以和业务系统自己的访问地址一样
  - 如果是远端 Flowable 联调，通常要改成公网、网关或 FRP 地址

第一次接入时，先把这 3 件事配对，再去做流程启动和 callback 联调。

### 3.2 业务系统默认不要先配这些

如果你是新项目，或者只是先走标准主路线联调，下面这些能力默认不要一上来就全开：

- `workflow.identity.*`
- `workflow.directory.*`
- `workflow.assignment.*`
- `workflow.formref.db_instance`
- `workflow.records.db_instance`

原因不是这些能力没用，而是它们都属于：

- 历史兼容能力
- 可选增强能力
- 特殊联调或排障能力

第一次接入最容易犯的错，就是先把这些配置全加上，结果把“Flowable 平台没通”和“业务系统兼容能力没配对”混成一个问题。

推荐顺序固定如下：

1. 先确认 Flowable UI 能登录、能建模
2. 再确认 Flowable REST 可调用
3. 再让业务系统只用最小配置跑通“保存业务 + 启动流程 + 待办处理 + callback 回流”
4. 最后才按需增加 `formref` fallback、records 独立库、directory、assignment、identity alias 等增强能力

---

## 4. Flowable UI 本地账号配置

推荐本地配置可以直接参考上面的完整示例文件。

如果你只关心“UI 本地账号模式”，最关键的是下面这些项：

```properties
server.port=8080
server.servlet.context-path=/flowable-ui

flowable.common.app.idm-admin.user=admin
flowable.common.app.idm-admin.password=test

flowable.idm.ldap.enabled=false
```

登录地址：

```text
http://127.0.0.1:8086/flowable-ui/
```

账号：

```text
admin / test
```

命令行验证登录：

```bash
curl -i -sS \
  -c /tmp/flowable-ui-cookie.txt \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -X POST \
  http://127.0.0.1:8086/flowable-ui/app/authentication \
  --data 'j_username=admin&j_password=test&_spring_security_remember_me=true&submit=Login'

curl -i -sS \
  -b /tmp/flowable-ui-cookie.txt \
  http://127.0.0.1:8086/flowable-ui/app/rest/account
```

如果第二个命令返回 `id=admin` 且包含 `access-modeler`，说明 UI 本地登录可用。

常见问题：

- 访问 `http://127.0.0.1:8086` 返回 `404` 是正常的
- 正确地址必须带 `/flowable-ui/`
- 如果登录提示“无效凭证”，优先检查 `flowable.idm.ldap.enabled` 是否仍为 `true`

---

## 5. Flowable REST 本地账号配置

推荐本地配置可以直接参考上面的完整示例文件。

如果你只关心“REST 服务账号模式”，最关键的是下面这些项：

```properties
server.port=8080
server.servlet.context-path=/flowable-rest

flowable.process.servlet.path=/service

flowable.rest.app.admin.user-id=rest-admin
flowable.rest.app.admin.password=test
flowable.idm.ldap.enabled=false
```

REST 根地址：

```text
http://127.0.0.1:8087/flowable-rest/service
```

账号：

```text
rest-admin / test
```

验证命令：

```bash
curl -u rest-admin:test \
  http://127.0.0.1:8087/flowable-rest/service/management/engine
```

---

## 6. 启动命令

进入 compose 所在目录：

```bash
cd <flowable-adapter-directory>
```

首次启动或配置变更后启动：

```bash
docker compose -f compose.yaml up -d --build mysql flowable-rest flowable-ui
```

只启动 UI：

```bash
docker compose -f compose.yaml up -d --build flowable-ui
```

只启动 REST：

```bash
docker compose -f compose.yaml up -d --build flowable-rest
```

查看状态：

```bash
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
```

查看 UI 日志：

```bash
docker compose logs --tail 120 flowable-ui
```

查看 REST 日志：

```bash
docker compose logs --tail 120 flowable-rest
```

---

## 7. 为什么修改配置后必须重新 build

当前 Dockerfile 是这样工作的：

```dockerfile
COPY ui/flowable-ui.properties /opt/flowable/config/application.properties
COPY rest/flowable-rest.properties /opt/flowable/config/application.properties
```

也就是说：

- 配置文件是在 build 镜像时复制进去的
- 容器启动时不会自动读取宿主机上的最新 properties
- 所以只改宿主机文件后，直接 `docker restart` 不会生效

正确做法是：

```bash
docker compose -f compose.yaml up -d --build flowable-ui flowable-rest
```

---

## 8. 如何切换为 UI 侧 LDAP 模式

当前这套官方部署包里：
- `flowable-ui.war` 已包含 `flowable-ldap`
- `flowable-rest.war` 未打入 `flowable-ldap`

因此可稳定支持的是：
- Flowable UI：LDAP 登录
- Flowable REST：服务账号 `rest-admin / test`

这也是当前公司标准主路线推荐的 Flowable 平台模式：

- Flowable 平台自己连接 LDAP
- 在 UI 中直接完成用户、候选用户、候选组配置
- 业务系统默认不再承担 Flowable identity 职责

如果需要验证 **UI 侧 LDAP 登录**，请保持下面配置为 `true`：

```properties
flowable.idm.ldap.enabled=true
```

并确认这些配置有效：

```properties
flowable.idm.ldap.server=ldap://113.45.4.22
flowable.idm.ldap.port=8389
flowable.idm.ldap.user=cn=admin,dc=msss,dc=com
flowable.idm.ldap.password=你的LDAP绑定密码
flowable.idm.ldap.base-dn=dc=msss,dc=com
flowable.idm.ldap.user-base-dn=ou=people,dc=msss,dc=com
flowable.idm.ldap.group-base-dn=ou=groups,dc=msss,dc=com
```

`group-base-dn` 要指向业务系统同步生成的“流程角色组”LDAP OU。当前推荐使用 `ou=groups`，不要再指向 `ou=departments`，否则用户能登录但组列表可能为空或一直加载。

公司标准建议：

- `ou=groups` 里只放流程角色组，例如 `wfgrp:<tenantCode>:<groupCode>`
- 不要再把部门组、岗位组暴露给 Flowable 作为候选组
- `candidateGroups` 只配置这类流程角色组，避免把组织投影误当成流程办理组

切换后同样必须重新 build：

```bash
docker compose -f compose.yaml up -d --build flowable-ui
```

如果日志里出现：

```text
LDAP: error code 49 - Invalid Credentials
```

说明 LDAP 绑定账号或密码不正确。

切换成功后，建议至少再验证下面 4 件事：

1. UI 中可以搜索到 LDAP 用户
2. UI 中可以搜索到 LDAP 组
3. BPMN 节点可以直接选到目标用户/组
4. 业务系统通过 `go-common` 启动流程后，待办能落到预期 LDAP 用户/组

---

## 9. 与业务系统 LDAP 目录能力的关系

这里必须区分两种 LDAP 关系。

第 1 种：Flowable 平台 LDAP

- 用于 Flowable UI 登录
- 用于 Flowable UI 中选择用户、候选用户、候选组
- 用于 Flowable 平台运行时识别 candidate user / candidate group

这就是当前公司标准主路线。

第 2 种：业务系统侧 LDAP

- 对应 `workflow.directory.*`
- 对应 `workflow.assignment.*`
- 对应业务系统运行时自己查 LDAP、自己算审批人的兼容路线

这条路线当前代码里仍然存在，但已经降级为：

- 历史兼容能力
- 特殊迁移场景能力

不是新项目标准主路径。

因此推荐理解如下：

1. 标准主路线
   - Flowable 平台自己连接 LDAP
   - 业务系统默认只需要通过 `go-common` 调 Flowable REST
2. 历史兼容路线
   - 业务系统自己也连 LDAP
   - 自己做目录查询或运行时分配
   - 仅在已有项目迁移或特殊兼容场景下使用

如果两种 LDAP 同时存在，也要明确职责边界：

- Flowable 平台 LDAP
  - 负责身份源、组源、节点配置来源
- 业务系统侧 LDAP
  - 负责兼容目录查询或特殊业务辅助能力

不要混成一个概念。

---

## 10. 常见排查

### 10.1 `http://127.0.0.1:8086` 返回 404

正常。

正确地址是：

```text
http://127.0.0.1:8086/flowable-ui/
```

### 10.2 UI 登录提示无效凭证

优先检查：

- `flowable.idm.ldap.enabled` 是否是 `false`
- 修改配置后是否重新 `--build`
- 是否访问了正确路径 `/flowable-ui/`

### 10.3 REST 请求 401

优先检查：

- Basic Auth 是否是 `rest-admin:test`
- REST 地址是否带 `/flowable-rest/service`

### 10.4 后端提示 Flowable REST 连接失败

优先检查业务系统配置：

```yaml
workflow:
  flowable:
    base_url: "http://localhost:8087/flowable-rest/service"
    username: "rest-admin"
    password: "test"
```

以及验证命令：

```bash
curl -u rest-admin:test \
  http://127.0.0.1:8087/flowable-rest/service/management/engine
```

### 10.5 callback 路由明明注册了，但就是收不到回调

先不要急着怀疑 go-common 路由没生效，优先按下面顺序查：

1. 业务系统是否真的注册了标准路径
   - `POST /api/v1/workflow/callback`
2. 启动流程时是否真的传了：
   - `variables.callbackUrl`
   - `variables.callbackEvents`
3. `callbackUrl` 是否是 Flowable 所在环境能访问到的地址
   - 本地同机联调时，常见是 `http://127.0.0.1:9080/api/v1/workflow/callback`
   - 远端 Flowable 联调时，通常不能再写本机回环地址
4. `workflow.callback.public_base_url` 是否写对
   - 它应该是 Flowable 访问业务系统时看到的外部地址
   - 不是 Flowable UI 地址
   - 也不是 Flowable REST 地址

一句话记忆：

- `workflow.callback.path`
  - 决定“业务系统把 callback 暴露在哪里”
- `variables.callbackUrl`
  - 决定“Flowable 实际回调哪个完整地址”

如果你想更系统地看 callback 的构造方式、签名规则、业务扩展方式，请直接去看《工作流平台接入总说明.md》里的 callback 章节。
