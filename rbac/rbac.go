package rbac

import (
	"errors"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	"github.com/casbin/redis-adapter/v3"
	rediswatcher "github.com/casbin/redis-watcher/v2"
	"github.com/goodbye-jack/go-common/log"
	"github.com/redis/go-redis/v9"
)

// [request_definition]
// dom: service_name
// sub: user
// obj: path
// act: method
//
// [policy_definition]
// dom: service_name
// sub: role
// obj: path
// act: method
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

type Req struct {
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
	User string
	Role string
}

type RbacClient struct {
	e *casbin.Enforcer
	w persist.Watcher
}

var rbacClient *RbacClient = nil

func NewRbacClient(redisAddr string) *RbacClient {
	if rbacClient != nil {
		return rbacClient
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
	return []string{p.sub, p.dom, p.obj, p.act}
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
	return []string {r.sub, r.dom, r.obj, r.act}
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
	_rp, err := c.GetRolePolicy(rp.User);
	if err != nil {
		log.Error("AddRolePolicy/GetRolePolicy(%v) error, %v", *rp, err)
		return err
	}
	if _rp != nil && _rp.Role != rp.Role {
		log.Info("AddRolePolicy(%v) had existed", *rp)
		return nil
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

func (c *RbacClient) Enforce(r *Req) (bool, error) {
	ok, err := c.e.Enforce(r.sub, r.dom, r.obj, r.act)
	if err != nil {
		log.Error("Enforce(%v) error, %v", *r, err)
		return false, err
	}
	log.Info("Enforce(%v) result , %v", *r, ok)
	return ok, nil
}
