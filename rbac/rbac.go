package rbac

import (
	"errors"
	"fmt"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	redisadapter "github.com/casbin/redis-adapter/v3"
	rediswatcher "github.com/casbin/redis-watcher/v2"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/orm"
	"github.com/redis/go-redis/v9"
	"strings"
)

type Req struct{ dom, sub, obj, act string }
type Policy interface{ ToArr() []string }
type ActionPolicy struct{ Dom, Sub, Obj, Act string }
type TenantPolicy struct{ ten, dom string }
type RolePolicy struct{ User, Role string }
type RbacClient struct {
	e *casbin.Enforcer
	w persist.Watcher
}

var rbacClient *RbacClient = nil

const text = `
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && r.dom == p.dom && r.obj == p.obj && r.act == p.act
`

// NewRbacClient 最终简化版：直接读取Config原始参数，无冗余方法
func NewRbacClient(redisAddrOpt ...string) *RbacClient {
	if rbacClient != nil {
		return rbacClient
	}

	// ========== 步骤1：从orm.Redis读取原始配置参数（无冗余封装） ==========
	var (
		redisAddr     string // host:port
		redisPassword string
		redisDB       int
	)

	if orm.Redis != nil && orm.Redis.GetConfig() != nil {
		cfg := orm.Redis.GetConfig()
		// 直接读取原始参数，无需封装方法
		redisAddr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		redisPassword = cfg.Password
		// 解析DB索引（兼容字符串/数字配置）
		fmt.Sscanf(cfg.Database, "%d", &redisDB)
		log.Info("NewRbacClient: 使用orm.Redis配置 | addr=%s | db=%d", redisAddr, redisDB)
	}

	// ========== 步骤2：降级逻辑 ==========
	if redisAddr == "" || redisAddr == ":0" {
		if len(redisAddrOpt) > 0 && redisAddrOpt[0] != "" {
			redisAddr = redisAddrOpt[0]
		} else {
			redisAddr = "127.0.0.1:6379"
		}
		redisDB = 0
		log.Info("NewRbacClient: 使用降级配置 | addr=%s | db=%d", redisAddr, redisDB)
	}

	// ========== 步骤3：构造casbin-adapter兼容的DSN ==========
	var dsnBuilder strings.Builder
	dsnBuilder.WriteString(redisAddr)
	params := []string{}
	if redisPassword != "" {
		params = append(params, fmt.Sprintf("password=%s", redisPassword))
	}
	if redisDB != 0 {
		params = append(params, fmt.Sprintf("db=%d", redisDB))
	}
	if len(params) > 0 {
		dsnBuilder.WriteString("?")
		dsnBuilder.WriteString(strings.Join(params, "&"))
	}
	finalDSN := dsnBuilder.String()

	// ========== 步骤4：初始化Adapter（基础API，零报错） ==========
	adapter, err := redisadapter.NewAdapter("tcp", finalDSN)
	if err != nil {
		log.Fatalf("NewAdapter失败: %v | DSN: %s", err, finalDSN)
	}

	// ========== 后续步骤（无修改） ==========
	m, err := model.NewModelFromString(text)
	if err != nil {
		log.Fatalf("NewModelFromString失败: %v", err)
	}

	e, err := casbin.NewEnforcer(m, adapter)
	if err != nil {
		log.Fatalf("NewEnforcer失败: %v", err)
	}
	if err := e.LoadPolicy(); err != nil {
		log.Fatalf("LoadPolicy失败: %v", err)
	}

	watcher, err := rediswatcher.NewWatcher(redisAddr, rediswatcher.WatcherOptions{
		Options: redis.Options{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       redisDB,
		},
		Channel:    "/casbin",
		IgnoreSelf: true,
	})
	if err != nil {
		log.Fatalf("NewWatcher失败: %v", err)
	}

	if err := e.SetWatcher(watcher); err != nil {
		log.Fatalf("SetWatcher失败: %v", err)
	}
	if err := watcher.SetUpdateCallback(func(string) {
		_ = e.LoadPolicy()
	}); err != nil {
		log.Fatalf("SetUpdateCallback失败: %v", err)
	}

	rbacClient = &RbacClient{
		e: e,
		w: watcher,
	}
	log.Info("RBAC客户端初始化成功 | Redis地址: %s", redisAddr)
	return rbacClient
}

