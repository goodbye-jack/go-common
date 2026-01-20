# CHANGELOG
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
