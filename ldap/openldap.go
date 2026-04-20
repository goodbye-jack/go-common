package ldap

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/goodbye-jack/go-common/config"
)

const (
	ldapAddrKey           = "ldap_addr"
	ldapBindDNKey         = "ldap_bind_dn"
	ldapBindPasswordKey   = "ldap_bind_password"
	ldapBaseDNKey         = "ldap_base_dn"
	ldapPeopleOUKey       = "ldap_people_ou"
	ldapDepartmentOUKey   = "ldap_department_ou"
	ldapPositionOUKey     = "ldap_position_ou"
	ldapGroupOUKey        = "ldap_group_ou"
	ldapUseTLSKey         = "ldap_use_tls"
	defaultPeopleOU       = "ou=people"
	defaultDepartmentOU   = "ou=departments"
	defaultPositionOU     = "ou=positions"
	defaultGroupOU        = "ou=groups"
	attrUID               = "uid"
	attrCN                = "cn"
	attrOU                = "ou"
	attrSN                = "sn"
	attrGivenName         = "givenName"
	attrMail              = "mail"
	attrEmployeeNumber    = "employeeNumber"
	attrDepartmentNumber  = "departmentNumber"
	attrEmployeeType      = "employeeType"
	attrBusinessCategory  = "businessCategory"
	attrUserPassword      = "userPassword"
	attrTelephoneNumber   = "telephoneNumber"
	attrPostalAddress     = "postalAddress"
	attrDNQualifier       = "dnQualifier"
	attrTitle             = "title"
	attrDescription       = "description"
	attrSeeAlso           = "seeAlso"
	attrUniqueMember      = "uniqueMember"
	objectClassPerson     = "inetOrgPerson"
	objectClassDept       = "organizationalUnit"
	objectClassPosition   = "organizationalRole"
	objectClassGroup      = "groupOfUniqueNames"
	objectClassExtensible = "extensibleObject"
)

type OpenLDAPConfig struct {
	Addr         string
	BindDN       string
	BindPassword string
	BaseDN       string
	PeopleOU     string
	DeptOU       string
	PositionOU   string
	GroupOU      string
	UseTLS       bool
}

type OpenLDAP struct {
	cfg OpenLDAPConfig
}

func NewOpenLDAPFromConfig() (Ldap, error) {
	cfg, err := loadOpenLDAPConfig()
	if err != nil {
		return nil, err
	}
	return &OpenLDAP{cfg: cfg}, nil
}

func NewOpenLDAP(cfg OpenLDAPConfig) (Ldap, error) {
	if err := validateOpenLDAPConfig(cfg); err != nil {
		return nil, err
	}
	return &OpenLDAP{cfg: cfg}, nil
}

func New() (Ldap, error) {
	return NewOpenLDAPFromConfig()
}

func loadOpenLDAPConfig() (OpenLDAPConfig, error) {
	cfg := OpenLDAPConfig{
		Addr:         firstNonBlankConfig("workflow.directory.ldap.addr", ldapAddrKey),
		BindDN:       firstNonBlankConfig("workflow.directory.ldap.bind_dn", ldapBindDNKey),
		BindPassword: firstNonBlankString(config.GetConfigString("workflow.directory.ldap.bind_password"), config.GetConfigString(ldapBindPasswordKey)),
		BaseDN:       firstNonBlankConfig("workflow.directory.ldap.base_dn", ldapBaseDNKey),
		PeopleOU:     firstNonBlankConfig("workflow.directory.ldap.people_ou", ldapPeopleOUKey),
		DeptOU:       firstNonBlankConfig("workflow.directory.ldap.department_ou", ldapDepartmentOUKey),
		PositionOU:   firstNonBlankConfig("workflow.directory.ldap.position_ou", ldapPositionOUKey),
		GroupOU:      firstNonBlankConfig("workflow.directory.ldap.group_ou", ldapGroupOUKey),
		UseTLS:       config.GetConfigBool(ldapUseTLSKey),
	}
	if config.GetConfigString("workflow.directory.ldap.addr") != "" {
		cfg.UseTLS = config.GetConfigBool("workflow.directory.ldap.use_tls")
	}
	if cfg.PeopleOU == "" {
		cfg.PeopleOU = defaultPeopleOU
	}
	if cfg.DeptOU == "" {
		cfg.DeptOU = defaultDepartmentOU
	}
	if cfg.PositionOU == "" {
		cfg.PositionOU = defaultPositionOU
	}
	if cfg.GroupOU == "" {
		cfg.GroupOU = defaultGroupOU
	}
	return cfg, validateOpenLDAPConfig(cfg)
}

func validateOpenLDAPConfig(cfg OpenLDAPConfig) error {
	missing := []string{}
	if cfg.Addr == "" {
		missing = append(missing, ldapAddrKey)
	}
	if cfg.BindDN == "" {
		missing = append(missing, ldapBindDNKey)
	}
	if cfg.BindPassword == "" {
		missing = append(missing, ldapBindPasswordKey)
	}
	if cfg.BaseDN == "" {
		missing = append(missing, ldapBaseDNKey)
	}
	if len(missing) > 0 {
		return LdapParamsError{Params: missing}
	}
	return nil
}

func firstNonBlankConfig(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(config.GetConfigString(key)); value != "" {
			return value
		}
	}
	return ""
}

func firstNonBlankString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (o *OpenLDAP) withConn(fn func(conn *ldap.Conn) error) error {
	conn, err := ldap.DialURL(o.cfg.url())
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.Bind(o.cfg.BindDN, o.cfg.BindPassword); err != nil {
		return err
	}
	return fn(conn)
}

func (cfg OpenLDAPConfig) url() string {
	addr := cfg.Addr
	if strings.Contains(addr, "://") {
		return addr
	}
	scheme := "ldap"
	if cfg.UseTLS {
		scheme = "ldaps"
	}
	return fmt.Sprintf("%s://%s", scheme, addr)
}

