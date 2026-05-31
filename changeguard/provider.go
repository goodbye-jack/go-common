package changeguard

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goodbye-jack/go-common/orm"
)

type providerRegistry struct {
	singletonFetchers map[string]SingletonFetcher
	customProviders   map[string]CustomProvider
}

func newProviderRegistry() *providerRegistry {
	return &providerRegistry{
		singletonFetchers: map[string]SingletonFetcher{},
		customProviders:   map[string]CustomProvider{},
	}
}

func (r *providerRegistry) resolve(resource ResourceProfile) (Provider, error) {
	switch resource.ProviderType {
	case ProviderSingletonConfig:
		fetcher := r.singletonFetchers[resource.FetcherName]
		if fetcher == nil {
			return nil, fmt.Errorf("singleton fetcher not found: %s", resource.FetcherName)
		}
		return &singletonProvider{fetcher: fetcher}, nil
	case ProviderGormEntitySave, ProviderGormEntityToggle:
		return &gormEntityProvider{modelValue: resource.ModelValue}, nil
	case ProviderCustomFetcher:
		custom := r.customProviders[resource.CustomKey]
		if custom == nil {
			return nil, fmt.Errorf("custom provider not found: %s", resource.CustomKey)
		}
		return custom, nil
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", resource.ProviderType)
	}
}

type singletonProvider struct {
	fetcher SingletonFetcher
}

func (p *singletonProvider) Before(s *Session) (*ResourceState, error) {
	return p.load(s)
}

func (p *singletonProvider) After(s *Session) (*ResourceState, error) {
	return p.load(s)
}

func (p *singletonProvider) load(s *Session) (*ResourceState, error) {
	if p == nil || p.fetcher == nil || s == nil {
		return nil, nil
	}
	raw, err := p.fetcher(sessionContext(s.Context))
	if err != nil {
		return nil, err
	}
	value, err := NormalizeAny(raw, s.Policy)
	if err != nil {
		return nil, err
	}
	return &ResourceState{
		ResourceType: s.Resource.ResourceType,
		ResourceID:   "singleton",
		ResourceName: chooseNonEmpty(s.Resource.Name, s.Resource.Key),
		Value:        value,
		RawValue:     raw,
	}, nil
}

type gormEntityProvider struct {
	modelValue any
}

func (p *gormEntityProvider) Before(s *Session) (*ResourceState, error) {
	return p.load(s)
}

func (p *gormEntityProvider) After(s *Session) (*ResourceState, error) {
	return p.load(s)
}

func (p *gormEntityProvider) load(s *Session) (*ResourceState, error) {
	if orm.DB == nil || s == nil {
		return nil, nil
	}
	filters := ResolvePrimaryKey(s, s.Resource)
	if len(filters) == 0 {
		return nil, nil
	}
	modelPtr, err := newModelPointer(p.modelValue)
	if err != nil {
		return nil, err
	}
	db := orm.DB.GetDB().WithContext(sessionContext(s.Context)).Model(modelPtr)
	for key, value := range filters {
		db = db.Where(key+" = ?", value)
	}
	if err := db.First(modelPtr).Error; err != nil {
		return nil, nil
	}
	value, err := NormalizeAny(modelPtr, s.Policy)
	if err != nil {
		return nil, err
	}
	return &ResourceState{
		ResourceType: s.Resource.ResourceType,
		ResourceID:   primaryKeyString(filters),
		ResourceName: chooseNonEmpty(s.Resource.Name, s.Resource.Key),
		Value:        value,
		RawValue:     modelPtr,
	}, nil
}

func ResolvePrimaryKey(s *Session, resource ResourceProfile) map[string]any {
	if s == nil || s.Context == nil {
		return map[string]any{}
	}
	keys := append([]string{}, resource.RequestKeys...)
	if len(keys) == 0 {
		keys = []string{"id", "plan_code", "package_code", "code"}
	}
	body, _ := GetCachedJSONMap(s.Context)
	result := map[string]any{}
	for idx, key := range keys {
		if value, ok := findMapValueCaseInsensitive(body, key); ok && value != nil && fmt.Sprint(value) != "" {
			result[lookupKeyByIndex(resource.LookupKeys, idx, key)] = value
			return result
		}
		if value := s.Context.Query(key); strings.TrimSpace(value) != "" {
			result[lookupKeyByIndex(resource.LookupKeys, idx, key)] = value
			return result
		}
		if value := s.Context.Param(key); strings.TrimSpace(value) != "" {
			result[lookupKeyByIndex(resource.LookupKeys, idx, key)] = value
			return result
		}
	}
	return result
}

func lookupKeyByIndex(lookupKeys []string, idx int, fallback string) string {
	if idx >= 0 && idx < len(lookupKeys) && strings.TrimSpace(lookupKeys[idx]) != "" {
		return strings.TrimSpace(lookupKeys[idx])
	}
	return fallback
}

func findMapValueCaseInsensitive(values map[string]any, target string) (any, bool) {
	if len(values) == 0 {
		return nil, false
	}
	if value, ok := values[target]; ok {
		return value, true
	}
	for key, value := range values {
		if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(target)) {
			return value, true
		}
	}
	return nil, false
}

func primaryKeyString(filters map[string]any) string {
	for _, value := range filters {
		return fmt.Sprint(value)
	}
	return ""
}

func newModelPointer(modelValue any) (any, error) {
	if modelValue == nil {
		return nil, fmt.Errorf("modelValue is nil")
	}
	value := reflect.ValueOf(modelValue)
	if value.Kind() != reflect.Ptr {
		ptr := reflect.New(value.Type())
		ptr.Elem().Set(value)
		return ptr.Interface(), nil
	}
	return reflect.New(value.Elem().Type()).Interface(), nil
}

func clonePolicy(policy PolicyProfile) PolicyProfile {
	return policy
}

func sessionContext(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	return c.Request.Context()
}