//// NewRbacClient 终极兼容版：适配所有casbin-redis-adapter/v3版本 + 零报错
//func NewRbacClient(redisAddrOpt ...string) *RbacClient {
//	if rbacClient != nil {
//		return rbacClient
//	}
//	// ========== 步骤1：从orm.Redis读取配置（仅取基础参数） ==========
//	var (
//		redisAddr     string // 纯host:port（如127.0.0.1:6379）
//		redisPassword string // 密码（空则无）
//		redisDB       int    // DB索引（默认0）
//	)
//	// 仅调用你已实现的导出方法，无任何未定义API
//	if orm.Redis != nil {
//		redisAddr = orm.Redis.GetAddr()
//		redisPassword = orm.Redis.GetPassword()
//		redisDB = orm.Redis.GetConfig().DB
//		log.Info("NewRbacClient: 使用orm.Redis配置 | addr=%s | db=%d", redisAddr, redisDB)
//	}
//	// ========== 步骤2：降级逻辑（兜底默认值） ==========
//	if redisAddr == "" || redisAddr == ":0" {
//		if len(redisAddrOpt) > 0 && redisAddrOpt[0] != "" {
//			redisAddr = redisAddrOpt[0]
//		} else {
//			redisAddr = "127.0.0.1:6379"
//		}
//		redisDB = 0
//		log.Info("NewRbacClient: 使用降级配置 | addr=%s | db=%d", redisAddr, redisDB)
//	}
//	// ========== 步骤3：核心兼容方案 - 直接构造DSN（适配v3所有版本） ==========
//	// 构造casbin-redis-adapter兼容的DSN格式（无协议前缀，仅host:port?参数）
//	var dsnBuilder strings.Builder
//	dsnBuilder.WriteString(redisAddr) // 先写host:port
//	// 添加参数（密码/DB）：用?拼接，适配底层解析逻辑
//	params := []string{}
//	if redisPassword != "" {
//		params = append(params, fmt.Sprintf("password=%s", redisPassword))
//	}
//	if redisDB != 0 {
//		params = append(params, fmt.Sprintf("db=%d", redisDB))
//	}
//	if len(params) > 0 {
//		dsnBuilder.WriteString("?")
//		dsnBuilder.WriteString(strings.Join(params, "&"))
//	}
//	finalDSN := dsnBuilder.String()
//	// ========== 步骤4：初始化Casbin Redis Adapter（基础API，兼容所有v3版本） ==========
//	// 核心：仅用NewAdapter基础方法，DSN格式为 "host:port?password=xxx&db=0"
//	adapter, err := redisadapter.NewAdapter("tcp", finalDSN)
//	if err != nil {
//		log.Fatalf("NewAdapter失败: %v | DSN: %s", err, finalDSN)
//	}
//	// ========== 步骤5：初始化Casbin模型（无修改） ==========
//	m, err := model.NewModelFromString(text)
//	if err != nil {
//		log.Fatalf("NewModelFromString失败: %v", err)
//	}
//	// ========== 步骤6：初始化Enforcer（无修改） ==========
//	e, err := casbin.NewEnforcer(m, adapter)
//	if err != nil {
//		log.Fatalf("NewEnforcer失败: %v", err)
//	}
//	if err := e.LoadPolicy(); err != nil {
//		log.Fatalf("LoadPolicy失败: %v", err)
//	}
//	// ========== 步骤7：初始化Watcher（基础API） ==========
//	watcher, err := rediswatcher.NewWatcher(redisAddr, rediswatcher.WatcherOptions{
//		Options: redis.Options{
//			Addr:     redisAddr,
//			Password: redisPassword,
//			DB:       redisDB,
//		},
//		Channel:    "/casbin",
//		IgnoreSelf: true,
//	})
//	if err != nil {
//		log.Fatalf("NewWatcher失败: %v", err)
//	}
//	// ========== 步骤8：设置Watcher回调（无修改） ==========
//	if err := e.SetWatcher(watcher); err != nil {
//		log.Fatalf("SetWatcher失败: %v", err)
//	}
//	if err := watcher.SetUpdateCallback(func(string) {
//		_ = e.LoadPolicy()
//	}); err != nil {
//		log.Fatalf("SetUpdateCallback失败: %v", err)
//	}
//	// ========== 步骤9：初始化RBAC客户端 ==========
//	rbacClient = &RbacClient{
//		e: e,
//		w: watcher,
//	}
//	log.Info("RBAC客户端初始化成功 | Redis地址: %s", redisAddr)
//	return rbacClient
//}