func (cfg OpenLDAPConfig) peopleDN() string {
	return normalizeDN(cfg.BaseDN, cfg.PeopleOU)
}

func (cfg OpenLDAPConfig) deptDN() string {
	return normalizeDN(cfg.BaseDN, cfg.DeptOU)
}

func (cfg OpenLDAPConfig) positionDN() string {
	return normalizeDN(cfg.BaseDN, cfg.PositionOU)
}

func (cfg OpenLDAPConfig) groupDN() string {
	groupOU := cfg.GroupOU
	if strings.TrimSpace(groupOU) == "" {
		groupOU = defaultGroupOU
	}
	return normalizeDN(cfg.BaseDN, groupOU)
}

func normalizeDN(baseDN, ou string) string {
	if ou == "" {
		return baseDN
	}
	if strings.Contains(ou, ",") {
		return ou
	}
	return fmt.Sprintf("%s,%s", ou, baseDN)
}

func validateDN(dn string) error {
	if dn == "" {
		return nil
	}
	_, err := ldap.ParseDN(dn)
	return err
}

func validateDNs(dns []string) error {
	for _, dn := range dns {
		if err := validateDN(dn); err != nil {
			return err
		}
	}
	return nil
}

func normalizeCodes(values []string) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		code := strings.TrimSpace(v)
		if code == "" {
			return nil, fmt.Errorf("empty code")
		}
		if !seen[code] {
			seen[code] = true
			out = append(out, code)
		}
	}
	return out, nil
}

func (o *OpenLDAP) ensureUnique(conn *ldap.Conn, baseDN, attr, value string) error {
	filter := fmt.Sprintf("(%s=%s)", attr, ldap.EscapeFilter(value))
	req := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		1, 0, false,
		filter,
		[]string{"dn"},
		nil,
	)
	res, err := conn.Search(req)
	if err != nil {
		return err
	}
	if len(res.Entries) > 0 {
		return LdapDuplicateError{}
	}
	return nil
}

func (o *OpenLDAP) getEntryByDN(conn *ldap.Conn, dn string, attrs []string) (*ldap.Entry, error) {
	req := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1, 0, false,
		"(objectClass=*)",
		attrs,
		nil,
	)
	res, err := conn.Search(req)
	if err != nil {
		return nil, ldapErr(err, "Entry", dn)
	}
	if len(res.Entries) == 0 {
		return nil, LdapNotFoundError{Type: "Entry", ID: dn}
	}
	return res.Entries[0], nil
}

func (o *OpenLDAP) ensureDNExists(conn *ldap.Conn, dn string) error {
	if strings.TrimSpace(dn) == "" {
		return nil
	}
	_, err := o.getEntryByDN(conn, dn, []string{"dn"})
	return err
}

func (o *OpenLDAP) ensureBaseEntryExists(conn *ldap.Conn, dn string) error {
	if strings.TrimSpace(dn) == "" {
		return nil
	}
	if err := o.ensureDNExists(conn, dn); err == nil {
		return nil
	} else if _, ok := err.(LdapNotFoundError); !ok {
		return err
	}
	parts := strings.SplitN(dn, ",", 2)
	if len(parts) != 2 {
		return LdapParamsError{Params: []string{"dn"}}
	}
	rdn := strings.SplitN(strings.TrimSpace(parts[0]), "=", 2)
	if len(rdn) != 2 {
		return LdapParamsError{Params: []string{"dn"}}
	}
	attrName := strings.TrimSpace(rdn[0])
	attrValue := strings.TrimSpace(rdn[1])
	parentDN := strings.TrimSpace(parts[1])
	if err := o.ensureDNExists(conn, parentDN); err != nil {
		return err
	}
	req := ldap.NewAddRequest(dn, nil)
	req.Attribute("objectClass", []string{objectClassDept})
	switch attrName {
	case attrOU:
		req.Attribute(attrOU, []string{attrValue})
	case attrCN:
		req.Attribute(attrCN, []string{attrValue})
	default:
		return LdapParamsError{Params: []string{"dn"}}
	}
	return ldapErr(conn.Add(req), "Entry", dn)
}

func (o *OpenLDAP) ensureEntryExistsByAttr(conn *ldap.Conn, baseDN, attr, value, typ string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return LdapParamsError{Params: []string{attr}}
	}
	filter := fmt.Sprintf("(%s=%s)", attr, ldap.EscapeFilter(value))
	req := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		1, 0, false,
		filter,
		[]string{"dn"},
		nil,
	)
	res, err := conn.Search(req)
	if err != nil {
		return err
	}
	if len(res.Entries) == 0 {
		return LdapNotFoundError{Type: typ, ID: value}
	}
	return nil
}

func ldapErr(err error, typ string, id interface{}) error {
	if err == nil {
		return nil
	}
	if le, ok := err.(*ldap.Error); ok {
		switch le.ResultCode {
		case ldap.LDAPResultEntryAlreadyExists:
			return LdapDuplicateError{}
		case ldap.LDAPResultNoSuchObject:
			return LdapNotFoundError{Type: typ, ID: id}
		}
	}
	return err
}

func normalizeOrgUser(u *OrgUser) (*OrgUser, error) {
	if u == nil || strings.TrimSpace(u.UID) == "" {
		return nil, LdapParamsError{Params: []string{"uid"}}
	}
	u.UID = strings.TrimSpace(u.UID)
	if u.DisplayName == "" {
		u.DisplayName = strings.TrimSpace(strings.Join([]string{u.FirstName, u.LastName}, " "))
	}
	if u.DisplayName == "" {
		return nil, LdapParamsError{Params: []string{"displayName"}}
	}
	if u.LastName == "" {
		u.LastName = u.DisplayName
	}
	var err error
	u.DeptCodes, err = normalizeCodes(u.DeptCodes)
	if err != nil {
		return nil, LdapParamsError{Params: []string{"deptCodes"}}
	}
	u.PositionCodes, err = normalizeCodes(u.PositionCodes)
	if err != nil {
		return nil, LdapParamsError{Params: []string{"positionCodes"}}
	}
	u.Gender = strings.TrimSpace(u.Gender)
	if u.Gender != "" {
		u.Gender = strings.ToUpper(u.Gender)
		switch u.Gender {
		case "M", "F", "O":
		default:
			return nil, LdapParamsError{Params: []string{"gender"}}
		}
	}
	u.Birthday = strings.TrimSpace(u.Birthday)
	if u.Birthday != "" {
		if _, err := time.Parse("2006-01-02", u.Birthday); err != nil {
			return nil, LdapParamsError{Params: []string{"birthday"}}
		}
	}
	return u, nil
}

