package directory

import (
	"testing"

	"github.com/spf13/viper"
)

func TestResolveProviderFromTopLevelLDAPConfig(t *testing.T) {
	clearDirectoryProviderConfig(t)
	viper.Set("ldap.addr", "ldap://127.0.0.1:8389")
	viper.Set("ldap.bind_dn", "cn=admin,dc=example,dc=com")
	viper.Set("ldap.bind_password", "admin123")
	viper.Set("ldap.base_dn", "dc=example,dc=com")

	if got := resolveProviderFromConfig(); got != "ldap" {
		t.Fatalf("provider mismatch: %s", got)
	}
}

func TestResolveProviderIgnoresLegacyFlatLDAPConfig(t *testing.T) {
	clearDirectoryProviderConfig(t)
	viper.Set("ldap_addr", "ldap://127.0.0.1:8389")
	viper.Set("ldap_bind_dn", "cn=admin,dc=example,dc=com")
	viper.Set("ldap_bind_password", "admin123")
	viper.Set("ldap_base_dn", "dc=example,dc=com")

	if got := resolveProviderFromConfig(); got != "none" {
		t.Fatalf("legacy flat ldap_* keys should not enable provider, got=%s", got)
	}
}

func clearDirectoryProviderConfig(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		configDirectoryProvider,
		"ldap.addr",
		"ldap.bind_dn",
		"ldap.bind_password",
		"ldap.base_dn",
		"ldap_addr",
		"ldap_bind_dn",
		"ldap_bind_password",
		"ldap_base_dn",
	} {
		viper.Set(key, nil)
	}
}