//// NewRbacClient 终版：适配casbin-redis-adapter/v3官方API + 零报错
//func NewRbacClient(redisAddrOpt ...string) *RbacClient {
//	if rbacClient != nil {
//		return rbacClient
//	}
//
//	// ========== 步骤1：从orm.Redis读取配置（仅取host:port/密码/DB） ==========
//	var (
//		redisAddr     string // 纯host:port格式（如127.0.0.1:6379）
//		redisPassword string // 密码
//		redisDB       int    // DB索引
//	)
//
//	// 读取配置（仅调用你已实现的导出方法，无未定义API）
//	if orm.Redis != nil {
//		redisAddr = orm.Redis.GetAddr() // 确保返回host:port
//		redisPassword = orm.Redis.GetPassword()
//		redisDB = orm.Redis.GetConfig().DB // 确保返回int类型DB索引
//		log.Info("NewRbacClient: 使用orm.Redis配置 | addr=%s | db=%d", redisAddr, redisDB)
//	}
//
//	// ========== 步骤2：降级逻辑（兜底默认值） ==========
//	if redisAddr == "" || redisAddr == ":0" {
//		if len(redisAddrOpt) > 0 && redisAddrOpt[0] != "" {
//			redisAddr = redisAddrOpt[0]
//		} else {
//			redisAddr = "127.0.0.1:6379"
//		}
//		redisDB = 0
//		log.Info("NewRbacClient: 使用降级配置 | addr=%s | db=%d", redisAddr, redisDB)
//	}
//
//	// ========== 步骤3：初始化Redis客户端（适配go-redis/v9官方API） ==========
//	// 手动创建Redis客户端（绕开casbin-adapter的参数限制）
//	redisClient := redis.NewClient(&redis.Options{
//		Addr:     redisAddr,     // 仅host:port，无协议前缀
//		Password: redisPassword, // 密码（空则无认证）
//		DB:       redisDB,       // DB索引
//	})
//
//	// ========== 步骤4：初始化Casbin Redis Adapter（适配v3官方API） ==========
//	// 核心修复：v3版本正确用法 - 先NewAdapterWithClient传入自定义客户端
//	adapter, err := redisadapter.NewAdapterWithClient(redisClient)
//	if err != nil {
//		log.Fatalf("NewAdapterWithClient失败: %v", err)
//	}
//
//	// ========== 步骤5：初始化Casbin模型（无修改） ==========
//	m, err := model.NewModelFromString(text)
//	if err != nil {
//		log.Fatalf("NewModelFromString失败: %v", err)
//	}
//
//	// ========== 步骤6：初始化Enforcer（无修改） ==========
//	e, err := casbin.NewEnforcer(m, adapter)
//	if err != nil {
//		log.Fatalf("NewEnforcer失败: %v", err)
//	}
//	if err := e.LoadPolicy(); err != nil {
//		log.Fatalf("LoadPolicy失败: %v", err)
//	}
//
//	// ========== 步骤7：初始化Watcher（适配官方API） ==========
//	watcher, err := rediswatcher.NewWatcher(redisAddr, rediswatcher.WatcherOptions{
//		Options: redis.Options{
//			Addr:     redisAddr,
//			Password: redisPassword,
//			DB:       redisDB,
//		},
//		Channel:    "/casbin",
//		IgnoreSelf: true,
//	})
//	if err != nil {
//		log.Fatalf("NewWatcher失败: %v", err)
//	}
//
//	// ========== 步骤8：设置Watcher回调（无修改） ==========
//	if err := e.SetWatcher(watcher); err != nil {
//		log.Fatalf("SetWatcher失败: %v", err)
//	}
//	if err := watcher.SetUpdateCallback(func(string) {
//		_ = e.LoadPolicy()
//	}); err != nil {
//		log.Fatalf("SetUpdateCallback失败: %v", err)
//	}
//
//	// ========== 步骤9：初始化RBAC客户端 ==========
//	rbacClient = &RbacClient{
//		e: e,
//		w: watcher,
//	}
//	log.Info("RBAC客户端初始化成功 | Redis地址: %s", redisAddr)
//	return rbacClient
//}