func normalizeDepartment(d *Department) (*Department, error) {
	if d == nil || strings.TrimSpace(d.Code) == "" {
		return nil, LdapParamsError{Params: []string{"code"}}
	}
	d.Code = strings.TrimSpace(d.Code)
	if d.Name == "" {
		d.Name = d.Code
	}
	if err := validateDN(d.ParentDN); err != nil {
		return nil, LdapParamsError{Params: []string{"parentDn"}}
	}
	if err := validateDN(d.ManagerDN); err != nil {
		return nil, LdapParamsError{Params: []string{"managerDn"}}
	}
	return d, nil
}

func normalizePosition(p *Position) (*Position, error) {
	if p == nil || strings.TrimSpace(p.Code) == "" {
		return nil, LdapParamsError{Params: []string{"code"}}
	}
	p.Code = strings.TrimSpace(p.Code)
	if p.Name == "" {
		p.Name = p.Code
	}
	if err := validateDN(p.DeptDN); err != nil {
		return nil, LdapParamsError{Params: []string{"deptDn"}}
	}
	return p, nil
}

func normalizeGroup(g *Group) (*Group, error) {
	if g == nil || strings.TrimSpace(g.Code) == "" {
		return nil, LdapParamsError{Params: []string{"code"}}
	}
	g.Code = strings.TrimSpace(g.Code)
	if g.Name == "" {
		g.Name = g.Code
	}
	g.Name = strings.TrimSpace(g.Name)
	memberDNs := make([]string, 0, len(g.MemberDNs))
	seen := make(map[string]struct{}, len(g.MemberDNs))
	for _, value := range g.MemberDNs {
		dn := strings.TrimSpace(value)
		if dn == "" {
			continue
		}
		if err := validateDN(dn); err != nil {
			return nil, LdapParamsError{Params: []string{"memberDns"}}
		}
		if _, ok := seen[dn]; ok {
			continue
		}
		seen[dn] = struct{}{}
		memberDNs = append(memberDNs, dn)
	}
	g.MemberDNs = memberDNs
	return g, nil
}

func (o *OpenLDAP) userDN(uid string) string {
	return fmt.Sprintf("uid=%s,%s", uid, o.cfg.peopleDN())
}

func (o *OpenLDAP) departmentDN(code string) string {
	return fmt.Sprintf("%s=%s,%s", attrOU, code, o.cfg.deptDN())
}

func (o *OpenLDAP) positionDN(code string) string {
	return fmt.Sprintf("%s=%s,%s", attrCN, code, o.cfg.positionDN())
}

func (o *OpenLDAP) groupDN(code string) string {
	return fmt.Sprintf("%s=%s,%s", attrCN, code, o.cfg.groupDN())
}

func isDescendantDN(child, base string) bool {
	child = strings.ToLower(strings.TrimSpace(child))
	base = strings.ToLower(strings.TrimSpace(base))
	if child == "" || base == "" {
		return false
	}
	if child == base {
		return true
	}
	if strings.HasSuffix(child, ","+base) {
		return true
	}
	return false
}

func (o *OpenLDAP) GetUser(ctx context.Context, uid string) (*OrgUser, error) {
	if strings.TrimSpace(uid) == "" {
		return nil, LdapParamsError{Params: []string{"uid"}}
	}
	uid = strings.TrimSpace(uid)
	var out *OrgUser
	err := o.withConn(func(conn *ldap.Conn) error {
		entry, err := o.getEntryByDN(conn, o.userDN(uid), []string{
			attrUID, attrCN, attrSN, attrGivenName, attrMail, attrEmployeeNumber, attrTelephoneNumber, attrPostalAddress,
			attrDNQualifier, attrEmployeeType, attrTitle,
			attrBusinessCategory, attrDepartmentNumber,
		})
		if err != nil {
			return err
		}
		out = entryToOrgUser(entry)
		return nil
	})
	return out, err
}

func (o *OpenLDAP) GetUserByDN(ctx context.Context, dn string) (*OrgUser, error) {
	dn = strings.TrimSpace(dn)
	if dn == "" {
		return nil, LdapParamsError{Params: []string{"dn"}}
	}
	var out *OrgUser
	err := o.withConn(func(conn *ldap.Conn) error {
		entry, err := o.getEntryByDN(conn, dn, []string{
			attrUID, attrCN, attrSN, attrGivenName, attrMail, attrEmployeeNumber, attrTelephoneNumber, attrPostalAddress,
			attrDNQualifier, attrEmployeeType, attrTitle,
			attrBusinessCategory, attrDepartmentNumber,
		})
		if err != nil {
			return err
		}
		out = entryToOrgUser(entry)
		return nil
	})
	return out, err
}

