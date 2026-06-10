package assignment

import (
	"fmt"
	"strings"
	"sync"

	"github.com/goodbye-jack/go-common/config"
)

const configAssignmentProvider = "workflow.assignment.provider"

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
		return fmt.Errorf("workflow assignment factory name is required")
	}
	if factory == nil {
		return fmt.Errorf("workflow assignment factory %s is nil", normalized)
	}
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	if _, exists := factories[normalized]; exists {
		return fmt.Errorf("workflow assignment factory already registered: %s", normalized)
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
	provider := strings.ToLower(strings.TrimSpace(config.GetConfigString(configAssignmentProvider)))
	if provider == "" {
		provider = "none"
	}
	factoriesMu.RLock()
	factory, ok := factories[provider]
	factoriesMu.RUnlock()
	if !ok {
		return nil, provider, ErrUnsupportedProvider
	}
	return factory.BuildFromConfig()
}

func init() {
	MustRegisterFactory("none", FactoryFunc(func() (Service, string, error) {
		return NewNoopService(), "none", nil
	}))
	MustRegisterFactory("directory", FactoryFunc(func() (Service, string, error) {
		service, directoryProvider, err := NewDirectoryBackedServiceFromConfig()
		return service, "directory:" + directoryProvider, err
	}))
	MustRegisterFactory("ldap", FactoryFunc(func() (Service, string, error) {
		service, err := NewLDAPServiceFromConfig()
		return service, "ldap", err
	}))
}
