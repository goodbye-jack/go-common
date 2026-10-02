package formref

import (
	"context"
	"testing"

	"github.com/goodbye-jack/go-common/orm"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/spf13/viper"
)

func TestResolveReturnsNotConfiguredWhenDBFallbackNotConfigured(t *testing.T) {
	originalDBInstance := viper.Get(configFormRefDBInstance)
	originalFallbackEnabled := viper.Get(configFormRefDBFallbackEnabled)
	t.Cleanup(func() {
		viper.Set(configFormRefDBInstance, originalDBInstance)
		viper.Set(configFormRefDBFallbackEnabled, originalFallbackEnabled)
	})
	viper.Set(configFormRefDBInstance, "")
	viper.Set(configFormRefDBFallbackEnabled, false)

	service := &FlowableService{}
	ref, err := service.Resolve(context.Background(), &TaskFormLocator{
		TaskID:  "task-001",
		FormKey: "preservation_alarm_form",
	})
	if err != nil {
		t.Fatalf("Resolve() error=%v, want nil", err)
	}
	if ref == nil {
		t.Fatalf("Resolve() returned nil reference")
	}
	if ref.Configured {
		t.Fatalf("ref.Configured=%v, want false", ref.Configured)
	}
	if ref.Resolved {
		t.Fatalf("ref.Resolved=%v, want false", ref.Resolved)
	}
}

func TestResolveDisablesDBFallbackExplicitly(t *testing.T) {
	originalDBInstance := viper.Get(configFormRefDBInstance)
	originalFallbackEnabled := viper.Get(configFormRefDBFallbackEnabled)
	originalORM := orm.RelationalMap["flowable_meta_disabled_test"]
	t.Cleanup(func() {
		viper.Set(configFormRefDBInstance, originalDBInstance)
		viper.Set(configFormRefDBFallbackEnabled, originalFallbackEnabled)
		if originalORM != nil {
			orm.RelationalMap["flowable_meta_disabled_test"] = originalORM
		} else {
			delete(orm.RelationalMap, "flowable_meta_disabled_test")
		}
	})

	viper.Set(configFormRefDBInstance, "flowable_meta_disabled_test")
	viper.Set(configFormRefDBFallbackEnabled, false)
	orm.RelationalMap["flowable_meta_disabled_test"] = orm.NewOrm("file::memory:?cache=shared", utils.DBTypeSQLite, 1)

	service := &FlowableService{}
	ref, err := service.Resolve(context.Background(), &TaskFormLocator{
		TaskID:  "task-disabled-fallback",
		FormKey: "preservation_alarm_form",
	})
	if err != nil {
		t.Fatalf("Resolve() error=%v, want nil", err)
	}
	if ref == nil || ref.Resolved {
		t.Fatalf("Resolve() ref=%+v, want unresolved REST-only result", ref)
	}
}

func TestResolveUsesConfiguredDBFallbackWhenClientUnavailable(t *testing.T) {
	db := orm.NewOrm("file::memory:?cache=shared", utils.DBTypeSQLite, 1)
	if db == nil || db.GetDB() == nil {
		t.Fatalf("failed to create sqlite orm")
	}
	if err := db.GetDB().Exec(`
CREATE TABLE ACT_FO_FORM_DEFINITION (
  KEY_ TEXT,
  TENANT_ID_ TEXT,
  VERSION_ INTEGER,
  DEPLOYMENT_ID_ TEXT,
  RESOURCE_NAME_ TEXT
);
`).Error; err != nil {
		t.Fatalf("create ACT_FO_FORM_DEFINITION failed: %v", err)
	}
	if err := db.GetDB().Exec(`
CREATE TABLE ACT_FO_FORM_RESOURCE (
  DEPLOYMENT_ID_ TEXT,
  NAME_ TEXT,
  RESOURCE_BYTES_ BLOB
);
`).Error; err != nil {
		t.Fatalf("create ACT_FO_FORM_RESOURCE failed: %v", err)
	}

	model := `{
  "name": "通用审批表单",
  "fields": [
    {"id": "title", "name": "标题", "type": "text", "required": true},
    {"id": "alarmLevel", "name": "预警级别", "type": "dropdown"}
  ],
  "outcomes": [
    {"id": "submit", "name": "提交"}
  ]
}`
	if err := db.GetDB().Exec(`
INSERT INTO ACT_FO_FORM_DEFINITION (KEY_, TENANT_ID_, VERSION_, DEPLOYMENT_ID_, RESOURCE_NAME_)
VALUES (?, ?, ?, ?, ?)
`, "preservation_alarm_form", "tenant-a", 3, "dep-001", "forms/preservation_alarm.form").Error; err != nil {
		t.Fatalf("insert ACT_FO_FORM_DEFINITION failed: %v", err)
	}
	if err := db.GetDB().Exec(`
INSERT INTO ACT_FO_FORM_RESOURCE (DEPLOYMENT_ID_, NAME_, RESOURCE_BYTES_)
VALUES (?, ?, ?)
`, "dep-001", "forms/preservation_alarm.form", []byte(model)).Error; err != nil {
		t.Fatalf("insert ACT_FO_FORM_RESOURCE failed: %v", err)
	}

	const instanceName = "flowable_meta_test"
	originalDBInstance := viper.Get(configFormRefDBInstance)
	originalORM := orm.RelationalMap[instanceName]
	t.Cleanup(func() {
		viper.Set(configFormRefDBInstance, originalDBInstance)
		if originalORM != nil {
			orm.RelationalMap[instanceName] = originalORM
		} else {
			delete(orm.RelationalMap, instanceName)
		}
	})
	viper.Set(configFormRefDBInstance, instanceName)
	orm.RelationalMap[instanceName] = db

	service := &FlowableService{}
	ref, err := service.Resolve(context.Background(), &TaskFormLocator{
		TaskID:   "task-001",
		FormKey:  "preservation_alarm_form",
		TenantID: "tenant-a",
	})
	if err != nil {
		t.Fatalf("Resolve() error=%v, want nil", err)
	}
	if ref == nil {
		t.Fatalf("Resolve() returned nil reference")
	}
	if !ref.Configured || !ref.Resolved {
		t.Fatalf("ref configured/resolved=%v/%v, want true/true", ref.Configured, ref.Resolved)
	}
	if ref.Source != "flowable-form-model" {
		t.Fatalf("ref.Source=%q, want flowable-form-model", ref.Source)
	}
	if ref.FormKey != "preservation_alarm_form" {
		t.Fatalf("ref.FormKey=%q, want preservation_alarm_form", ref.FormKey)
	}
	if ref.DeploymentID != "dep-001" {
		t.Fatalf("ref.DeploymentID=%q, want dep-001", ref.DeploymentID)
	}
	if len(ref.Fields) != 2 {
		t.Fatalf("len(ref.Fields)=%d, want 2", len(ref.Fields))
	}
	if len(ref.Outcomes) != 1 || ref.Outcomes[0].ID != "submit" {
		t.Fatalf("unexpected outcomes: %+v", ref.Outcomes)
	}
}
