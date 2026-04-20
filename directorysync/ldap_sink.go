package directorysync

import (
	"context"
	"fmt"
	"strings"

	"github.com/goodbye-jack/go-common/config"
	commonldap "github.com/goodbye-jack/go-common/ldap"
)

const (
	defaultPeopleOU      = "ou=people"
	defaultDeptOU        = "ou=departments"
	defaultPositionOU    = "ou=positions"
	defaultGroupOU       = "ou=groups"
	defaultEnabledValue  = "normal"
	defaultDisabledValue = "disabled"
)

// LDAPSinkOptions 描述 LDAP 落地时的行为选项。
type LDAPSinkOptions struct {
	ResetPasswordOnExisting bool
}

// LDAPSink 使用 go-common/ldap 的原子能力落地同步记录。
type LDAPSink struct {
	client                  commonldap.Ldap
	naming                  ldapNaming
	resetPasswordOnExisting bool
}

type ldapNaming struct {
	baseDN       string
	peopleOU     string
	departmentOU string
	positionOU   string
	groupOU      string
}

// NewLDAPSinkFromConfig 使用当前应用配置创建 LDAP Sink。
func NewLDAPSinkFromConfig(options LDAPSinkOptions) (*LDAPSink, error) {
	client, err := commonldap.NewOpenLDAPFromConfig()
	if err != nil {
		return nil, err
	}
	return &LDAPSink{
		client: client,
		naming: ldapNaming{
			baseDN:       firstConfigValue("workflow.directory.ldap.base_dn", "ldap_base_dn"),
			peopleOU:     firstConfigValue("workflow.directory.ldap.people_ou", "ldap_people_ou", defaultPeopleOU),
			departmentOU: firstConfigValue("workflow.directory.ldap.department_ou", "ldap_department_ou", defaultDeptOU),
			positionOU:   firstConfigValue("workflow.directory.ldap.position_ou", "ldap_position_ou", defaultPositionOU),
			groupOU:      firstConfigValue("workflow.directory.ldap.group_ou", "ldap_group_ou", defaultGroupOU),
		},
		resetPasswordOnExisting: options.ResetPasswordOnExisting,
	}, nil
}

func firstConfigValue(keys ...string) string {
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		value := strings.TrimSpace(config.GetConfigString(trimmed))
		if value != "" {
			return value
		}
	}
	return ""
}

func (sink *LDAPSink) UpsertDepartment(ctx context.Context, record DepartmentRecord) error {
	if sink == nil || sink.client == nil {
		return fmt.Errorf("ldap sink is not initialized")
	}
	department := &commonldap.Department{
		Code:      record.Code,
		Name:      record.Name,
		ParentDN:  sink.departmentDN(record.ParentCode),
		ManagerDN: sink.managerDN(record.Attributes),
	}
	_, err := sink.client.GetDepartment(ctx, record.Code)
	if err == nil {
		return sink.client.UpdateDepartment(ctx, department)
	}
	if _, ok := err.(commonldap.LdapNotFoundError); ok {
		return sink.client.AddDepartment(ctx, department)
	}
	return err
}

func (sink *LDAPSink) UpsertPosition(ctx context.Context, record PositionRecord) error {
	if sink == nil || sink.client == nil {
		return fmt.Errorf("ldap sink is not initialized")
	}
	position := &commonldap.Position{
		Code:   record.Code,
		Name:   record.Name,
		DeptDN: sink.departmentDN(record.DepartmentCode),
	}
	_, err := sink.client.GetPosition(ctx, record.Code)
	if err == nil {
		return sink.client.UpdatePosition(ctx, position)
	}
	if _, ok := err.(commonldap.LdapNotFoundError); ok {
		return sink.client.AddPosition(ctx, position)
	}
	return err
}

func (sink *LDAPSink) UpsertUser(ctx context.Context, record UserRecord) error {
	if sink == nil || sink.client == nil {
		return fmt.Errorf("ldap sink is not initialized")
	}
	orgUser, err := sink.client.GetUser(ctx, record.UserID)
	userPayload := sink.toOrgUser(record)
	if err == nil {
		if orgUser != nil {
			userPayload.DeptCodes = orgUser.DeptCodes
			userPayload.PositionCodes = orgUser.PositionCodes
		}
		if !sink.resetPasswordOnExisting {
			userPayload.Password = ""
		}
		return sink.client.UpdateUser(ctx, userPayload)
	}
	if _, ok := err.(commonldap.LdapNotFoundError); ok {
		return sink.client.AddUser(ctx, userPayload)
	}
	return err
}

func (sink *LDAPSink) UpsertGroup(ctx context.Context, record GroupRecord) error {
	if sink == nil || sink.client == nil {
		return fmt.Errorf("ldap sink is not initialized")
	}
	groupClient, ok := sink.client.(commonldap.GroupLdap)
	if !ok {
		return fmt.Errorf("ldap client does not support group projection")
	}
	groupPayload := sink.toLDAPGroup(record)
	if len(groupPayload.MemberDNs) == 0 {
		err := groupClient.DeleteGroup(ctx, record.Code)
		if err != nil {
			if _, notFound := err.(commonldap.LdapNotFoundError); notFound {
				return nil
			}
			return err
		}
		return nil
	}
	_, err := groupClient.GetGroup(ctx, record.Code)
	if err == nil {
		return groupClient.UpdateGroup(ctx, groupPayload)
	}
	if _, notFound := err.(commonldap.LdapNotFoundError); notFound {
		return groupClient.AddGroup(ctx, groupPayload)
	}
	return err
}

