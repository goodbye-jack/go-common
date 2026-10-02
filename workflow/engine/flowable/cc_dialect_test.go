package flowable

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/goodbye-jack/go-common/orm"
	"github.com/goodbye-jack/go-common/utils"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCCDialectIdentifiersAndScopeCondition(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	column := ccColumn(database, "workflow_cc_records", "task_id")
	if !strings.Contains(column, "task_id") || !strings.Contains(column, "workflow_cc_records") {
		t.Fatalf("quoted task column=%q, want table and column", column)
	}
	condition := ccScopeCondition(database, "workflow_cc_records", "tenant_id")
	if !strings.Contains(condition, "IS NULL") || !strings.Contains(condition, "= ''") {
		t.Fatalf("scope condition=%q, want NULL and empty-string compatibility", condition)
	}
}

func TestEnsureCCTablesIsSafeWhenTablesAlreadyExist(t *testing.T) {
	const dbInstance = "workflow_cc_dialect_existing_tables_test"
	originalDB := orm.DB
	originalInstance := orm.RelationalMap[dbInstance]
	originalCCDB := viper.Get(configCCDBInstance)
	t.Cleanup(func() {
		orm.DB = originalDB
		if originalInstance != nil {
			orm.RelationalMap[dbInstance] = originalInstance
		} else {
			delete(orm.RelationalMap, dbInstance)
		}
		viper.Set(configCCDBInstance, originalCCDB)
		ccMigrateMu.Lock()
		delete(ccMigrated, "default")
		ccMigrateMu.Unlock()
	})

	testDB := orm.NewOrm(filepath.Join(t.TempDir(), "workflow_cc_dialect_existing_tables_test.sqlite"), utils.DBTypeSQLite, 1)
	if testDB == nil || testDB.GetDB() == nil {
		t.Fatal("NewOrm() returned nil")
	}
	orm.DB = testDB
	viper.Set(configCCDBInstance, "")
	ccMigrateMu.Lock()
	delete(ccMigrated, "default")
	ccMigrateMu.Unlock()

	if err := ensureCCTables(); err != nil {
		t.Fatalf("initial ensureCCTables() error=%v", err)
	}
	ccMigrateMu.Lock()
	delete(ccMigrated, "default")
	ccMigrateMu.Unlock()
	if err := ensureCCTables(); err != nil {
		t.Fatalf("existing-table ensureCCTables() error=%v", err)
	}
}
