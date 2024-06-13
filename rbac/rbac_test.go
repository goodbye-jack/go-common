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

	rp := &RolePolicy{
		User: "admin",
		Role: utils.RoleAdministrator,
	}

	rbac := NewRbacClient(redisAddr)

	rbac.AddRolePolicy(rp)

	rbac.GetRolePolicy("admin")

	rbac.UpdateRolePolicy(rp, utils.RoleManager)

	rbac.GetRolePolicy("admin")
}