//// NewRbacClient 最终适配版：复用通用DSN逻辑 + 兼容集群模式
//func NewRbacClient(redisAddrOpt ...string) *RbacClient {
//	if rbacClient != nil {
//		return rbacClient
//	}
//	// ========== 步骤1：从orm.Redis读取配置（复用通用Config） ==========
//	var (
//		redisAddr     string          // 最终地址（集群用逗号分隔）
//		redisPassword string          // 密码
//		redisDB       int             // DB索引
//		redisMode     dbconfig.DBMode // 部署模式（single/cluster）
//	)
//
//	// 直接调用导出方法读取配置 + 复用通用Config的模式字段
//	if orm.Redis != nil {
//		redisAddr = orm.Redis.GetAddr()
//		redisPassword = orm.Redis.GetPassword()
//		redisDB = orm.Redis.GetConfig().DB
//		// 新增：读取Redis部署模式（集群/单机）
//		if cfg := orm.Redis.GetConfig(); cfg != nil {
//			redisMode = cfg.Mode
//		}
//		log.Info("NewRbacClient: 使用orm.Redis配置, addr=%s, mode=%s, db=%d", redisAddr, redisMode, redisDB)
//	}
//
//	// ========== 步骤2：降级逻辑（传入参数 → 默认值） ==========
//	if redisAddr == "" || redisAddr == ":0" {
//		if len(redisAddrOpt) > 0 && redisAddrOpt[0] != "" {
//			redisAddr = redisAddrOpt[0]
//		} else {
//			redisAddr = "127.0.0.1:6379"
//		}
//		redisDB = 0
//		redisMode = dbconfig.DBModeSingle // 默认单机
//		log.Info("NewRbacClient: 使用降级配置, addr=%s, mode=%s", redisAddr, redisMode)
//	}
//
//	// ========== 步骤3：初始化Casbin Adapter（复用通用DSN逻辑 + 兼容集群） ==========
//	var adapter persist.Adapter
//	var adapterDSN string
//
//	// 核心调整：复用你genRedisDSN的集群/单机DSN拼接逻辑
//	if redisMode == dbconfig.DBModeCluster && strings.Contains(redisAddr, ",") {
//		// 集群模式：适配genRedisDSN的集群DSN格式
//		adapterDSN = fmt.Sprintf("redis://%s?db=%d", redisAddr, redisDB)
//		if redisPassword != "" {
//			adapterDSN = fmt.Sprintf("redis://:%s@%s?db=%d", redisPassword, redisAddr, redisDB)
//		}
//	} else {
//		// 单机模式：原有逻辑（与genRedisDSN对齐）
//		adapterDSN = fmt.Sprintf("redis://%s/%d", redisAddr, redisDB)
//		if redisPassword != "" {
//			adapterDSN = fmt.Sprintf("redis://:%s@%s/%d", redisPassword, redisAddr, redisDB)
//		}
//	}
//
//	// 核心：仅传2个参数，解决Too many arguments报错（兼容低版本）
//	adapter, err := redisadapter.NewAdapter("tcp", adapterDSN)
//	if err != nil {
//		log.Fatalf("NewAdapter error: %v | DSN: %s", err, adapterDSN)
//	}
//
//	// ========== 步骤4：初始化Casbin模型（无修改） ==========
//	m, err := model.NewModelFromString(text)
//	if err != nil {
//		log.Fatalf("NewModelFromString error: %v", err)
//	}
//
//	// ========== 步骤5：初始化Enforcer（无修改） ==========
//	e, err := casbin.NewEnforcer(m, adapter)
//	if err != nil {
//		log.Fatalf("NewEnforcer error: %v", err)
//	}
//	if err := e.LoadPolicy(); err != nil {
//		log.Fatalf("LoadPolicy error: %v", err)
//	}
//
//	// ========== 步骤6：初始化Watcher（适配集群模式） ==========
//	var w persist.Watcher
//	watcherOpts := rediswatcher.WatcherOptions{
//		Options: redis.Options{
//			Addr:     redisAddr,
//			Password: redisPassword,
//			DB:       redisDB,
//			Network:  "tcp",
//		},
//		Channel:    "/casbin",
//		IgnoreSelf: true,
//	}
//
//	// 新增：集群模式下Watcher适配（可选，低版本可能不支持，保留原有逻辑即可）
//	if redisMode == dbconfig.DBModeCluster && strings.Contains(redisAddr, ",") {
//		log.Warn("Redis集群模式下，casbin redis-watcher可能存在兼容性问题，建议使用单机Redis存储策略")
//	}
//	w, err = rediswatcher.NewWatcher(redisAddr, watcherOpts)
//	if err != nil {
//		log.Fatalf("NewWatcher error: %v", err)
//	}
//
//	// ========== 步骤7：设置Watcher回调（无修改） ==========
//	if err := e.SetWatcher(w); err != nil {
//		log.Fatalf("SetWatcher error: %v", err)
//	}
//	if err := w.SetUpdateCallback(func(string) {
//		_ = e.LoadPolicy()
//	}); err != nil {
//		log.Fatalf("SetUpdateCallback error: %v", err)
//	}
//
//	// ========== 步骤8：初始化RBAC客户端（无修改） ==========
//	rbacClient = &RbacClient{w: w, e: e}
//	log.Info("RBAC客户端初始化成功（适配通用DSN + 集群模式）")
//	return rbacClient
//}

