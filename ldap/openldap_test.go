package ldap

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/spf13/viper"
)

func TestNormalizeDN(t *testing.T) {
	base := "dc=example,dc=com"
	if got := normalizeDN(base, "ou=people"); got != "ou=people,dc=example,dc=com" {
		t.Fatalf("normalizeDN base join mismatch: %s", got)
	}
	if got := normalizeDN(base, "ou=people,dc=example,dc=com"); got != "ou=people,dc=example,dc=com" {
		t.Fatalf("normalizeDN should keep full DN: %s", got)
	}
	if got := normalizeDN(base, ""); got != "dc=example,dc=com" {
		t.Fatalf("normalizeDN empty ou mismatch: %s", got)
	}
}

func TestNormalizeOrgUser(t *testing.T) {
	_, err := normalizeOrgUser(&OrgUser{})
	if err == nil {
		t.Fatalf("expected error for empty uid")
	}

	u := &OrgUser{
		UID:       "alice",
		FirstName: "Alice",
		LastName:  "Wonder",
	}
	out, err := normalizeOrgUser(u)
	if err != nil {
		t.Fatalf("normalizeOrgUser error: %v", err)
	}
	if out.DisplayName != "Alice Wonder" {
		t.Fatalf("displayName not derived: %s", out.DisplayName)
	}
	if out.LastName != "Wonder" {
		t.Fatalf("lastName mismatch: %s", out.LastName)
	}

	u = &OrgUser{
		UID:       "bob",
		FirstName: "Bob",
	}
	out, err = normalizeOrgUser(u)
	if err != nil {
		t.Fatalf("normalizeOrgUser error: %v", err)
	}
	if out.DisplayName != "Bob" || out.LastName != "Bob" {
		t.Fatalf("displayName/lastName default mismatch: %s/%s", out.DisplayName, out.LastName)
	}

	u = &OrgUser{
		UID:         "bad",
		DisplayName: "Bad User",
		DeptCodes:   []string{""},
	}
	if _, err := normalizeOrgUser(u); err == nil {
		t.Fatalf("expected error for invalid deptCodes")
	}

	u = &OrgUser{
		UID:         "gender1",
		DisplayName: "Gender One",
		Gender:      "m",
	}
	out, err = normalizeOrgUser(u)
	if err != nil {
		t.Fatalf("normalizeOrgUser gender error: %v", err)
	}
	if out.Gender != "M" {
		t.Fatalf("gender normalization mismatch: %s", out.Gender)
	}

	u = &OrgUser{
		UID:         "gender2",
		DisplayName: "Gender Two",
		Gender:      "X",
	}
	if _, err := normalizeOrgUser(u); err == nil {
		t.Fatalf("expected error for invalid gender")
	}

	u = &OrgUser{
		UID:         "birth1",
		DisplayName: "Birth One",
		Birthday:    "1990-02-30",
	}
	if _, err := normalizeOrgUser(u); err == nil {
		t.Fatalf("expected error for invalid birthday")
	}
}

func TestNormalizeDepartment(t *testing.T) {
	_, err := normalizeDepartment(&Department{})
	if err == nil {
		t.Fatalf("expected error for empty code")
	}

	d := &Department{Code: "D001"}
	out, err := normalizeDepartment(d)
	if err != nil {
		t.Fatalf("normalizeDepartment error: %v", err)
	}
	if out.Name != "D001" {
		t.Fatalf("department name default mismatch: %s", out.Name)
	}

	d = &Department{Code: "D002", ParentDN: "invalidDN"}
	if _, err := normalizeDepartment(d); err == nil {
		t.Fatalf("expected error for invalid parentDn")
	}
}

func TestNormalizePosition(t *testing.T) {
	_, err := normalizePosition(&Position{})
	if err == nil {
		t.Fatalf("expected error for empty code")
	}

	p := &Position{Code: "P001"}
	out, err := normalizePosition(p)
	if err != nil {
		t.Fatalf("normalizePosition error: %v", err)
	}
	if out.Name != "P001" {
		t.Fatalf("position name default mismatch: %s", out.Name)
	}

	p = &Position{Code: "P002", DeptDN: "invalidDN"}
	if _, err := normalizePosition(p); err == nil {
		t.Fatalf("expected error for invalid deptDn")
	}
}

