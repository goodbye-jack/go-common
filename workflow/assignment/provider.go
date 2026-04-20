package assignment

import (
	"strings"

	"github.com/goodbye-jack/go-common/config"
)

const configAssignmentProvider = "workflow.assignment.provider"

func NewServiceFromConfig() (Service, string, error) {
	provider := strings.ToLower(strings.TrimSpace(config.GetConfigString(configAssignmentProvider)))
	switch provider {
	case "", "none":
		return NewNoopService(), "none", nil
	case "directory":
		service, directoryProvider, err := NewDirectoryBackedServiceFromConfig()
		return service, "directory:" + directoryProvider, err
	case "ldap":
		service, err := NewLDAPServiceFromConfig()
		return service, "ldap", err
	default:
		return nil, provider, ErrUnsupportedProvider
	}
}
