package ldap

import "context"

// User 保留旧版字段定义，兼容仍在使用 NewLLDap + *User 的业务代码。
type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

type LegacyLdap struct {
	Ldap
}

// NewLLDap 兼容旧版构造方式，当前从配置中心读取 LDAP 配置。
func NewLLDap(serviceName string, bindDN string, bindPassword string) *LegacyLdap {
	client, err := NewOpenLDAPFromConfig()
	if err != nil {
		return &LegacyLdap{}
	}
	return &LegacyLdap{Ldap: client}
}

func (l *LegacyLdap) AddUser(ctx context.Context, user *User) error {
	if l == nil || l.Ldap == nil || user == nil {
		return LdapIntervalError{}
	}
	return l.Ldap.AddUser(ctx, &OrgUser{
		UID:         user.ID,
		DisplayName: user.DisplayName,
		Email:       user.Email,
	})
}
