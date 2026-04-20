# Flowable UI 与 REST 本地启动说明

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

## 2. 当前本地目录

本地 compose 文件：

```text
/usr/local/work/workspace/dev_personal_project/zhuoue-flowable-6.8.1-adapter/compose.yaml
```

Flowable UI 配置：

```text
/usr/local/work/workspace/dev_personal_project/zhuoue-flowable-6.8.1-suite/deploy/ui/flowable-ui.properties
```

Flowable REST 配置：

```text
/usr/local/work/workspace/dev_personal_project/zhuoue-flowable-6.8.1-suite/deploy/rest/flowable-rest.properties
```

说明：

- `compose.yaml` 在 adapter 项目中
- UI / REST 的 war 包和配置在 suite 项目中
- Dockerfile 会把 `*.properties` 复制进镜像，所以修改配置后必须重新 build 对应镜像

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

文件：

```text
/usr/local/work/workspace/dev_personal_project/zhuoue-flowable-6.8.1-suite/deploy/ui/flowable-ui.properties
```

推荐本地配置：

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

文件：

```text
/usr/local/work/workspace/dev_personal_project/zhuoue-flowable-6.8.1-suite/deploy/rest/flowable-rest.properties
```

推荐本地配置：

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
cd /usr/local/work/workspace/dev_personal_project/zhuoue-flowable-6.8.1-adapter
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
docker logs --tail 120 zhuoue-flowable-681-adapter-flowable-ui-1
```

查看 REST 日志：

```bash
docker logs --tail 120 zhuoue-flowable-681-adapter-flowable-rest-1
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