func (o *OpenLDAP) AddUser(ctx context.Context, u *OrgUser) error {
	nu, err := normalizeOrgUser(u)
	if err != nil {
		return err
	}
	return o.withConn(func(conn *ldap.Conn) error {
		if err := o.ensureUnique(conn, o.cfg.peopleDN(), attrUID, nu.UID); err != nil {
			return err
		}
		for _, code := range nu.DeptCodes {
			if err := o.ensureEntryExistsByAttr(conn, o.cfg.deptDN(), attrOU, code, "Department"); err != nil {
				return err
			}
		}
		for _, code := range nu.PositionCodes {
			if err := o.ensureEntryExistsByAttr(conn, o.cfg.positionDN(), attrCN, code, "Position"); err != nil {
				return err
			}
		}

		req := ldap.NewAddRequest(o.userDN(nu.UID), nil)
		req.Attribute("objectClass", []string{objectClassPerson, objectClassExtensible})
		req.Attribute(attrUID, []string{nu.UID})
		req.Attribute(attrCN, []string{nu.DisplayName})
		req.Attribute(attrSN, []string{nu.LastName})
		if nu.FirstName != "" {
			req.Attribute(attrGivenName, []string{nu.FirstName})
		}
		if nu.Email != "" {
			req.Attribute(attrMail, []string{nu.Email})
		}
		if nu.Phone != "" {
			req.Attribute(attrTelephoneNumber, []string{nu.Phone})
		}
		if nu.Address != "" {
			req.Attribute(attrPostalAddress, []string{nu.Address})
		}
		if nu.Birthday != "" {
			req.Attribute(attrDNQualifier, []string{nu.Birthday})
		}
		if nu.Gender != "" {
			req.Attribute(attrEmployeeType, []string{nu.Gender})
		}
		if nu.EmployeeNo != "" {
			req.Attribute(attrEmployeeNumber, []string{nu.EmployeeNo})
		}
		if nu.Password != "" {
			req.Attribute(attrUserPassword, []string{nu.Password})
		}
		if nu.Status != "" {
			req.Attribute(attrBusinessCategory, []string{nu.Status})
		}
		if len(nu.DeptCodes) > 0 {
			req.Attribute(attrDepartmentNumber, nu.DeptCodes)
		}
		if len(nu.PositionCodes) > 0 {
			req.Attribute(attrTitle, nu.PositionCodes)
		}
		return ldapErr(conn.Add(req), "User", nu.UID)
	})
}

func (o *OpenLDAP) UpdateUser(ctx context.Context, u *OrgUser) error {
	nu, err := normalizeOrgUser(u)
	if err != nil {
		return err
	}
	return o.withConn(func(conn *ldap.Conn) error {
		if nu.Birthday != "" {
			if err := o.ensureUserObjectClass(conn, nu.UID, objectClassExtensible); err != nil {
				return err
			}
		}
		entry, err := o.getEntryByDN(conn, o.userDN(nu.UID), []string{
			attrGivenName, attrMail, attrTelephoneNumber, attrPostalAddress, attrDNQualifier,
			attrEmployeeType, attrEmployeeNumber, attrBusinessCategory, attrDepartmentNumber, attrTitle,
		})
		if err != nil {
			return err
		}
		hasAttr := func(attr string) bool {
			return len(entry.GetAttributeValues(attr)) > 0
		}
		if nu.DeptCodes != nil {
			for _, code := range nu.DeptCodes {
				if err := o.ensureEntryExistsByAttr(conn, o.cfg.deptDN(), attrOU, code, "Department"); err != nil {
					return err
				}
			}
		}
		if nu.PositionCodes != nil {
			for _, code := range nu.PositionCodes {
				if err := o.ensureEntryExistsByAttr(conn, o.cfg.positionDN(), attrCN, code, "Position"); err != nil {
					return err
				}
			}
		}

		req := ldap.NewModifyRequest(o.userDN(nu.UID), nil)
		req.Replace(attrCN, []string{nu.DisplayName})
		req.Replace(attrSN, []string{nu.LastName})

		if nu.FirstName != "" {
			req.Replace(attrGivenName, []string{nu.FirstName})
		} else if hasAttr(attrGivenName) {
			req.Delete(attrGivenName, nil)
		}
		if nu.Email != "" {
			req.Replace(attrMail, []string{nu.Email})
		} else if hasAttr(attrMail) {
			req.Delete(attrMail, nil)
		}
		if nu.Phone != "" {
			req.Replace(attrTelephoneNumber, []string{nu.Phone})
		} else if hasAttr(attrTelephoneNumber) {
			req.Delete(attrTelephoneNumber, nil)
		}
		if nu.Address != "" {
			req.Replace(attrPostalAddress, []string{nu.Address})
		} else if hasAttr(attrPostalAddress) {
			req.Delete(attrPostalAddress, nil)
		}
		if nu.Birthday != "" {
			req.Replace(attrDNQualifier, []string{nu.Birthday})
		} else if hasAttr(attrDNQualifier) {
			req.Delete(attrDNQualifier, nil)
		}
		if nu.Gender != "" {
			req.Replace(attrEmployeeType, []string{nu.Gender})
		} else if hasAttr(attrEmployeeType) {
			req.Delete(attrEmployeeType, nil)
		}
		if nu.EmployeeNo != "" {
			req.Replace(attrEmployeeNumber, []string{nu.EmployeeNo})
		} else if hasAttr(attrEmployeeNumber) {
			req.Delete(attrEmployeeNumber, nil)
		}
		if nu.Password != "" {
			req.Replace(attrUserPassword, []string{nu.Password})
		}
		if nu.Status != "" {
			req.Replace(attrBusinessCategory, []string{nu.Status})
		} else if hasAttr(attrBusinessCategory) {
			req.Delete(attrBusinessCategory, nil)
		}

		if nu.DeptCodes != nil {
			if len(nu.DeptCodes) == 0 {
				if hasAttr(attrDepartmentNumber) {
					req.Delete(attrDepartmentNumber, nil)
				}
			} else {
				req.Replace(attrDepartmentNumber, nu.DeptCodes)
			}
		}
		if nu.PositionCodes != nil {
			if len(nu.PositionCodes) == 0 {
				if hasAttr(attrTitle) {
					req.Delete(attrTitle, nil)
				}
			} else {
				req.Replace(attrTitle, nu.PositionCodes)
			}
		}
		return ldapErr(conn.Modify(req), "User", nu.UID)
	})
}

