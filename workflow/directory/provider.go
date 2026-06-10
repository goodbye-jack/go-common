package directory

import (
	"fmt"
	"strings"
	"sync"

	"github.com/goodbye-jack/go-common/config"
	commonldap "github.com/goodbye-jack/go-common/ldap"
)

const configDirectoryProvider = "workflow.directory.provider"

type Factory interface {
	BuildFromConfig() (Service, string, error)
}

type FactoryFunc func() (Service, string, error)

func (f FactoryFunc) BuildFromConfig() (Service, string, error) {
	return f()
}

var (
	factoriesMu sync.RWMutex
	factories   = map[string]Factory{}
)

func RegisterFactory(name string, factory Factory) error {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return fmt.Errorf("workflow directory factory name is required")
	}
	if factory == nil {
		return fmt.Errorf("workflow directory factory %s is nil", normalized)
	}
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	if _, exists := factories[normalized]; exists {
		return fmt.Errorf("workflow directory factory already registered: %s", normalized)
	}
	factories[normalized] = factory
	return nil
}

func MustRegisterFactory(name string, factory Factory) {
	if err := RegisterFactory(name, factory); err != nil {
		panic(err)
	}
}

func RegisteredFactories() []string {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()
	result := make([]string, 0, len(factories))
	for name := range factories {
		result = append(result, name)
	}
	return result
}

func NewServiceFromConfig() (Service, string, error) {
	provider := resolveProviderFromConfig()
	factoriesMu.RLock()
	factory, ok := factories[provider]
	factoriesMu.RUnlock()
	if !ok {
		return nil, provider, commonldap.LdapParamsError{Params: []string{configDirectoryProvider}}
	}
	return factory.BuildFromConfig()
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

func init() {
	MustRegisterFactory("none", FactoryFunc(func() (Service, string, error) {
		return NewNoopService(), "none", nil
	}))
	MustRegisterFactory("ldap", FactoryFunc(func() (Service, string, error) {
		service, err := NewLDAPServiceFromConfig()
		return service, "ldap", err
	}))
}
