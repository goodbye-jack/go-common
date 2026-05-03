package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
)

const requestJSONCacheContextKey = "__request_json_cache"

type GuardCheckFunc func(*gin.Context, *Principal) error

type namedGuard struct {
	name string
	fn   GuardCheckFunc
}

func (g namedGuard) Name() string {
	return g.name
}

func (g namedGuard) Check(c *gin.Context, principal *Principal) error {
	if g.fn == nil {
		return nil
	}
	return g.fn(c, principal)
}

func NewGuard(name string, fn GuardCheckFunc) Guard {
	return namedGuard{
		name: strings.TrimSpace(name),
		fn:   fn,
	}
}

func ForbidRequestFields(keys ...string) Guard {
	normalizedKeys := normalizeGuardKeys(keys...)
	return NewGuard("forbid_request_fields", func(c *gin.Context, _ *Principal) error {
		for _, key := range normalizedKeys {
			if HasRequestValue(c, key) {
				return fmt.Errorf("request field %s is not allowed", key)
			}
		}
		return nil
	})
}

func HasRequestValue(c *gin.Context, keys ...string) bool {
	return strings.TrimSpace(ResolveRequestValue(c, keys...)) != ""
}

func ResolveRequestValue(c *gin.Context, keys ...string) string {
	if c == nil {
		return ""
	}
	normalizedKeys := normalizeGuardKeys(keys...)
	for _, key := range normalizedKeys {
		if value := strings.TrimSpace(c.Param(key)); value != "" {
			return value
		}
		if value := strings.TrimSpace(c.Query(key)); value != "" {
			return value
		}
		if value, ok := c.GetPostForm(key); ok {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}

	jsonValues := getRequestJSONValues(c)
	for _, key := range normalizedKeys {
		if value := stringifyRequestValue(jsonValues[key]); value != "" {
			return value
		}
	}
	return ""
}

func normalizeGuardKeys(keys ...string) []string {
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		result = append(result, key)
	}
	return result
}

func getRequestJSONValues(c *gin.Context) map[string]any {
	if c == nil {
		return map[string]any{}
	}
	if cached, ok := c.Get(requestJSONCacheContextKey); ok {
		if values, ok := cached.(map[string]any); ok && values != nil {
			return values
		}
	}

	values := map[string]any{}
	if c.Request == nil || c.Request.Body == nil {
		c.Set(requestJSONCacheContextKey, values)
		return values
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Request.Body = io.NopCloser(bytes.NewBuffer(nil))
		c.Set(requestJSONCacheContextKey, values)
		return values
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		c.Set(requestJSONCacheContextKey, values)
		return values
	}

	if err := json.Unmarshal(bodyBytes, &values); err != nil {
		values = map[string]any{}
	}
	c.Set(requestJSONCacheContextKey, values)
	return values
}

func stringifyRequestValue(value any) string {
	switch typedValue := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typedValue)
	case json.Number:
		return strings.TrimSpace(typedValue.String())
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%.0f", typedValue))
	case float32:
		return strings.TrimSpace(fmt.Sprintf("%.0f", typedValue))
	case int:
		return strings.TrimSpace(fmt.Sprintf("%d", typedValue))
	case int8:
		return strings.TrimSpace(fmt.Sprintf("%d", typedValue))
	case int16:
		return strings.TrimSpace(fmt.Sprintf("%d", typedValue))
	case int32:
		return strings.TrimSpace(fmt.Sprintf("%d", typedValue))
	case int64:
		return strings.TrimSpace(fmt.Sprintf("%d", typedValue))
	case uint:
		return strings.TrimSpace(fmt.Sprintf("%d", typedValue))
	case uint8:
		return strings.TrimSpace(fmt.Sprintf("%d", typedValue))
	case uint16:
		return strings.TrimSpace(fmt.Sprintf("%d", typedValue))
	case uint32:
		return strings.TrimSpace(fmt.Sprintf("%d", typedValue))
	case uint64:
		return strings.TrimSpace(fmt.Sprintf("%d", typedValue))
	case bool:
		if typedValue {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typedValue))
	}
}

var ErrGuardDenied = errors.New("guard denied")