func (o *OpenLDAP) DeleteUser(ctx context.Context, uid string) error {
	if strings.TrimSpace(uid) == "" {
		return LdapParamsError{Params: []string{"uid"}}
	}
	uid = strings.TrimSpace(uid)
	return o.withConn(func(conn *ldap.Conn) error {
		req := ldap.NewDelRequest(o.userDN(uid), nil)
		return ldapErr(conn.Del(req), "User", uid)
	})
}

func (o *OpenLDAP) ListUser(ctx context.Context) ([]*OrgUser, error) {
	var out []*OrgUser
	err := o.withConn(func(conn *ldap.Conn) error {
		req := ldap.NewSearchRequest(
			o.cfg.peopleDN(),
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0, 0, false,
			fmt.Sprintf("(objectClass=%s)", objectClassPerson),
			[]string{
				attrUID, attrCN, attrSN, attrGivenName, attrMail, attrEmployeeNumber, attrTelephoneNumber, attrPostalAddress,
				attrDNQualifier, attrEmployeeType, attrTitle,
				attrBusinessCategory, attrDepartmentNumber,
			},
			nil,
		)
		res, err := conn.Search(req)
		if err != nil {
			return err
		}
		out = make([]*OrgUser, 0, len(res.Entries))
		for _, entry := range res.Entries {
			out = append(out, entryToOrgUser(entry))
		}
		return nil
	})
	return out, err
}

func entryToOrgUser(entry *ldap.Entry) *OrgUser {
	if entry == nil {
		return nil
	}
	return &OrgUser{
		DN:            entry.DN,
		UID:           entry.GetAttributeValue(attrUID),
		Email:         entry.GetAttributeValue(attrMail),
		Phone:         entry.GetAttributeValue(attrTelephoneNumber),
		Address:       entry.GetAttributeValue(attrPostalAddress),
		Gender:        entry.GetAttributeValue(attrEmployeeType),
		Birthday:      entry.GetAttributeValue(attrDNQualifier),
		DisplayName:   entry.GetAttributeValue(attrCN),
		FirstName:     entry.GetAttributeValue(attrGivenName),
		LastName:      entry.GetAttributeValue(attrSN),
		EmployeeNo:    entry.GetAttributeValue(attrEmployeeNumber),
		Status:        entry.GetAttributeValue(attrBusinessCategory),
		DeptCodes:     entry.GetAttributeValues(attrDepartmentNumber),
		PositionCodes: entry.GetAttributeValues(attrTitle),
	}
}

func (o *OpenLDAP) ensureUserObjectClass(conn *ldap.Conn, uid, objectClass string) error {
	entry, err := o.getEntryByDN(conn, o.userDN(uid), []string{"objectClass"})
	if err != nil {
		return err
	}
	for _, oc := range entry.GetAttributeValues("objectClass") {
		if strings.EqualFold(oc, objectClass) {
			return nil
		}
	}
	req := ldap.NewModifyRequest(o.userDN(uid), nil)
	req.Add("objectClass", []string{objectClass})
	return ldapErr(conn.Modify(req), "User", uid)
}

func (o *OpenLDAP) GetDepartment(ctx context.Context, code string) (*Department, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, LdapParamsError{Params: []string{"code"}}
	}
	var out *Department
	err := o.withConn(func(conn *ldap.Conn) error {
		entry, err := o.getEntryByDN(conn, o.departmentDN(code), []string{
			attrOU, attrDescription, attrSeeAlso,
		})
		if err != nil {
			return err
		}
		out = o.entryToDepartment(entry)
		return nil
	})
	return out, err
}

func (o *OpenLDAP) GetDepartmentByDN(ctx context.Context, dn string) (*Department, error) {
	dn = strings.TrimSpace(dn)
	if dn == "" {
		return nil, LdapParamsError{Params: []string{"dn"}}
	}
	var out *Department
	err := o.withConn(func(conn *ldap.Conn) error {
		entry, err := o.getEntryByDN(conn, dn, []string{
			attrOU, attrDescription, attrSeeAlso,
		})
		if err != nil {
			return err
		}
		out = o.entryToDepartment(entry)
		return nil
	})
	return out, err
}

func (o *OpenLDAP) AddDepartment(ctx context.Context, d *Department) error {
	nd, err := normalizeDepartment(d)
	if err != nil {
		return err
	}
	if baseCode := o.deptBaseCode(); baseCode != "" && strings.EqualFold(nd.Code, baseCode) {
		return LdapParamsError{Params: []string{"code"}}
	}
	return o.withConn(func(conn *ldap.Conn) error {
		if err := o.ensureUnique(conn, o.cfg.deptDN(), attrOU, nd.Code); err != nil {
			return err
		}
		if err := o.ensureDNExists(conn, nd.ParentDN); err != nil {
			return err
		}
		if err := o.ensureDNExists(conn, nd.ManagerDN); err != nil {
			return err
		}
		req := ldap.NewAddRequest(o.departmentDN(nd.Code), nil)
		req.Attribute("objectClass", []string{objectClassDept})
		req.Attribute(attrOU, []string{nd.Code})
		req.Attribute(attrDescription, []string{nd.Name})
		seeAlso := []string{}
		if nd.ParentDN != "" {
			seeAlso = append(seeAlso, nd.ParentDN)
		}
		if nd.ManagerDN != "" {
			seeAlso = append(seeAlso, nd.ManagerDN)
		}
		if len(seeAlso) > 0 {
			req.Attribute(attrSeeAlso, seeAlso)
		}
		return ldapErr(conn.Add(req), "Department", nd.Code)
	})
}