func TestValidateDN(t *testing.T) {
	if err := validateDN("uid=test,ou=people,dc=example,dc=com"); err != nil {
		t.Fatalf("valid DN should pass: %v", err)
	}
	if err := validateDN("uid=test,ou=people,dc=example,dc=com,invalid"); err == nil {
		t.Fatalf("invalid DN should error")
	}
}

func TestOpenLDAPConfigDefaults(t *testing.T) {
	viper.Set(ldapAddrKey, "127.0.0.1:389")
	viper.Set(ldapBindDNKey, "cn=admin,dc=example,dc=com")
	viper.Set(ldapBindPasswordKey, "admin123")
	viper.Set(ldapBaseDNKey, "dc=example,dc=com")
	viper.Set(ldapPeopleOUKey, "")
	viper.Set(ldapDepartmentOUKey, "")
	viper.Set(ldapPositionOUKey, "")

	cfg, err := loadOpenLDAPConfig()
	if err != nil {
		t.Fatalf("loadOpenLDAPConfig error: %v", err)
	}
	if cfg.PeopleOU != defaultPeopleOU || cfg.DeptOU != defaultDepartmentOU || cfg.PositionOU != defaultPositionOU {
		t.Fatalf("default OU mismatch: %+v", cfg)
	}
}

func TestOpenLDAPURL(t *testing.T) {
	cfg := OpenLDAPConfig{Addr: "127.0.0.1:389", UseTLS: false}
	if got := cfg.url(); got != "ldap://127.0.0.1:389" {
		t.Fatalf("url mismatch: %s", got)
	}
	cfg.UseTLS = true
	if got := cfg.url(); got != "ldaps://127.0.0.1:389" {
		t.Fatalf("url mismatch: %s", got)
	}
	cfg.Addr = "ldaps://ldap.example.com:636"
	if got := cfg.url(); got != "ldaps://ldap.example.com:636" {
		t.Fatalf("url should keep scheme: %s", got)
	}
}

