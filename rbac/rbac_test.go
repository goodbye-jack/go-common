package rbac

import (
	"os"
	"testing"

	"github.com/goodbye-jack/go-common/utils"
)

func TestRbac(t *testing.T) {
	redisAddr := os.Getenv("GO_COMMON_TEST_REDIS_ADDR")
	redisPassword := os.Getenv("GO_COMMON_TEST_REDIS_PASSWORD")
	if redisAddr == "" {
		t.Skip("GO_COMMON_TEST_REDIS_ADDR is not configured")
	}

	rbac := NewRbacClient(redisAddr, redisPassword)

	aRp, err := rbac.GetRolePolicy("admin")
	if err != nil {
		t.Fatalf("GetRolePolicy, error %v", err)
	}
	t.Logf("existing role policy: %v", aRp)

	rp := &RolePolicy{
		User: "admin",
		Role: utils.RoleAdministrator,
	}

	if err := rbac.AddRolePolicy(rp); err != nil {
		t.Fatalf("AddRolePolicy, error, %v", err)
	}

	ok, err := rbac.Enforce(NewReq("admin", "go-common", "/ping", "GET"))
	if err != nil {
		t.Fatalf("Enforce, error %v", err)
	}
	t.Logf("Enforce, result, %v", ok)

	aRp, err = rbac.GetRolePolicy("admin")
	if err != nil {
		t.Fatalf("GetRolePolicy, error %v", err)
	}
	t.Logf("%v", aRp)

	if err := rbac.UpdateRolePolicy(rp, utils.RoleManager); err != nil {
		t.Fatalf("UpdateRolePolicy, error %v", err)
	}

	if _, err := rbac.GetRolePolicy("admin"); err != nil {
		t.Fatalf("GetRolePolicy, error %v", err)
	}

	aps, err := rbac.GetActionPolicies("administrator")
	if err != nil {
		t.Fatalf("GetActionPolicies, error %v", err)
	}
	t.Logf("All ActionPolicies, %v", aps)
	if len(aps) == 0 {
		t.Skip("no administrator action policies to delete")
	}

	if err := rbac.DeleteActionPolicy(aps[0]); err != nil {
		t.Fatalf("DeleteActionPolicy, failed %v", err)
	}
}
