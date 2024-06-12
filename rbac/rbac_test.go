package rbac

import (
	"testing"

	"github.com/goodbye-jack/go-common/utils"
)


func TestRbac(t *testing.T) {
	rbac := NewRbacClient()
	policy := NewRolePolicy("admin", utils.RoleAdministrator)

	rp, ok := policy.(*RolePolicy)
	if !ok {
		return
	}

	rbac.AddRolePolicy(rp)

	rbac.GetRolePolicy("admin")

	rbac.UpdateRolePolicy(rp, utils.RoleManager)

	rbac.GetRolePolicy("admin")
}
