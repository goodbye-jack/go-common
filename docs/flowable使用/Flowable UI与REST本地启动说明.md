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

## 1. 文档目标

本文说明本地全量联调时，如何启动 Flowable 6.8.1 的三个依赖服务：

- Flowable MySQL
- Flowable UI
- Flowable REST

同时说明：

- UI 和 REST 使用本地账号登录时要改哪些配置
- 如果切换回 LDAP 要怎么改
- 为什么修改配置后必须重新 build 镜像
- 业务系统使用 LDAP 目录能力时，与 Flowable UI/REST 登录方式是什么关系

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

---

## 3. 本地全量测试推荐模式

本地全量测试推荐：

- Flowable UI：LDAP 登录
- Flowable REST：`rest-admin/test`
- 业务系统目录与自动分配：LDAP / directory provider

也就是：

```yaml
workflow:
  directory:
    provider: "ldap"
  assignment:
    provider: "directory"
```

注意：

- Flowable UI 登录方式和业务系统 `workflow.directory.provider` 不是一回事
- Flowable UI 只负责画流程图、发布流程定义
- 业务系统运行时的“查人、查部门、找下一环节办理人”由 LDAP / directory provider 负责
- 所以本地测试时，UI 与业务系统都应保持 LDAP 主链路一致

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

`group-base-dn` 要指向 `go-common` / 业务同步生成的通用 LDAP Group 投影 OU。当前推荐使用 `ou=groups`，不要再指向 `ou=departments`，否则用户能登录但组列表可能为空或一直加载。

切换后同样必须重新 build：

```bash
docker compose -f compose.yaml up -d --build flowable-ui
```

如果日志里出现：

```text
LDAP: error code 49 - Invalid Credentials
```

说明 LDAP 绑定账号或密码不正确。

---

## 9. 与业务系统 LDAP 目录能力的关系

业务系统配置：

```yaml
workflow:
  directory:
    provider: "ldap"
  assignment:
    provider: "directory"
```

这表示：

- `go-common` 标准接口运行时通过 LDAP 查询目录关系
- `go-common` 标准接口运行时通过 LDAP / directory provider 计算下一环节办理人
- Flowable UI 的登录与选人走 LDAP；业务系统流程调用 Flowable REST 走服务账号
- Flowable UI 的组查询走 `ou=groups`；业务系统的上级、部门、岗位查询仍分别走用户、部门、岗位 OU

本地全量测试推荐链路：

1. Flowable UI 使用 LDAP 账号登录
2. 在 UI 中修改并发布 BPMN
3. Flowable REST 使用 `rest-admin/test`
4. `relics-protect-backend` 保持 `workflow.directory.provider=ldap`
5. `relics-protect-backend` 保持 `workflow.assignment.provider=directory`
6. 通过业务系统接口发起流程、查看待办、完成任务、查看已办和进度

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