//// NewRbacClient 最终最终版：彻底适配你的orm.Redis结构，无任何报错
//func NewRbacClient(redisAddrOpt ...string) *RbacClient {
//	if rbacClient != nil {
//		return rbacClient
//	}
//	// ========== 步骤1：从orm.Redis读取配置（核心修改） ==========
//	var (
//		redisAddr     string // 最终地址
//		redisPassword string // 密码
//		redisDB       int    // DB索引
//	)
//	// 直接调用导出方法读取配置（无任何未导出字段访问）
//	if orm.Redis != nil {
//		redisAddr = orm.Redis.GetAddr()
//		redisPassword = orm.Redis.GetPassword()
//		redisDB = orm.Redis.GetDB()
//		log.Info("NewRbacClient: 使用orm.Redis配置, addr=%s, db=%d", redisAddr, redisDB)
//	}
//	// ========== 步骤2：降级逻辑（传入参数 → 默认值） ==========
//	if redisAddr == "" || redisAddr == ":0" {
//		if len(redisAddrOpt) > 0 && redisAddrOpt[0] != "" {
//			redisAddr = redisAddrOpt[0]
//		} else {
//			redisAddr = "127.0.0.1:6379"
//		}
//		redisDB = 0
//		log.Info("NewRbacClient: 使用降级配置, addr=%s", redisAddr)
//	}
//	// ========== 步骤3：初始化Casbin Adapter（适配低版本，无参数报错） ==========
//	// 拼接DSN：包含密码和DB，仅传2个参数给NewAdapter
//	adapterDSN := fmt.Sprintf("redis://%s/%d", redisAddr, redisDB)
//	if redisPassword != "" {
//		adapterDSN = fmt.Sprintf("redis://:%s@%s/%d", redisPassword, redisAddr, redisDB)
//	}
//	// 核心：仅传2个参数，解决Too many arguments报错
//	adapter, err := redisadapter.NewAdapter("tcp", adapterDSN)
//	if err != nil {
//		log.Fatalf("NewAdapter error: %v", err)
//	}
//	// ========== 步骤4：初始化Casbin模型 ==========
//	m, err := model.NewModelFromString(text)
//	if err != nil {
//		log.Fatalf("NewModelFromString error: %v", err)
//	}
//	// ========== 步骤5：初始化Enforcer ==========
//	e, err := casbin.NewEnforcer(m, adapter)
//	if err != nil {
//		log.Fatalf("NewEnforcer error: %v", err)
//	}
//	if err := e.LoadPolicy(); err != nil {
//		log.Fatalf("LoadPolicy error: %v", err)
//	}
//	// ========== 步骤6：初始化Watcher ==========
//	watcherOpts := rediswatcher.WatcherOptions{
//		Options: redis.Options{
//			Addr:     redisAddr,
//			Password: redisPassword,
//			DB:       redisDB,
//			Network:  "tcp",
//		},
//		Channel:    "/casbin",
//		IgnoreSelf: true,
//	}
//	w, err := rediswatcher.NewWatcher(redisAddr, watcherOpts)
//	if err != nil {
//		log.Fatalf("NewWatcher error: %v", err)
//	}
//	// ========== 步骤7：设置Watcher回调 ==========
//	if err := e.SetWatcher(w); err != nil {
//		log.Fatalf("SetWatcher error: %v", err)
//	}
//	if err := w.SetUpdateCallback(func(string) {
//		_ = e.LoadPolicy()
//	}); err != nil {
//		log.Fatalf("SetUpdateCallback error: %v", err)
//	}
//	// ========== 步骤8：初始化RBAC客户端 ==========
//	rbacClient = &RbacClient{w: w, e: e}
//	log.Info("RBAC客户端初始化成功（无任何报错）")
//	return rbacClient
//}