func TestOpenLDAPIntegration(t *testing.T) {
	if os.Getenv("OPENLDAP_TEST") == "" {
		t.Skip("set OPENLDAP_TEST=1 to run integration test")
	}

	cfg := OpenLDAPConfig{
		Addr:         "113.45.4.22:8389",
		BindDN:       "cn=admin,dc=msss,dc=com",
		BindPassword: "msss@2026",
		BaseDN:       "dc=msss,dc=com",
		PeopleOU:     defaultPeopleOU,
		DeptOU:       defaultDepartmentOU,
		PositionOU:   defaultPositionOU,
		UseTLS:       false,
	}

	client, err := NewOpenLDAP(cfg)
	if err != nil {
		t.Fatalf("NewOpenLDAP error: %v", err)
	}

	if err := assertBaseOUs(cfg); err != nil {
		t.Fatalf("OpenLDAP base OU not ready: %v", err)
	}

	ctx := context.Background()
	suffix := time.Now().UnixNano()
	deptParentCode := fmt.Sprintf("D%dP", suffix)
	deptCode := fmt.Sprintf("D%d", suffix)
	deptCode2 := fmt.Sprintf("D%dX", suffix)
	posCode := fmt.Sprintf("P%d", suffix)
	uid := fmt.Sprintf("u%d", suffix)

	deptParentDN := fmt.Sprintf("%s=%s,%s", attrOU, deptParentCode, normalizeDN(cfg.BaseDN, cfg.DeptOU))
	deptDN := fmt.Sprintf("%s=%s,%s", attrOU, deptCode, normalizeDN(cfg.BaseDN, cfg.DeptOU))
	deptDN2 := fmt.Sprintf("%s=%s,%s", attrOU, deptCode2, normalizeDN(cfg.BaseDN, cfg.DeptOU))
	userDN := fmt.Sprintf("%s=%s,%s", attrUID, uid, normalizeDN(cfg.BaseDN, cfg.PeopleOU))

	deptParent := &Department{
		Code: deptParentCode,
		Name: "Parent Department",
	}
	if err := client.AddDepartment(ctx, deptParent); err != nil {
		t.Fatalf("AddDepartment(parent) error: %v", err)
	}
	defer func() { _ = client.DeleteDepartment(ctx, deptParentCode) }()

	dept := &Department{
		Code: deptCode,
		Name: "Test Department",
	}
	if err := client.AddDepartment(ctx, dept); err != nil {
		t.Fatalf("AddDepartment error: %v", err)
	}
	defer func() { _ = client.DeleteDepartment(ctx, deptCode) }()

	dept2 := &Department{
		Code: deptCode2,
		Name: "Test Department 2",
	}
	if err := client.AddDepartment(ctx, dept2); err != nil {
		t.Fatalf("AddDepartment(2) error: %v", err)
	}
	defer func() { _ = client.DeleteDepartment(ctx, deptCode2) }()

	pos := &Position{
		Code:   posCode,
		Name:   "Test Position",
		DeptDN: deptDN,
	}
	if err := client.AddPosition(ctx, pos); err != nil {
		t.Fatalf("AddPosition error: %v", err)
	}
	defer func() { _ = client.DeletePosition(ctx, posCode) }()

	user := &OrgUser{
		UID:         uid,
		Password:    "testPass123",
		Phone:       "13800138000",
		Address:     "Shenzhen, Nanshan District",
		Gender:      "M",
		Birthday:    "1990-01-01",
		DisplayName: "Test User",
		LastName:    "User",
		Status:      "enabled",
	}
	if err := client.AddUser(ctx, user); err != nil {
		t.Fatalf("AddUser error: %v", err)
	}
	defer func() { _ = client.DeleteUser(ctx, uid) }()

	if gotUser, err := client.ValidateUser(ctx, user.Phone, user.Password); err != nil {
		t.Fatalf("ValidateUser error: %v", err)
	} else if gotUser.UID != uid {
		t.Fatalf("ValidateUser uid mismatch: %s", gotUser.UID)
	}
	if _, err := client.ValidateUser(ctx, user.Phone, "badPass"); err == nil {
		t.Fatalf("expected ValidateUser error with bad password")
	}

	dept.ManagerDN = userDN
	dept.ParentDN = deptParentDN
	dept.Name = "Test Department Updated"
	if err := client.UpdateDepartment(ctx, dept); err != nil {
		t.Fatalf("UpdateDepartment(manager) error: %v", err)
	}

	if gotDept, err := client.GetDepartment(ctx, deptCode); err != nil {
		t.Fatalf("GetDepartment error: %v", err)
	} else if gotDept.ManagerDN != userDN || gotDept.ParentDN != deptParentDN || gotDept.Name != dept.Name {
		t.Fatalf("GetDepartment mismatch: %+v", gotDept)
	}

	pos.Name = "Test Position Updated"
	pos.DeptDN = deptDN2
	if err := client.UpdatePosition(ctx, pos); err != nil {
		t.Fatalf("UpdatePosition error: %v", err)
	}

	if gotPos, err := client.GetPosition(ctx, posCode); err != nil {
		t.Fatalf("GetPosition error: %v", err)
	} else if gotPos.DeptDN != deptDN2 || gotPos.Name != pos.Name {
		t.Fatalf("GetPosition mismatch: %+v", gotPos)
	}

	if err := client.AddUserDepartments(ctx, uid, []string{deptCode}); err != nil {
		t.Fatalf("AddUserDepartments error: %v", err)
	}
	if err := client.AddUserPositions(ctx, uid, []string{posCode}); err != nil {
		t.Fatalf("AddUserPositions error: %v", err)
	}

	got, err := client.GetUser(ctx, uid)
	if err != nil {
		t.Fatalf("GetUser error: %v", err)
	}
	if got.UID != uid {
		t.Fatalf("GetUser uid mismatch: %s", got.UID)
	}
	if got.Gender != "M" || got.Birthday != "1990-01-01" {
		t.Fatalf("GetUser gender/birthday mismatch: %s/%s", got.Gender, got.Birthday)
	}

	user.DisplayName = "Test User Updated"
	user.Email = "test@example.com"
	user.Phone = "13800138001"
	user.Address = "Shenzhen, Futian District"
	user.Gender = "F"
	user.Birthday = "1991-02-02"
	user.EmployeeNo = "E20260001"
	user.Status = "disabled"
	if err := client.UpdateUser(ctx, user); err != nil {
		t.Fatalf("UpdateUser error: %v", err)
	}

	got, err = client.GetUser(ctx, uid)
	if err != nil {
		t.Fatalf("GetUser after update error: %v", err)
	}
	if got.DisplayName != user.DisplayName || got.Email != user.Email || got.Phone != user.Phone || got.Address != user.Address {
		t.Fatalf("GetUser updated fields mismatch: %+v", got)
	}
	if got.Gender != "F" || got.Birthday != "1991-02-02" || got.EmployeeNo != "E20260001" || got.Status != "disabled" {
		t.Fatalf("GetUser updated meta mismatch: %+v", got)
	}

	if err := client.AddUserDepartments(ctx, uid, []string{deptCode2}); err != nil {
		t.Fatalf("AddUserDepartments(2) error: %v", err)
	}
	if err := client.RemoveUserDepartments(ctx, uid, []string{deptCode}); err != nil {
		t.Fatalf("RemoveUserDepartments error: %v", err)
	}
	if err := client.RemoveUserPositions(ctx, uid, []string{posCode}); err != nil {
		t.Fatalf("RemoveUserPositions error: %v", err)
	}

	got, err = client.GetUser(ctx, uid)
	if err != nil {
		t.Fatalf("GetUser after remove error: %v", err)
	}
	if len(got.DeptCodes) != 1 || got.DeptCodes[0] != deptCode2 {
		t.Fatalf("DeptCodes mismatch after remove: %+v", got.DeptCodes)
	}
	if len(got.PositionCodes) != 0 {
		t.Fatalf("PositionCodes should be empty after remove: %+v", got.PositionCodes)
	}

	users, err := client.ListUser(ctx)
	if err != nil {
		t.Fatalf("ListUser error: %v", err)
	}
	if !containsUser(users, uid) {
		t.Fatalf("ListUser missing uid: %s", uid)
	}

	depts, err := client.ListDepartment(ctx)
	if err != nil {
		t.Fatalf("ListDepartment error: %v", err)
	}
	if !containsDept(depts, deptCode) || !containsDept(depts, deptCode2) {
		t.Fatalf("ListDepartment missing dept codes")
	}

	posList, err := client.ListPosition(ctx)
	if err != nil {
		t.Fatalf("ListPosition error: %v", err)
	}
	if !containsPosition(posList, posCode) {
		t.Fatalf("ListPosition missing pos code: %s", posCode)
	}

	if err := client.DeleteUser(ctx, uid); err != nil {
		t.Fatalf("DeleteUser error: %v", err)
	}
	if _, err := client.GetUser(ctx, uid); err == nil {
		t.Fatalf("expected GetUser error after delete")
	}
}

