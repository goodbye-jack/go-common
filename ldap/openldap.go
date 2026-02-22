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
	ldapAddrKey          = "ldap_addr"
	ldapBindDNKey        = "ldap_bind_dn"
	ldapBindPasswordKey  = "ldap_bind_password"
	ldapBaseDNKey        = "ldap_base_dn"
	ldapPeopleOUKey      = "ldap_people_ou"
	ldapDepartmentOUKey  = "ldap_department_ou"
	ldapPositionOUKey    = "ldap_position_ou"
	ldapUseTLSKey        = "ldap_use_tls"
	defaultPeopleOU      = "ou=people"
	defaultDepartmentOU  = "ou=departments"
	defaultPositionOU    = "ou=positions"
	attrUID              = "uid"
	attrCN               = "cn"
	attrOU               = "ou"
	attrSN               = "sn"
	attrGivenName        = "givenName"
	attrMail             = "mail"
	attrEmployeeNumber   = "employeeNumber"
	attrDepartmentNumber = "departmentNumber"
	attrEmployeeType     = "employeeType"
	attrBusinessCategory = "businessCategory"
	attrUserPassword     = "userPassword"
	attrTelephoneNumber  = "telephoneNumber"
	attrPostalAddress    = "postalAddress"
	attrDNQualifier      = "dnQualifier"
	attrTitle            = "title"
	attrDescription      = "description"
	attrSeeAlso          = "seeAlso"
	objectClassPerson    = "inetOrgPerson"
	objectClassDept      = "organizationalUnit"
	objectClassPosition  = "organizationalRole"
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
		Addr:         strings.TrimSpace(config.GetConfigString(ldapAddrKey)),
		BindDN:       strings.TrimSpace(config.GetConfigString(ldapBindDNKey)),
		BindPassword: config.GetConfigString(ldapBindPasswordKey),
		BaseDN:       strings.TrimSpace(config.GetConfigString(ldapBaseDNKey)),
		PeopleOU:     strings.TrimSpace(config.GetConfigString(ldapPeopleOUKey)),
		DeptOU:       strings.TrimSpace(config.GetConfigString(ldapDepartmentOUKey)),
		PositionOU:   strings.TrimSpace(config.GetConfigString(ldapPositionOUKey)),
		UseTLS:       config.GetConfigBool(ldapUseTLSKey),
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
		return nil, err
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

func (o *OpenLDAP) userDN(uid string) string {
	return fmt.Sprintf("uid=%s,%s", uid, o.cfg.peopleDN())
}

func (o *OpenLDAP) departmentDN(code string) string {
	return fmt.Sprintf("%s=%s,%s", attrOU, code, o.cfg.deptDN())
}

func (o *OpenLDAP) positionDN(code string) string {
	return fmt.Sprintf("%s=%s,%s", attrCN, code, o.cfg.positionDN())
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

func (o *OpenLDAP) AddDepartment(ctx context.Context, d *Department) error {
	nd, err := normalizeDepartment(d)
	if err != nil {
		return err
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
		out = make([]*Department, 0, len(res.Entries))
		for _, entry := range res.Entries {
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
