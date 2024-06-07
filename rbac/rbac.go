package rbac

import (
	"errors"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	"github.com/casbin/redis-adapter/v3"
	rediswatcher "github.com/casbin/redis-watcher/v2"
	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/log"
	"github.com/redis/go-redis/v9"
)

// [request_definition]
// ten: tenant
// dom: service_name
// sub: user
// obj: path
// act: method
//
// [policy_definition]
// ten: tenant
// dom: service_name
// sub: role
// obj: path
// act: method
const text = `
[request_definition]
r = ten, dom, sub, obj, act

[policy_definition]
p = dom, sub, obj, act
p1 = ten, dom

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && r.ten == p1.ten && r.dom == p1.dom && r.dom == p.dom && r.obj == p.obj && r.act == p.act
`

type Req struct {
	ten string
	dom string
	sub string
	obj string
	act string
}

type Policy interface {
	ToArr() []string
}

type ActionPolicy struct {
	dom string
	sub string
	obj string
	act string
}

type TenantPolicy struct {
	ten string
	dom string
}

type RolePolicy struct {
	user string
	role string
}

type RbacClient struct {
	e *casbin.Enforcer
	w persist.Watcher
}

var rbacClient *RbacClient = nil

func NewRbacClient() *RbacClient {
	if rbacClient != nil {
		return rbacClient
	}

	redisAddr := config.GetConfigString("redis_addr")
	if redisAddr == "" {
		log.Fatal("config.yaml no redis_addr configuration")
	}
	log.Info("rbac redis address is %v", redisAddr)

	m, err := model.NewModelFromString(text)
	if err != nil {
		log.Fatalf("error, newModelFromString, %v", err)
	}
	adapter, err := redisadapter.NewAdapter("tcp", redisAddr)
	if err != nil {
		log.Fatalf("error, newAdapter, %v", err)
	}

	e, err := casbin.NewEnforcer(m, adapter)
	if err != nil {
		log.Fatalf("error, NewEnforcer, %v", err)
	}

	if err := e.LoadPolicy(); err != nil {
		log.Fatalf("error, LoadPolicy, %v", err)
	}

	w, err := rediswatcher.NewWatcher(redisAddr, rediswatcher.WatcherOptions{
		Options: redis.Options{
			Network: "tcp",
		},
		Channel:    "/casbin",
		IgnoreSelf: true,
	})
	if err != nil {
		log.Fatalf("error, NewWatcher, %v", err)
	}

	if err := e.SetWatcher(w); err != nil {
		log.Fatalf("error, SetWatcher, %v", err)
	}

	rbacClient = &RbacClient{
		w: w,
		e: e,
	}
	return rbacClient
}

func NewActionPolicy(dom, sub, obj, act string) Policy {
	return &ActionPolicy{
		dom,
		sub,
		obj,
		act,
	}
}

func (p ActionPolicy) ToArr() []string {
	return []string{"p", p.dom, p.sub, p.obj, p.act}
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

func NewRolePolicy(user, role string) Policy {
	return &RolePolicy{
		role,
		user,
	}
}

func (p *RolePolicy) ToArr() []string {
	return []string{"g", p.user, p.role}
}

func NewReq(ten, dom, sub, obj, act string) *Req {
	return &Req{
		ten,
		dom,
		sub,
		obj,
		act,
	}
}

func (c *RbacClient) AddPolicies(policies []Policy) error {
	_policies := [][]string{}
	for _, p := range policies {
		_policy := p.ToArr()
		_policies = append(_policies, _policy)
	}
	log.Infof("rabc AddPolicies, %+v", _policies)

	ok, err := c.e.AddPoliciesEx(_policies)
	if err != nil {
		log.Error("AddPolicies, %+v, %v", _policies, err)
		return err
	}
	if !ok {
		log.Errorf("AddPolicies, casbin Enforcer.AddPolices not ok")
		return errors.New("AddPolices Failed")
	}
	if err := c.e.SavePolicy(); err != nil {
		log.Errorf("AddPolicies/SavePolicy, %v", err)
		return err
	}
	if err := c.w.Update(); err != nil {
		log.Errorf("AddPolicies/Update, %v", err)
	}
	log.Info("AddPolicies %+v success", policies)
	return nil
}

func (c *RbacClient) Enforce(r *Req) (bool, error) {
	return true, nil
}