func (o *OpenLDAP) UpdateDepartment(ctx context.Context, d *Department) error {
	nd, err := normalizeDepartment(d)
	if err != nil {
		return err
	}
	if baseCode := o.deptBaseCode(); baseCode != "" && strings.EqualFold(nd.Code, baseCode) {
		return LdapParamsError{Params: []string{"code"}}
	}
	return o.withConn(func(conn *ldap.Conn) error {
		if err := o.ensureDNExists(conn, nd.ParentDN); err != nil {
			return err
		}
		if err := o.ensureDNExists(conn, nd.ManagerDN); err != nil {
			return err
		}
		req := ldap.NewModifyRequest(o.departmentDN(nd.Code), nil)
		req.Replace(attrDescription, []string{nd.Name})
		seeAlso := []string{}
		if nd.ParentDN != "" {
			seeAlso = append(seeAlso, nd.ParentDN)
		}
		if nd.ManagerDN != "" {
			seeAlso = append(seeAlso, nd.ManagerDN)
		}
		if len(seeAlso) > 0 {
			req.Replace(attrSeeAlso, seeAlso)
		} else {
			req.Delete(attrSeeAlso, nil)
		}
		return ldapErr(conn.Modify(req), "Department", nd.Code)
	})
}

func (o *OpenLDAP) DeleteDepartment(ctx context.Context, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return LdapParamsError{Params: []string{"code"}}
	}
	return o.withConn(func(conn *ldap.Conn) error {
		req := ldap.NewDelRequest(o.departmentDN(code), nil)
		return ldapErr(conn.Del(req), "Department", code)
	})
}

func (o *OpenLDAP) ListDepartment(ctx context.Context) ([]*Department, error) {
	var out []*Department
	err := o.withConn(func(conn *ldap.Conn) error {
		req := ldap.NewSearchRequest(
			o.cfg.deptDN(),
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0, 0, false,
			fmt.Sprintf("(objectClass=%s)", objectClassDept),
			[]string{attrOU, attrDescription, attrSeeAlso},
			nil,
		)
		res, err := conn.Search(req)
		if err != nil {
			return err
		}
		baseDN := strings.ToLower(o.cfg.deptDN())
		out = make([]*Department, 0, len(res.Entries))
		for _, entry := range res.Entries {
			if entry == nil {
				continue
			}
			if strings.ToLower(entry.DN) == baseDN {
				continue
			}
			out = append(out, o.entryToDepartment(entry))
		}
		return nil
	})
	return out, err
}

func (o *OpenLDAP) entryToDepartment(entry *ldap.Entry) *Department {
	if entry == nil {
		return nil
	}
	parentDN := ""
	managerDN := ""
	for _, dn := range entry.GetAttributeValues(attrSeeAlso) {
		switch {
		case isDescendantDN(dn, o.cfg.deptDN()):
			if parentDN == "" {
				parentDN = dn
			}
		case isDescendantDN(dn, o.cfg.peopleDN()):
			if managerDN == "" {
				managerDN = dn
			}
		default:
			if parentDN == "" {
				parentDN = dn
			} else if managerDN == "" {
				managerDN = dn
			}
		}
	}
	return &Department{
		DN:        entry.DN,
		Code:      entry.GetAttributeValue(attrOU),
		Name:      entry.GetAttributeValue(attrDescription),
		ParentDN:  parentDN,
		ManagerDN: managerDN,
		Status:    "",
	}
}

func (o *OpenLDAP) deptBaseCode() string {
	dn := o.cfg.deptDN()
	parsed, err := ldap.ParseDN(dn)
	if err != nil || len(parsed.RDNs) == 0 || len(parsed.RDNs[0].Attributes) == 0 {
		return ""
	}
	for _, attr := range parsed.RDNs[0].Attributes {
		if strings.EqualFold(attr.Type, attrOU) {
			return attr.Value
		}
	}
	return ""
}

func (o *OpenLDAP) GetPosition(ctx context.Context, code string) (*Position, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, LdapParamsError{Params: []string{"code"}}
	}
	var out *Position
	err := o.withConn(func(conn *ldap.Conn) error {
		entry, err := o.getEntryByDN(conn, o.positionDN(code), []string{
			attrCN, attrDescription, attrSeeAlso,
		})
		if err != nil {
			return err
		}
		out = entryToPosition(entry)
		return nil
	})
	return out, err
}

func (o *OpenLDAP) GetPositionByDN(ctx context.Context, dn string) (*Position, error) {
	dn = strings.TrimSpace(dn)
	if dn == "" {
		return nil, LdapParamsError{Params: []string{"dn"}}
	}
	var out *Position
	err := o.withConn(func(conn *ldap.Conn) error {
		entry, err := o.getEntryByDN(conn, dn, []string{
			attrCN, attrDescription, attrSeeAlso,
		})
		if err != nil {
			return err
		}
		out = entryToPosition(entry)
		return nil
	})
	return out, err
}

func (o *OpenLDAP) AddPosition(ctx context.Context, p *Position) error {
	np, err := normalizePosition(p)
	if err != nil {
		return err
	}
	return o.withConn(func(conn *ldap.Conn) error {
		if err := o.ensureUnique(conn, o.cfg.positionDN(), attrCN, np.Code); err != nil {
			return err
		}
		if err := o.ensureDNExists(conn, np.DeptDN); err != nil {
			return err
		}
		req := ldap.NewAddRequest(o.positionDN(np.Code), nil)
		req.Attribute("objectClass", []string{objectClassPosition})
		req.Attribute(attrCN, []string{np.Code})
		req.Attribute(attrDescription, []string{np.Name})
		if np.DeptDN != "" {
			req.Attribute(attrSeeAlso, []string{np.DeptDN})
		}
		return ldapErr(conn.Add(req), "Position", np.Code)
	})
}

func (o *OpenLDAP) UpdatePosition(ctx context.Context, p *Position) error {
	np, err := normalizePosition(p)
	if err != nil {
		return err
	}
	return o.withConn(func(conn *ldap.Conn) error {
		if err := o.ensureDNExists(conn, np.DeptDN); err != nil {
			return err
		}
		req := ldap.NewModifyRequest(o.positionDN(np.Code), nil)
		req.Replace(attrDescription, []string{np.Name})
		if np.DeptDN != "" {
			req.Replace(attrSeeAlso, []string{np.DeptDN})
		} else {
			req.Delete(attrSeeAlso, nil)
		}
		return ldapErr(conn.Modify(req), "Position", np.Code)
	})
}