func (sink *LDAPSink) BindUserDepartments(ctx context.Context, userID string, departmentCodes []string) error {
	userProfile, err := sink.client.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	userProfile.DeptCodes = append([]string(nil), departmentCodes...)
	return sink.client.UpdateUser(ctx, userProfile)
}

func (sink *LDAPSink) BindUserPositions(ctx context.Context, userID string, positionCodes []string) error {
	userProfile, err := sink.client.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	userProfile.PositionCodes = append([]string(nil), positionCodes...)
	return sink.client.UpdateUser(ctx, userProfile)
}

func (sink *LDAPSink) DisableUser(ctx context.Context, userID string) error {
	userProfile, err := sink.client.GetUser(ctx, userID)
	if err != nil {
		if _, ok := err.(commonldap.LdapNotFoundError); ok {
			return nil
		}
		return err
	}
	userProfile.Status = defaultDisabledValue
	return sink.client.UpdateUser(ctx, userProfile)
}

func (sink *LDAPSink) UpdatePassword(ctx context.Context, userID, plainPassword string) error {
	if strings.TrimSpace(plainPassword) == "" {
		return nil
	}
	userProfile, err := sink.client.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	userProfile.Password = plainPassword
	return sink.client.UpdateUser(ctx, userProfile)
}

func (sink *LDAPSink) toOrgUser(record UserRecord) *commonldap.OrgUser {
	attributes := record.Attributes
	firstName := firstAttribute(attributes, "first_name", "given_name")
	lastName := firstAttribute(attributes, "last_name", "family_name")
	if lastName == "" {
		lastName = record.DisplayName
	}
	return &commonldap.OrgUser{
		UID:         record.UserID,
		Password:    strings.TrimSpace(record.InitialPassword),
		Phone:       strings.TrimSpace(record.Mobile),
		Address:     firstAttribute(attributes, "address"),
		Gender:      strings.TrimSpace(strings.ToUpper(firstAttribute(attributes, "gender"))),
		Birthday:    firstAttribute(attributes, "birthday"),
		Email:       strings.TrimSpace(record.Email),
		DisplayName: strings.TrimSpace(record.DisplayName),
		FirstName:   firstName,
		LastName:    lastName,
		EmployeeNo:  firstAttribute(attributes, "employee_no"),
		Status:      statusValue(record.Enabled),
	}
}

func (sink *LDAPSink) toLDAPGroup(record GroupRecord) *commonldap.Group {
	memberDNs := make([]string, 0, len(record.MemberUserIDs))
	for _, userID := range normalizeStringSlice(record.MemberUserIDs) {
		if dn := sink.userDN(userID); dn != "" {
			memberDNs = append(memberDNs, dn)
		}
	}
	return &commonldap.Group{
		Code:      strings.TrimSpace(record.Code),
		Name:      strings.TrimSpace(record.Name),
		MemberDNs: memberDNs,
	}
}

func firstAttribute(attributes map[string]string, keys ...string) string {
	for _, key := range keys {
		if attributes == nil {
			return ""
		}
		value := strings.TrimSpace(attributes[key])
		if value != "" {
			return value
		}
	}
	return ""
}

func statusValue(enabled bool) string {
	if enabled {
		return defaultEnabledValue
	}
	return defaultDisabledValue
}

func (sink *LDAPSink) departmentDN(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	return normalizeDNComponent("ou", code, sink.naming.departmentBaseDN())
}

func (sink *LDAPSink) positionDN(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	return normalizeDNComponent("cn", code, sink.naming.positionBaseDN())
}

func normalizeDNComponent(attribute, code, base string) string {
	if code == "" || base == "" {
		return ""
	}
	return fmt.Sprintf("%s=%s,%s", attribute, code, base)
}

func (naming ldapNaming) departmentBaseDN() string {
	departmentOU := strings.TrimSpace(naming.departmentOU)
	if departmentOU == "" {
		departmentOU = defaultDeptOU
	}
	return joinBaseDN(departmentOU, naming.baseDN)
}

func (naming ldapNaming) positionBaseDN() string {
	positionOU := strings.TrimSpace(naming.positionOU)
	if positionOU == "" {
		positionOU = defaultPositionOU
	}
	return joinBaseDN(positionOU, naming.baseDN)
}

func (naming ldapNaming) groupBaseDN() string {
	groupOU := strings.TrimSpace(naming.groupOU)
	if groupOU == "" {
		groupOU = defaultGroupOU
	}
	return joinBaseDN(groupOU, naming.baseDN)
}

func joinBaseDN(ouValue, baseDN string) string {
	trimmedOU := strings.TrimSpace(ouValue)
	trimmedBaseDN := strings.TrimSpace(baseDN)
	if trimmedOU == "" {
		return trimmedBaseDN
	}
	if strings.Contains(trimmedOU, ",") {
		return trimmedOU
	}
	if trimmedBaseDN == "" {
		return trimmedOU
	}
	return fmt.Sprintf("%s,%s", trimmedOU, trimmedBaseDN)
}

func (sink *LDAPSink) managerDN(attributes map[string]string) string {
	managerUserID := firstAttribute(attributes, "manager_user_id", "manager_uid")
	if managerUserID == "" {
		return ""
	}
	return sink.userDN(managerUserID)
}

func (sink *LDAPSink) userDN(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}
	return normalizeDNComponent("uid", userID, sink.naming.peopleBaseDN())
}

func (naming ldapNaming) peopleBaseDN() string {
	peopleOU := strings.TrimSpace(naming.peopleOU)
	if peopleOU == "" {
		peopleOU = defaultPeopleOU
	}
	return joinBaseDN(peopleOU, naming.baseDN)
}
