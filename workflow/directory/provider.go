package directory

import (
	"strings"

	"github.com/goodbye-jack/go-common/config"
	commonldap "github.com/goodbye-jack/go-common/ldap"
)

const configDirectoryProvider = "workflow.directory.provider"

func NewServiceFromConfig() (Service, string, error) {
	provider := resolveProviderFromConfig()
	switch provider {
	case "none":
		return NewNoopService(), provider, nil
	case "ldap":
		service, err := NewLDAPServiceFromConfig()
		return service, provider, err
	default:
		return nil, provider, commonldap.LdapParamsError{Params: []string{configDirectoryProvider}}
	}
}

func resolveProviderFromConfig() string {
	provider := strings.ToLower(strings.TrimSpace(config.GetConfigString(configDirectoryProvider)))
	if provider != "" {
		return provider
	}
	if hasLegacyOrNamespacedLDAPConfig() {
		return "ldap"
	}
	return "none"
}

func hasLegacyOrNamespacedLDAPConfig() bool {
	keys := []string{
		"workflow.directory.ldap.addr",
		"workflow.directory.ldap.bind_dn",
		"workflow.directory.ldap.bind_password",
		"workflow.directory.ldap.base_dn",
		"ldap_addr",
		"ldap_bind_dn",
		"ldap_bind_password",
		"ldap_base_dn",
	}
	for _, key := range keys {
		if strings.TrimSpace(config.GetConfigString(key)) != "" {
			return true
		}
	}
	return false
}