func (o *OpenLDAP) DeletePosition(ctx context.Context, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return LdapParamsError{Params: []string{"code"}}
	}
	return o.withConn(func(conn *ldap.Conn) error {
		req := ldap.NewDelRequest(o.positionDN(code), nil)
		return ldapErr(conn.Del(req), "Position", code)
	})
}

func (o *OpenLDAP) ListPosition(ctx context.Context) ([]*Position, error) {
	var out []*Position
	err := o.withConn(func(conn *ldap.Conn) error {
		req := ldap.NewSearchRequest(
			o.cfg.positionDN(),
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0, 0, false,
			fmt.Sprintf("(objectClass=%s)", objectClassPosition),
			[]string{attrCN, attrDescription, attrSeeAlso},
			nil,
		)
		res, err := conn.Search(req)
		if err != nil {
			return err
		}
		out = make([]*Position, 0, len(res.Entries))
		for _, entry := range res.Entries {
			out = append(out, entryToPosition(entry))
		}
		return nil
	})
	return out, err
}

func entryToPosition(entry *ldap.Entry) *Position {
	if entry == nil {
		return nil
	}
	return &Position{
		DN:     entry.DN,
		Code:   entry.GetAttributeValue(attrCN),
		Name:   entry.GetAttributeValue(attrDescription),
		DeptDN: entry.GetAttributeValue(attrSeeAlso),
		Status: "",
	}
}

func (o *OpenLDAP) GetGroup(ctx context.Context, code string) (*Group, error) {
	if strings.TrimSpace(code) == "" {
		return nil, LdapParamsError{Params: []string{"code"}}
	}
	var out *Group
	err := o.withConn(func(conn *ldap.Conn) error {
		entry, err := o.getEntryByDN(conn, o.groupDN(strings.TrimSpace(code)), []string{
			attrCN, attrDescription, attrUniqueMember,
		})
		if err != nil {
			return err
		}
		out = entryToGroup(entry)
		return nil
	})
	return out, err
}

func (o *OpenLDAP) GetGroupByDN(ctx context.Context, dn string) (*Group, error) {
	dn = strings.TrimSpace(dn)
	if dn == "" {
		return nil, LdapParamsError{Params: []string{"dn"}}
	}
	var out *Group
	err := o.withConn(func(conn *ldap.Conn) error {
		entry, err := o.getEntryByDN(conn, dn, []string{
			attrCN, attrDescription, attrUniqueMember,
		})
		if err != nil {
			return err
		}
		out = entryToGroup(entry)
		return nil
	})
	return out, err
}

func (o *OpenLDAP) AddGroup(ctx context.Context, group *Group) error {
	ng, err := normalizeGroup(group)
	if err != nil {
		return err
	}
	if len(ng.MemberDNs) == 0 {
		return LdapParamsError{Params: []string{"memberDns"}}
	}
	return o.withConn(func(conn *ldap.Conn) error {
		if err := o.ensureBaseEntryExists(conn, o.cfg.groupDN()); err != nil {
			return err
		}
		if err := o.ensureUnique(conn, o.cfg.groupDN(), attrCN, ng.Code); err != nil {
			return err
		}
		for _, memberDN := range ng.MemberDNs {
			if err := o.ensureDNExists(conn, memberDN); err != nil {
				return err
			}
		}
		req := ldap.NewAddRequest(o.groupDN(ng.Code), nil)
		req.Attribute("objectClass", []string{objectClassGroup})
		req.Attribute(attrCN, []string{ng.Code})
		req.Attribute(attrDescription, []string{ng.Name})
		req.Attribute(attrUniqueMember, ng.MemberDNs)
		return ldapErr(conn.Add(req), "Group", ng.Code)
	})
}

func (o *OpenLDAP) UpdateGroup(ctx context.Context, group *Group) error {
	ng, err := normalizeGroup(group)
	if err != nil {
		return err
	}
	if len(ng.MemberDNs) == 0 {
		return LdapParamsError{Params: []string{"memberDns"}}
	}
	return o.withConn(func(conn *ldap.Conn) error {
		if err := o.ensureBaseEntryExists(conn, o.cfg.groupDN()); err != nil {
			return err
		}
		for _, memberDN := range ng.MemberDNs {
			if err := o.ensureDNExists(conn, memberDN); err != nil {
				return err
			}
		}
		req := ldap.NewModifyRequest(o.groupDN(ng.Code), nil)
		req.Replace(attrDescription, []string{ng.Name})
		req.Replace(attrUniqueMember, ng.MemberDNs)
		return ldapErr(conn.Modify(req), "Group", ng.Code)
	})
}

func (o *OpenLDAP) DeleteGroup(ctx context.Context, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return LdapParamsError{Params: []string{"code"}}
	}
	return o.withConn(func(conn *ldap.Conn) error {
		req := ldap.NewDelRequest(o.groupDN(code), nil)
		return ldapErr(conn.Del(req), "Group", code)
	})
}

func (o *OpenLDAP) ListGroup(ctx context.Context) ([]*Group, error) {
	var out []*Group
	err := o.withConn(func(conn *ldap.Conn) error {
		if err := o.ensureBaseEntryExists(conn, o.cfg.groupDN()); err != nil {
			return err
		}
		req := ldap.NewSearchRequest(
			o.cfg.groupDN(),
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0, 0, false,
			fmt.Sprintf("(objectClass=%s)", objectClassGroup),
			[]string{attrCN, attrDescription, attrUniqueMember},
			nil,
		)
		res, err := conn.Search(req)
		if err != nil {
			return err
		}
		out = make([]*Group, 0, len(res.Entries))
		for _, entry := range res.Entries {
			out = append(out, entryToGroup(entry))
		}
		return nil
	})
	return out, err
}

