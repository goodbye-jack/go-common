package http

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
)

// RequireCapability checks business capabilities after login has resolved a Principal.
// Business projects can fill Principal.CapabilityCodes directly, or put []string/string
// into Principal.Attributes["capabilities"] during their PrincipalResolver.
func RequireCapability(codes ...string) Guard {
	required := normalizeCapabilityCodes(codes...)
	return NewGuard("require_capability", func(_ *gin.Context, principal *Principal) error {
		if len(required) == 0 {
			return nil
		}
		if principal == nil || principal.Type == PrincipalAnonymous {
			return errors.New("missing authenticated principal")
		}
		actual := principalCapabilities(principal)
		for _, want := range required {
			if _, ok := actual[want]; ok {
				return nil
			}
		}
		return ErrGuardDenied
	})
}

func principalCapabilities(principal *Principal) map[string]struct{} {
	result := map[string]struct{}{}
	if principal == nil {
		return result
	}
	addCapabilities(result, principal.CapabilityCodes...)
	if principal.Attributes == nil {
		return result
	}
	if value, ok := principal.Attributes["capabilities"]; ok {
		addCapabilityAttribute(result, value)
	}
	if value, ok := principal.Attributes["capability_codes"]; ok {
		addCapabilityAttribute(result, value)
	}
	return result
}

func addCapabilityAttribute(result map[string]struct{}, value any) {
	switch typed := value.(type) {
	case string:
		addCapabilities(result, strings.Split(typed, ",")...)
	case []string:
		addCapabilities(result, typed...)
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				addCapabilities(result, text)
			}
		}
	}
}

func normalizeCapabilityCodes(codes ...string) []string {
	result := make([]string, 0, len(codes))
	seen := map[string]struct{}{}
	for _, code := range codes {
		normalized := strings.TrimSpace(code)
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func addCapabilities(result map[string]struct{}, codes ...string) {
	for _, code := range codes {
		normalized := strings.TrimSpace(code)
		if normalized == "" {
			continue
		}
		result[strings.ToLower(normalized)] = struct{}{}
	}
}