func assertBaseOUs(cfg OpenLDAPConfig) error {
	conn, err := ldap.DialURL(cfg.url())
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
		return err
	}

	ouBases := []string{
		normalizeDN(cfg.BaseDN, cfg.PeopleOU),
		normalizeDN(cfg.BaseDN, cfg.DeptOU),
		normalizeDN(cfg.BaseDN, cfg.PositionOU),
	}
	for _, dn := range ouBases {
		if err := assertDNExists(conn, dn); err != nil {
			return err
		}
	}
	return nil
}

func assertDNExists(conn *ldap.Conn, dn string) error {
	req := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1, 0, false,
		"(objectClass=*)",
		[]string{"dn"},
		nil,
	)
	res, err := conn.Search(req)
	if err != nil {
		return err
	}
	if len(res.Entries) == 0 {
		return fmt.Errorf("dn not found: %s", dn)
	}
	return nil
}

func containsUser(users []*OrgUser, uid string) bool {
	for _, u := range users {
		if u != nil && u.UID == uid {
			return true
		}
	}
	return false
}

func containsDept(depts []*Department, code string) bool {
	for _, d := range depts {
		if d != nil && d.Code == code {
			return true
		}
	}
	return false
}

func containsPosition(positions []*Position, code string) bool {
	for _, p := range positions {
		if p != nil && p.Code == code {
			return true
		}
	}
	return false
}
