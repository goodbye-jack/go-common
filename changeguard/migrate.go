package changeguard

import "github.com/goodbye-jack/go-common/orm"

// autoMigrateChangeguardTables 统一初始化 changeguard 依赖的持久化表。
// 这里集中处理，避免 sink/store/worker 各自懒建表造成时序问题和重复开销。
func autoMigrateChangeguardTables() error {
	if orm.DB == nil {
		return nil
	}
	db := orm.DB.GetDB()
	if db == nil {
		return nil
	}
	if err := db.AutoMigrate(&EventRecord{}); err != nil {
		return err
	}
	if err := db.AutoMigrate(&VersionRecord{}); err != nil {
		return err
	}
	if err := db.AutoMigrate(&DriftReportRecord{}); err != nil {
		return err
	}
	if err := db.AutoMigrate(&SecondFactorChallengeRecord{}); err != nil {
		return err
	}
	return nil
}
