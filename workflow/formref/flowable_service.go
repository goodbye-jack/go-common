package formref

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/goodbye-jack/go-common/config"
	"github.com/goodbye-jack/go-common/orm"
	"github.com/goodbye-jack/go-common/workflow/engine/flowable"
	"github.com/goodbye-jack/go-common/workflow/types"
)

const configFormRefDBInstance = "workflow.formref.db_instance"

type FlowableService struct {
	client *flowable.RESTClient
}

func NewFlowableService(client *flowable.RESTClient) *FlowableService {
	return &FlowableService{client: client}
}

func NewFlowableServiceFromConfig() (*FlowableService, error) {
	client, err := flowable.NewRESTClientFromConfig()
	if err != nil {
		return nil, err
	}
	return NewFlowableService(client), nil
}

func (s *FlowableService) Resolve(ctx context.Context, locator *TaskFormLocator) (*types.TaskFormReference, error) {
	if s == nil || s.client == nil || locator == nil || strings.TrimSpace(locator.TaskID) == "" {
		return notConfigured(), nil
	}

	formData, err := s.client.GetTaskFormData(ctx, locator.TaskID)
	if err != nil {
		return notConfigured(), nil
	}

	formKey := firstNonBlank(stringValue(formData["formKey"]), locator.FormKey)
	deploymentID := stringValue(formData["deploymentId"])
	formProperties := parseFormProperties(formData["formProperties"])

	if formKey == "" && len(formProperties) == 0 {
		return notConfigured(), nil
	}

	if deploymentID != "" && formKey != "" {
		resource, err := s.findFormResource(ctx, deploymentID, formKey)
		if err == nil && resource != nil {
			modelBytes, err := s.client.GetDeploymentResourceData(ctx, deploymentID, resource.ID)
			if err == nil {
				model := map[string]interface{}{}
				if json.Unmarshal(modelBytes, &model) == nil {
					return buildFormModelReference(formKey, deploymentID, resource.Name, model), nil
				}
			}
		}
	}

	if formKey != "" {
		record := s.findFormDefinition(formKey, locator.TenantID)
		if record != nil {
			model := map[string]interface{}{}
			if json.Unmarshal(record.ResourceBytes, &model) == nil {
				return buildFormModelReference(formKey, record.DeploymentID, record.ResourceName, model), nil
			}
		}
	}

	if len(formProperties) > 0 {
		return &types.TaskFormReference{
			Configured:   true,
			Resolved:     true,
			Source:       "form-properties",
			FormKey:      formKey,
			FormName:     firstNonBlank(formKey, "task-form"),
			DeploymentID: deploymentID,
			Fields:       formProperties,
			Outcomes:     []types.TaskFormOutcomeReference{},
		}, nil
	}

	return &types.TaskFormReference{
		Configured:   true,
		Resolved:     false,
		Source:       "form-key",
		FormKey:      formKey,
		FormName:     formKey,
		DeploymentID: deploymentID,
		Fields:       []types.TaskFormFieldReference{},
		Outcomes:     []types.TaskFormOutcomeReference{},
	}, nil
}

func (s *FlowableService) findFormResource(ctx context.Context, deploymentID, formKey string) (*flowable.DeploymentResource, error) {
	resources, err := s.client.ListDeploymentResources(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	exactName := normalizeFormResourceName(formKey)
	var fallback *flowable.DeploymentResource
	for _, item := range resources {
		name := strings.TrimSpace(item.Name)
		if name == "" || strings.TrimSpace(item.ID) == "" {
			continue
		}
		if name == exactName || strings.HasSuffix(name, "/"+exactName) {
			current := item
			return &current, nil
		}
		if fallback == nil && looksLikeFormResource(name, formKey) {
			current := item
			fallback = &current
		}
	}
	return fallback, nil
}

func (s *FlowableService) findFormDefinition(formKey, tenantID string) *formDefinitionRecord {
	db := formMetaDB()
	if db == nil || db.GetDB() == nil || strings.TrimSpace(formKey) == "" {
		return nil
	}
	records := queryFormDefinitions(db, formKey, tenantID)
	if len(records) == 0 && strings.TrimSpace(tenantID) != "" {
		records = queryFormDefinitions(db, formKey, "")
	}
	if len(records) == 0 {
		return nil
	}
	return &records[0]
}

func queryFormDefinitions(db *orm.Orm, formKey, tenantID string) []formDefinitionRecord {
	if db == nil || db.GetDB() == nil {
		return nil
	}
	sql := `
SELECT d.DEPLOYMENT_ID_ AS deployment_id,
       d.RESOURCE_NAME_ AS resource_name,
       r.RESOURCE_BYTES_ AS resource_bytes
  FROM ACT_FO_FORM_DEFINITION d
  JOIN ACT_FO_FORM_RESOURCE r
    ON r.DEPLOYMENT_ID_ = d.DEPLOYMENT_ID_
   AND r.NAME_ = d.RESOURCE_NAME_
 WHERE d.KEY_ = ?
`
	args := []interface{}{strings.TrimSpace(formKey)}
	if strings.TrimSpace(tenantID) == "" {
		sql += " AND (d.TENANT_ID_ IS NULL OR d.TENANT_ID_ = '')"
	} else {
		sql += " AND d.TENANT_ID_ = ?"
		args = append(args, strings.TrimSpace(tenantID))
	}
	sql += " ORDER BY d.VERSION_ DESC"
	rows := make([]formDefinitionRecord, 0)
	if err := db.GetDB().Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil
	}
	return rows
}