func entryToGroup(entry *ldap.Entry) *Group {
	if entry == nil {
		return nil
	}
	return &Group{
		DN:        entry.DN,
		Code:      entry.GetAttributeValue(attrCN),
		Name:      entry.GetAttributeValue(attrDescription),
		MemberDNs: entry.GetAttributeValues(attrUniqueMember),
	}
}

func (o *OpenLDAP) AddUserDepartments(ctx context.Context, uid string, deptCodes []string) error {
	return o.updateUserMultiAttr(uid, attrDepartmentNumber, deptCodes, nil, func(conn *ldap.Conn, code string) error {
		return o.ensureEntryExistsByAttr(conn, o.cfg.deptDN(), attrOU, code, "Department")
	})
}

func (o *OpenLDAP) RemoveUserDepartments(ctx context.Context, uid string, deptCodes []string) error {
	return o.updateUserMultiAttr(uid, attrDepartmentNumber, nil, deptCodes, nil)
}

func (o *OpenLDAP) AddUserPositions(ctx context.Context, uid string, positionCodes []string) error {
	return o.updateUserMultiAttr(uid, attrTitle, positionCodes, nil, func(conn *ldap.Conn, code string) error {
		return o.ensureEntryExistsByAttr(conn, o.cfg.positionDN(), attrCN, code, "Position")
	})
}

func (o *OpenLDAP) RemoveUserPositions(ctx context.Context, uid string, positionCodes []string) error {
	return o.updateUserMultiAttr(uid, attrTitle, nil, positionCodes, nil)
}

func (o *OpenLDAP) ValidateUser(ctx context.Context, phone, password string) (*OrgUser, error) {
	phone = strings.TrimSpace(phone)
	password = strings.TrimSpace(password)
	missing := []string{}
	if phone == "" {
		missing = append(missing, "phone")
	}
	if password == "" {
		missing = append(missing, "password")
	}
	if len(missing) > 0 {
		return nil, LdapParamsError{Params: missing}
	}

	var out *OrgUser
	err := o.withConn(func(conn *ldap.Conn) error {
		filter := fmt.Sprintf("(%s=%s)", attrTelephoneNumber, ldap.EscapeFilter(phone))
		req := ldap.NewSearchRequest(
			o.cfg.peopleDN(),
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			2, 0, false,
			filter,
			[]string{
				attrUID, attrCN, attrSN, attrGivenName, attrMail, attrEmployeeNumber, attrTelephoneNumber, attrPostalAddress,
				attrDNQualifier, attrEmployeeType, attrTitle,
				attrBusinessCategory, attrDepartmentNumber,
			},
			nil,
		)
		res, err := conn.Search(req)
		if err != nil {
			return err
		}
		if len(res.Entries) == 0 {
			return LdapNotFoundError{Type: "User", ID: phone}
		}
		if len(res.Entries) > 1 {
			return LdapDuplicateError{}
		}

		entry := res.Entries[0]
		authConn, err := ldap.DialURL(o.cfg.url())
		if err != nil {
			return err
		}
		defer authConn.Close()

		if err := authConn.Bind(entry.DN, password); err != nil {
			return err
		}

		out = entryToOrgUser(entry)
		return nil
	})
	return out, err
}

func (o *OpenLDAP) ValidateUserByUID(ctx context.Context, uid, password string) (*OrgUser, error) {
	uid = strings.TrimSpace(uid)
	password = strings.TrimSpace(password)
	missing := []string{}
	if uid == "" {
		missing = append(missing, "uid")
	}
	if password == "" {
		missing = append(missing, "password")
	}
	if len(missing) > 0 {
		return nil, LdapParamsError{Params: missing}
	}

	var out *OrgUser
	err := o.withConn(func(conn *ldap.Conn) error {
		entry, err := o.getEntryByDN(conn, o.userDN(uid), []string{
			attrUID, attrCN, attrSN, attrGivenName, attrMail, attrEmployeeNumber, attrTelephoneNumber, attrPostalAddress,
			attrDNQualifier, attrEmployeeType, attrTitle,
			attrBusinessCategory, attrDepartmentNumber,
		})
		if err != nil {
			return err
		}

		authConn, err := ldap.DialURL(o.cfg.url())
		if err != nil {
			return err
		}
		defer authConn.Close()

		if err := authConn.Bind(entry.DN, password); err != nil {
			return err
		}

		out = entryToOrgUser(entry)
		return nil
	})
	return out, err
}

func (o *OpenLDAP) updateUserMultiAttr(uid, attr string, addValues, removeValues []string, validateFn func(conn *ldap.Conn, code string) error) error {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return LdapParamsError{Params: []string{"uid"}}
	}
	if len(addValues) == 0 && len(removeValues) == 0 {
		return nil
	}
	var err error
	addValues, err = normalizeCodes(addValues)
	if err != nil {
		return LdapParamsError{Params: []string{attr}}
	}
	removeValues, err = normalizeCodes(removeValues)
	if err != nil {
		return LdapParamsError{Params: []string{attr}}
	}
	return o.withConn(func(conn *ldap.Conn) error {
		if validateFn != nil {
			for _, code := range addValues {
				if err := validateFn(conn, code); err != nil {
					return err
				}
			}
		}
		entry, err := o.getEntryByDN(conn, o.userDN(uid), []string{attr})
		if err != nil {
			return err
		}
		current := map[string]bool{}
		for _, v := range entry.GetAttributeValues(attr) {
			current[v] = true
		}
		for _, v := range addValues {
			current[v] = true
		}
		for _, v := range removeValues {
			delete(current, v)
		}
		values := make([]string, 0, len(current))
		for v := range current {
			values = append(values, v)
		}
		req := ldap.NewModifyRequest(o.userDN(uid), nil)
		if len(values) == 0 {
			req.Delete(attr, nil)
		} else {
			req.Replace(attr, values)
		}
		return ldapErr(conn.Modify(req), "User", uid)
	})
}
