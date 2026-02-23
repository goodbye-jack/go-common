package rbac

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/orm"
	"github.com/goodbye-jack/go-common/utils"
	"gorm.io/gorm"
)

const (
	RoleTypeInternal = "internal"
	RoleTypeBusiness = "business"
)

const (
	InternalRolePrefix = "internal."
	ActionRead         = "read"
	ActionWrite        = "write"
)

type Role struct {
	ID        uint      `gorm:"primaryKey"`
	Code      string    `gorm:"size:128;uniqueIndex"`
	Name      string    `gorm:"size:128"`
	Type      string    `gorm:"size:16;index"`
	Status    int       `gorm:"default:1"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Role) TableName() string {
	return "rbac_roles"
}

type RoleInherit struct {
	ID          uint      `gorm:"primaryKey"`
	RoleCode    string    `gorm:"size:128;index;uniqueIndex:uniq_role_inherit"`
	InheritCode string    `gorm:"size:128;index;uniqueIndex:uniq_role_inherit"`
	CreatedAt   time.Time
}

func (RoleInherit) TableName() string {
	return "rbac_role_inherits"
}

type UserRole struct {
	ID       uint      `gorm:"primaryKey"`
	UID      string    `gorm:"size:128;index;uniqueIndex:uniq_user_role"`
	RoleCode string    `gorm:"size:128;index;uniqueIndex:uniq_user_role"`
	CreatedAt time.Time
}

func (UserRole) TableName() string {
	return "rbac_user_roles"
}

var (
	storeOnce sync.Once
	storeDB   *gorm.DB
	storeErr  error
)

func getStoreDB() (*gorm.DB, error) {
	storeOnce.Do(func() {
		if orm.DB != nil {
			storeDB = orm.DB.GetDB()
		}
		if storeDB == nil {
			storeDB, storeErr = initStoreDBFromLegacyConfig()
			if storeErr != nil {
				return
			}
		}
		if storeDB == nil {
			storeErr = errors.New("rbac store db not initialized")
			return
		}
		if err := storeDB.AutoMigrate(&Role{}, &RoleInherit{}, &UserRole{}); err != nil {
			storeErr = err
			return
		}
	})
	return storeDB, storeErr
}

func initStoreDBFromLegacyConfig() (*gorm.DB, error) {
	dsn := strings.TrimSpace(config.GetConfigString("db_dsn"))
	dbType := strings.TrimSpace(config.GetConfigString("db_type"))
	slow := config.GetConfigInt("db_sql_execute_time")
	if dsn == "" {
		return nil, errors.New("db_dsn is empty")
	}
	if dbType == "" {
		dbType = string(utils.DBTypeMySQL)
	}
	ormInstance := orm.NewOrm(dsn, utils.DBType(dbType), slow)
	if ormInstance == nil {
		return nil, errors.New("init orm failed")
	}
	orm.DB = ormInstance
	return ormInstance.GetDB(), nil
}

func normalizeRoleCode(code string) string {
	return strings.TrimSpace(code)
}

func ensureRole(code, name, typ string, status int) error {
	code = normalizeRoleCode(code)
	if code == "" {
		return errors.New("role code is empty")
	}
	if typ != RoleTypeInternal && typ != RoleTypeBusiness {
		return fmt.Errorf("invalid role type: %s", typ)
	}
	if status == 0 {
		status = 1
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = code
	}

	db, err := getStoreDB()
	if err != nil {
		return err
	}

	var role Role
	if err := db.Where("code = ?", code).First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			role = Role{Code: code, Name: name, Type: typ, Status: status}
			return db.Create(&role).Error
		}
		return err
	}

	if role.Type != typ {
		return fmt.Errorf("role %s type mismatch", code)
	}
	updates := map[string]interface{}{}
	if role.Name != name {
		updates["name"] = name
	}
	if role.Status != status {
		updates["status"] = status
	}
	if len(updates) == 0 {
		return nil
	}
	return db.Model(&Role{}).Where("code = ?", code).Updates(updates).Error
}

func EnsureInternalRole(resource, action string) (string, error) {
	roleCode, err := BuildInternalRole(resource, action)
	if err != nil {
		return "", err
	}
	if roleCode == "" {
		return "", nil
	}
	name := fmt.Sprintf("%s %s", resource, action)
	if err := ensureRole(roleCode, name, RoleTypeInternal, 1); err != nil {
		return "", err
	}
	return roleCode, nil
}

func EnsureBusinessRole(code, name string, status int) error {
	if IsInternalRoleCode(code) {
		return fmt.Errorf("role %s is internal", code)
	}
	return ensureRole(code, name, RoleTypeBusiness, status)
}

func UpdateBusinessRole(code, name string, status int) error {
	code = normalizeRoleCode(code)
	if code == "" {
		return errors.New("role code is empty")
	}
	if IsInternalRoleCode(code) {
		return fmt.Errorf("role %s is internal", code)
	}
	db, err := getStoreDB()
	if err != nil {
		return err
	}
	var role Role
	if err := db.Where("code = ?", code).First(&role).Error; err != nil {
		return err
	}
	if role.Type != RoleTypeBusiness {
		return fmt.Errorf("role %s is not business", code)
	}
	updates := map[string]interface{}{}
	if name != "" && role.Name != name {
		updates["name"] = strings.TrimSpace(name)
	}
	if status >= 0 && role.Status != status {
		updates["status"] = status
	}
	if len(updates) == 0 {
		return nil
	}
	return db.Model(&Role{}).Where("code = ?", code).Updates(updates).Error
}

func DeleteBusinessRole(code string) error {
	code = normalizeRoleCode(code)
	if code == "" {
		return errors.New("role code is empty")
	}
	if IsInternalRoleCode(code) {
		return fmt.Errorf("role %s is internal", code)
	}
	if code == "ADMIN_ROLE" {
		return errors.New("admin role cannot be deleted")
	}
	db, err := getStoreDB()
	if err != nil {
		return err
	}
	ok, err := roleExists(db, code, RoleTypeBusiness)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("business role %s not found", code)
	}
	oldInherits, err := ListRoleInherits(code)
	if err != nil {
		return err
	}
	oldUsers, err := listUsersForRole(code)
	if err != nil {
		return err
	}

	client := NewRbacClient()
	if err := client.RemoveGroupingPoliciesForSubject(code); err != nil {
		return err
	}
	if err := client.RemoveGroupingPoliciesForRole(code); err != nil {
		return err
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_code = ?", code).Delete(&RoleInherit{}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_code = ?", code).Delete(&UserRole{}).Error; err != nil {
			return err
		}
		if err := tx.Where("code = ?", code).Delete(&Role{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		_ = client.RemoveGroupingPoliciesForSubject(code)
		for _, inherit := range oldInherits {
			_ = client.AddGroupingPolicy(code, inherit)
		}
		for _, uid := range oldUsers {
			_ = client.AddGroupingPolicy(uid, code)
		}
		return err
	}
	return nil
}

func ListRolesByType(typ string) ([]Role, error) {
	db, err := getStoreDB()
	if err != nil {
		return nil, err
	}
	var roles []Role
	if err := db.Where("type = ?", typ).Order("id asc").Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

func ListInternalRoles() ([]Role, error) {
	return ListRolesByType(RoleTypeInternal)
}

func ListBusinessRoles() ([]Role, error) {
	return ListRolesByType(RoleTypeBusiness)
}

func ListRoleInherits(roleCode string) ([]string, error) {
	roleCode = normalizeRoleCode(roleCode)
	if roleCode == "" {
		return nil, errors.New("role code is empty")
	}
	db, err := getStoreDB()
	if err != nil {
		return nil, err
	}
	var rows []RoleInherit
	if err := db.Where("role_code = ?", roleCode).Find(&rows).Error; err != nil {
		return nil, err
	}
	ans := make([]string, 0, len(rows))
	for _, row := range rows {
		ans = append(ans, row.InheritCode)
	}
	return ans, nil
}

func ListUserRoles(uid string) ([]string, error) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil, errors.New("uid is empty")
	}
	db, err := getStoreDB()
	if err != nil {
		return nil, err
	}
	var rows []UserRole
	if err := db.Where("uid = ?", uid).Find(&rows).Error; err != nil {
		return nil, err
	}
	ans := make([]string, 0, len(rows))
	for _, row := range rows {
		ans = append(ans, row.RoleCode)
	}
	return ans, nil
}

func listUsersForRole(roleCode string) ([]string, error) {
	roleCode = normalizeRoleCode(roleCode)
	if roleCode == "" {
		return nil, errors.New("role code is empty")
	}
	db, err := getStoreDB()
	if err != nil {
		return nil, err
	}
	var rows []UserRole
	if err := db.Where("role_code = ?", roleCode).Find(&rows).Error; err != nil {
		return nil, err
	}
	users := make([]string, 0, len(rows))
	for _, row := range rows {
		users = append(users, row.UID)
	}
	return users, nil
}

func SetRoleInherits(roleCode string, inheritCodes []string) error {
	roleCode = normalizeRoleCode(roleCode)
	if roleCode == "" {
		return errors.New("role code is empty")
	}
	if IsInternalRoleCode(roleCode) {
		return fmt.Errorf("role %s is internal", roleCode)
	}
	clean := make([]string, 0, len(inheritCodes))
	seen := map[string]bool{}
	for _, code := range inheritCodes {
		code = normalizeRoleCode(code)
		if code == "" {
			continue
		}
		if !IsInternalRoleCode(code) {
			return fmt.Errorf("inherit role %s is not internal", code)
		}
		if !seen[code] {
			seen[code] = true
			clean = append(clean, code)
		}
	}

	db, err := getStoreDB()
	if err != nil {
		return err
	}
	ok, err := roleExists(db, roleCode, RoleTypeBusiness)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("business role %s not found", roleCode)
	}
	for _, code := range clean {
		ok, err := roleExists(db, code, RoleTypeInternal)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("internal role %s not found", code)
		}
	}

	oldInherits, err := ListRoleInherits(roleCode)
	if err != nil {
		return err
	}

	client := NewRbacClient()
	if err := client.RemoveGroupingPoliciesForSubject(roleCode); err != nil {
		return err
	}
	for _, code := range clean {
		if err := client.AddGroupingPolicy(roleCode, code); err != nil {
			return err
		}
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_code = ?", roleCode).Delete(&RoleInherit{}).Error; err != nil {
			return err
		}
		if len(clean) == 0 {
			return nil
		}
		rows := make([]RoleInherit, 0, len(clean))
		for _, code := range clean {
			rows = append(rows, RoleInherit{RoleCode: roleCode, InheritCode: code})
		}
		return tx.Create(&rows).Error
	}); err != nil {
		_ = client.RemoveGroupingPoliciesForSubject(roleCode)
		for _, code := range oldInherits {
			_ = client.AddGroupingPolicy(roleCode, code)
		}
		return err
	}
	return nil
}

func SetUserRoles(uid string, roleCodes []string) error {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return errors.New("uid is empty")
	}
	clean := make([]string, 0, len(roleCodes))
	seen := map[string]bool{}
	for _, code := range roleCodes {
		code = normalizeRoleCode(code)
		if code == "" {
			continue
		}
		if IsInternalRoleCode(code) {
			return fmt.Errorf("role %s is internal", code)
		}
		if !seen[code] {
			seen[code] = true
			clean = append(clean, code)
		}
	}

	db, err := getStoreDB()
	if err != nil {
		return err
	}
	for _, code := range clean {
		ok, err := roleExists(db, code, RoleTypeBusiness)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("business role %s not found", code)
		}
	}

	oldRoles, err := ListUserRoles(uid)
	if err != nil {
		return err
	}

	client := NewRbacClient()
	if err := client.RemoveGroupingPoliciesForSubject(uid); err != nil {
		return err
	}
	for _, code := range clean {
		if err := client.AddGroupingPolicy(uid, code); err != nil {
			return err
		}
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("uid = ?", uid).Delete(&UserRole{}).Error; err != nil {
			return err
		}
		if len(clean) == 0 {
			return nil
		}
		rows := make([]UserRole, 0, len(clean))
		for _, code := range clean {
			rows = append(rows, UserRole{UID: uid, RoleCode: code})
		}
		return tx.Create(&rows).Error
	}); err != nil {
		_ = client.RemoveGroupingPoliciesForSubject(uid)
		for _, code := range oldRoles {
			_ = client.AddGroupingPolicy(uid, code)
		}
		return err
	}
	return nil
}

func roleExists(db *gorm.DB, code string, typ string) (bool, error) {
	if db == nil {
		return false, errors.New("db is nil")
	}
	var count int64
	if err := db.Model(&Role{}).Where("code = ? AND type = ?", code, typ).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func EnsureAdminInheritsInternal(adminRole string) error {
	adminRole = normalizeRoleCode(adminRole)
	if adminRole == "" {
		return errors.New("admin role is empty")
	}
	internals, err := ListInternalRoles()
	if err != nil {
		return err
	}
	client := NewRbacClient()
	for _, r := range internals {
		if err := client.AddGroupingPolicy(adminRole, r.Code); err != nil {
			return err
		}
	}
	return nil
}

func BuildInternalRole(resource, action string) (string, error) {
	resource = strings.TrimSpace(resource)
	action = strings.TrimSpace(action)
	if resource == "" && action == "" {
		return "", nil
	}
	if resource == "" || action == "" {
		return "", fmt.Errorf("resource/action is required together")
	}
	resource = strings.ToLower(resource)
	action = strings.ToLower(action)
	switch action {
	case ActionRead, ActionWrite:
	default:
		return "", fmt.Errorf("invalid action %s", action)
	}
	if strings.Contains(resource, " ") {
		return "", fmt.Errorf("invalid resource %s", resource)
	}
	return InternalRolePrefix + resource + "." + action, nil
}

func IsInternalRoleCode(code string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(code)), InternalRolePrefix)
}

func EnsureInternalRoleByCode(code string) error {
	code = normalizeRoleCode(code)
	if !IsInternalRoleCode(code) {
		return fmt.Errorf("role %s is not internal", code)
	}
	return ensureRole(code, code, RoleTypeInternal, 1)
}