func formMetaDB() *orm.Orm {
	instanceName := strings.TrimSpace(config.GetConfigString(configFormRefDBInstance))
	if instanceName != "" {
		if db := orm.GetDB(instanceName); db != nil {
			return db
		}
		return nil
	}
	return orm.DB
}

func buildFormModelReference(formKey, deploymentID, resourceName string, formModel map[string]interface{}) *types.TaskFormReference {
	fields := make([]types.TaskFormFieldReference, 0)
	seen := map[string]bool{}
	collectFieldReferences(formModel["fields"], &fields, seen)
	outcomes := parseOutcomes(formModel["outcomes"])
	formName := firstNonBlank(stringValue(formModel["name"]), formKey)
	return &types.TaskFormReference{
		Configured:   true,
		Resolved:     true,
		Source:       "flowable-form-model",
		FormKey:      formKey,
		FormName:     formName,
		DeploymentID: deploymentID,
		ResourceName: resourceName,
		Fields:       fields,
		Outcomes:     outcomes,
	}
}

func collectFieldReferences(node interface{}, fields *[]types.TaskFormFieldReference, seen map[string]bool) {
	switch current := node.(type) {
	case []interface{}:
		for _, item := range current {
			collectFieldReferences(item, fields, seen)
		}
	case map[string]interface{}:
		typeName := stringValue(current["type"])
		id := stringValue(current["id"])
		name := firstNonBlank(stringValue(current["name"]), id)
		if typeName != "" && name != "" && looksLikeInputField(typeName) {
			dedupKey := firstNonBlank(id, typeName+":"+name)
			if !seen[dedupKey] {
				seen[dedupKey] = true
				*fields = append(*fields, types.TaskFormFieldReference{
					ID:       id,
					Name:     name,
					Type:     typeName,
					Required: booleanValue(current["required"]),
				})
			}
		}
		for _, value := range current {
			collectFieldReferences(value, fields, seen)
		}
	}
}

func parseOutcomes(node interface{}) []types.TaskFormOutcomeReference {
	rows, ok := node.([]interface{})
	if !ok {
		return []types.TaskFormOutcomeReference{}
	}
	result := make([]types.TaskFormOutcomeReference, 0, len(rows))
	for _, row := range rows {
		item, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		id := stringValue(item["id"])
		name := firstNonBlank(stringValue(item["name"]), id)
		if id == "" && name == "" {
			continue
		}
		result = append(result, types.TaskFormOutcomeReference{
			ID:   id,
			Name: name,
		})
	}
	return result
}

func parseFormProperties(node interface{}) []types.TaskFormFieldReference {
	rows, ok := node.([]interface{})
	if !ok {
		return []types.TaskFormFieldReference{}
	}
	result := make([]types.TaskFormFieldReference, 0, len(rows))
	for _, row := range rows {
		item, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		id := stringValue(item["id"])
		name := firstNonBlank(stringValue(item["name"]), id)
		typeName := firstNonBlank(stringValue(item["type"]), "string")
		if id == "" && name == "" {
			continue
		}
		result = append(result, types.TaskFormFieldReference{
			ID:       id,
			Name:     name,
			Type:     typeName,
			Required: booleanValue(item["required"]),
		})
	}
	return result
}

func normalizeFormResourceName(formKey string) string {
	key := strings.TrimSpace(formKey)
	if key == "" {
		return ""
	}
	if strings.HasSuffix(key, ".form") {
		return key
	}
	return key + ".form"
}

func looksLikeFormResource(resourceName, formKey string) bool {
	resource := strings.ToLower(strings.TrimSpace(resourceName))
	key := strings.ToLower(strings.TrimSpace(formKey))
	return resource != "" && key != "" && strings.HasSuffix(resource, ".form") && strings.Contains(resource, key)
}

func looksLikeInputField(typeName string) bool {
	switch strings.ToLower(strings.TrimSpace(typeName)) {
	case "container", "fieldset", "tab", "tabs", "panel", "group", "columns", "column":
		return false
	default:
		return strings.TrimSpace(typeName) != ""
	}
}

func booleanValue(value interface{}) bool {
	switch current := value.(type) {
	case bool:
		return current
	case string:
		return strings.EqualFold(strings.TrimSpace(current), "true")
	default:
		return false
	}
}

func stringValue(value interface{}) string {
	switch current := value.(type) {
	case string:
		return strings.TrimSpace(current)
	default:
		if current == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprintf("%v", current))
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func notConfigured() *types.TaskFormReference {
	return &types.TaskFormReference{
		Configured: false,
		Resolved:   false,
		Fields:     []types.TaskFormFieldReference{},
		Outcomes:   []types.TaskFormOutcomeReference{},
	}
}

type formDefinitionRecord struct {
	DeploymentID  string `gorm:"column:deployment_id"`
	ResourceName  string `gorm:"column:resource_name"`
	ResourceBytes []byte `gorm:"column:resource_bytes"`
}
