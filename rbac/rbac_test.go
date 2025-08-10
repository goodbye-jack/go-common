package rbac

import (
	"testing"

	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/log"
	"github.com/goodbye-jack/go-common/utils"
)

func TestRbac(t *testing.T) {
	redisAddr := config.GetConfigString("redis_addr")
	if redisAddr == "" {
		log.Fatal("config.yaml no redis_addr configuration")
	}
	rbac := NewRbacPolicy(redisAddr)
	aRp, err := rbac.GetRolePolicy("admin")
	if err != nil {
		log.Error("GetRolePolicy, error %v", err)
	}
	rp := &RolePolicy{
		User: "admin",
		Role: utils.RoleAdministrator,
	}
	if err := rbac.AddRolePolicy(rp); err != nil {
		log.Error("AddRolePolicy, error, %v", err)
	}

	ok, err := rbac.Enforce(NewReq("admin", "go-common", "/ping", "GET"))
	log.Info("Enforce, result, %v", ok)

	aRp, err = rbac.GetRolePolicy("admin")
	if err != nil {
		log.Error("GetRolePolicy, error %v", err)
	}
	log.Info("%v", aRp)

	rbac.UpdateRolePolicy(rp, utils.RoleManager)

	rbac.GetRolePolicy("admin")

	aps, err := rbac.GetActionPolicies("administrator")
	if err != nil {
		log.Error("GetActionPolicies, error %v", err)
	}
	log.Info("All ActionPolicies, %v", aps)

	if err := rbac.DeleteActionPolicy(aps[0]); err != nil {
		log.Error("DeleteActionPolicy, failed %v", err)
	}
}