func NewActionPolicy(dom, sub, obj, act string) Policy {
	return &ActionPolicy{
		Dom: dom,
		Sub: sub,
		Obj: obj,
		Act: act,
	}
}

func (p ActionPolicy) ToArr() []string {
	return []string{p.Sub, p.Dom, p.Obj, p.Act}
}

func NewTenantPolicy(ten, dom string) Policy {
	return &TenantPolicy{
		ten,
		dom,
	}
}

func (p TenantPolicy) ToArr() []string {
	return []string{"p1", p.ten, p.dom}
}

func (p *RolePolicy) ToArr() []string {
	return []string{p.User, p.Role}
}

func NewReq(sub, dom, obj, act string) *Req {
	return &Req{
		dom: dom,
		sub: sub,
		obj: obj,
		act: act,
	}
}

func (r Req) ToArr() []string {
	return []string{r.sub, r.dom, r.obj, r.act}
}

func (c *RbacClient) GetRolePolicy(sub string) (*RolePolicy, error) {
	policies, err := c.e.GetFilteredGroupingPolicy(0, sub)
	if err != nil {
		log.Error("GetRolePolicy/GetFilteredPolicy(0, %s) error, %v", sub, err)
		return nil, err
	}
	if len(policies) == 0 {
		return nil, nil
	}
	if len(policies) > 1 {
		log.Warn("GetRolePolicy/GetFilterPolicy(0, %s) result count %d", sub, len(policies))
	}

	log.Info("GetRolePolicy(%s), result, %+v", sub, policies)
	return &RolePolicy{
		Role: policies[0][1],
		User: policies[0][0],
	}, nil
}

func (c *RbacClient) AddRolePolicy(rp *RolePolicy) error {
	log.Info("AddRolePolicy(%v)", *rp)
	_rp, err := c.GetRolePolicy(rp.User)
	if err != nil {
		log.Error("AddRolePolicy/GetRolePolicy(%v) error, %v", *rp, err)
		return err
	}
	if _rp != nil && _rp.Role != rp.Role { // 如果缓存中的角色和当前传入的角色不一致,那么删除缓存的角色,重新传入新的角色
		log.Info("AddRolePolicy(%v) had existed", *rp)
		if errDR := c.DeleteRolePolicy(_rp); errDR != nil {
			return errDR
		}
	}
	added, err := c.e.AddGroupingPolicy(rp.ToArr())
	if err != nil {
		log.Error("AddRolePolicy/AddGroupingPolicy(%v) error, %v", *rp, err)
		return err
	}
	if added {
		return c.save()
	}
	return nil
}

func (c *RbacClient) UpdateRolePolicy(rp *RolePolicy, role string) error {
	newRp := &RolePolicy{
		User: rp.User,
		Role: role,
	}
	updated, err := c.e.UpdatePolicy(rp.ToArr(), newRp.ToArr())
	if err != nil {
		return err
	}
	log.Info("UpdateRolePolicy(%v, %v), %b updated", rp.ToArr(), newRp.ToArr(), updated)

	if updated {
		return c.save()
	}

	return nil
}

func (c *RbacClient) DeleteRolePolicy(rp *RolePolicy) error {
	removed, err := c.e.RemoveFilteredGroupingPolicy(0, rp.User)
	if err != nil {
		return err
	}
	log.Info("DeleteRolePolicy %v, %b removed", rp)
	if removed {
		return c.save()
	}
	return nil
}

func (c *RbacClient) save() error {
	if err := c.e.SavePolicy(); err != nil {
		log.Errorf("save/SavePolicy, %v", err)
		return err
	}
	if err := c.w.Update(); err != nil {
		log.Errorf("save/Update, %v", err)
	}
	return nil
}

func (c *RbacClient) AddActionPolicies(policies []Policy) error {
	if len(policies) == 0 {
		return nil
	}
	_policies := [][]string{}
	for _, p := range policies {
		_policy := p.ToArr()
		_policies = append(_policies, _policy)
	}
	log.Info("rabc AddPolicies, %+v", _policies)

	ok, err := c.e.AddPoliciesEx(_policies)
	if err != nil {
		log.Error("AddPolicies, %+v, %v", _policies, err)
		return err
	}
	if !ok {
		log.Errorf("AddPolicies, casbin Enforcer.AddPolices not ok")
		return errors.New("AddPolices Failed")
	}
	log.Info("AddPolicies %+v success", policies)
	return c.save()
}

func (c *RbacClient) GetActionPolicies(role string) ([]*ActionPolicy, error) {
	log.Info("GetActionPolicies, role is %s", role)
	content, err := c.e.GetFilteredPolicy(0, role)
	if err != nil {
		log.Error("GetActionPolicies/GetFilteredPolicy failed, %v, error, %v", content, err)
		return nil, err
	}

	log.Info("GetActionPolicies/FilteredPolicies return content: %v", content)
	ans := []*ActionPolicy{}
	for _, item := range content {
		ans = append(ans, &ActionPolicy{
			Sub: item[0],
			Dom: item[1],
			Obj: item[2],
			Act: item[3],
		})
	}
	return ans, nil
}

func (c *RbacClient) DeleteActionPolicy(ap *ActionPolicy) error {
	removed, err := c.e.RemovePolicy(ap.ToArr())
	if err != nil {
		log.Error("DeleteActionPolicy/RemovePolicy, error %v", err)
		return err
	}

	log.Info("DeleteActionPolicy %v, removed: %v", *ap, removed)
	if removed {
		return c.save()
	}
	return nil
}

func (c *RbacClient) Enforce(r *Req) (bool, error) {
	ok, err := c.e.Enforce(r.sub, r.dom, r.obj, r.act)
	if err != nil {
		log.Error("Enforce(%v) error, %v", *r, err)
		return false, err
	}
	log.Info("Enforce(%v) result , %v", *r, ok)
	return ok, nil
}
