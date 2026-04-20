package directorysync

import (
	"context"
	"fmt"
)

// Engine 负责把业务侧 Source 和目录侧 Sink 组合成一次完整同步。
type Engine struct {
	source Source
	sink   Sink
	logger Logger
}

// NewEngine 创建一个新的通用目录同步执行器。
func NewEngine(source Source, sink Sink, logger Logger) *Engine {
	if logger == nil {
		logger = noopLogger{}
	}
	return &Engine{source: source, sink: sink, logger: logger}
}

// RunFullSync 执行一次完整的部门、岗位、用户同步。
func (engine *Engine) RunFullSync(ctx context.Context, options SyncOptions) (*SyncReport, error) {
	report := newReport(options)
	if engine == nil || engine.source == nil || engine.sink == nil {
		report.addDetail(SyncDetail{Action: "bootstrap", Status: DetailStatusFailed, Message: "directorysync engine is not initialized"})
		report.finish("failed")
		return report, fmt.Errorf("directorysync engine is not initialized")
	}
	departmentRecords, err := engine.source.ListDepartments(ctx)
	if err != nil {
		report.addDetail(SyncDetail{RecordType: RecordTypeDepartment, Action: "load", Status: DetailStatusFailed, Message: err.Error()})
		report.finish("failed")
		return report, err
	}
	positionRecords, err := engine.source.ListPositions(ctx)
	if err != nil {
		report.addDetail(SyncDetail{RecordType: RecordTypePosition, Action: "load", Status: DetailStatusFailed, Message: err.Error()})
		report.finish("failed")
		return report, err
	}
	userRecords, err := engine.source.ListUsers(ctx)
	if err != nil {
		report.addDetail(SyncDetail{RecordType: RecordTypeUser, Action: "load", Status: DetailStatusFailed, Message: err.Error()})
		report.finish("failed")
		return report, err
	}
	groupRecords := make([]GroupRecord, 0)
	if groupSource, ok := engine.source.(GroupSource); ok {
		groupRecords, err = groupSource.ListGroups(ctx)
		if err != nil {
			report.addDetail(SyncDetail{RecordType: RecordTypeGroup, Action: "load", Status: DetailStatusFailed, Message: err.Error()})
			report.finish("failed")
			return report, err
		}
	}
	orderedDepartments, validationDetails, err := validateAndOrderDepartments(departmentRecords)
	for _, detail := range validationDetails {
		report.addDetail(detail)
	}
	if err != nil {
		report.addDetail(SyncDetail{RecordType: RecordTypeDepartment, Action: "validate", Status: DetailStatusFailed, Message: err.Error()})
		report.finish("failed")
		return report, err
	}
	orderedPositions, validationDetails, err := validatePositions(positionRecords, orderedDepartments)
	for _, detail := range validationDetails {
		report.addDetail(detail)
	}
	if err != nil {
		report.addDetail(SyncDetail{RecordType: RecordTypePosition, Action: "validate", Status: DetailStatusFailed, Message: err.Error()})
		report.finish("failed")
		return report, err
	}
	orderedUsers, validationDetails, err := validateUsers(userRecords, orderedDepartments, orderedPositions)
	for _, detail := range validationDetails {
		report.addDetail(detail)
	}
	if err != nil {
		report.addDetail(SyncDetail{RecordType: RecordTypeUser, Action: "validate", Status: DetailStatusFailed, Message: err.Error()})
		report.finish("failed")
		return report, err
	}
	orderedGroups := make([]GroupRecord, 0)
	if len(groupRecords) > 0 {
		orderedGroups, validationDetails, err = validateGroups(groupRecords, orderedUsers)
		for _, detail := range validationDetails {
			report.addDetail(detail)
		}
		if err != nil {
			report.addDetail(SyncDetail{RecordType: RecordTypeGroup, Action: "validate", Status: DetailStatusFailed, Message: err.Error()})
			report.finish("failed")
			return report, err
		}
	}
	report.DepartmentTotal = len(orderedDepartments)
	report.PositionTotal = len(orderedPositions)
	report.UserTotal = len(orderedUsers)
	report.GroupTotal = len(orderedGroups)
	if options.DryRun {
		report.finish("dry_run")
		return report, nil
	}
	for _, departmentRecord := range orderedDepartments {
		engine.syncDepartment(ctx, report, departmentRecord)
	}
	for _, positionRecord := range orderedPositions {
		engine.syncPosition(ctx, report, positionRecord)
	}
	for _, userRecord := range orderedUsers {
		engine.syncUser(ctx, report, userRecord, options)
	}
	if len(orderedGroups) > 0 {
		if groupSink, ok := engine.sink.(GroupSink); ok {
			for _, groupRecord := range orderedGroups {
				engine.syncGroup(ctx, report, groupSink, groupRecord)
			}
		} else {
			report.addDetail(SyncDetail{
				RecordType: RecordTypeGroup,
				Action:     "sync",
				Status:     DetailStatusWarning,
				Message:    "group projection is available from source but current sink does not support groups",
			})
		}
	}
	for _, departmentRecord := range orderedDepartments {
		engine.refreshDepartmentRelations(ctx, report, departmentRecord)
	}
	status := "success"
	if report.FailedCount > 0 {
		status = "partial_failed"
	}
	report.finish(status)
	return report, nil
}

func (engine *Engine) syncGroup(ctx context.Context, report *SyncReport, sink GroupSink, record GroupRecord) {
	if !record.Enabled {
		report.addDetail(SyncDetail{RecordType: RecordTypeGroup, RecordKey: record.Code, RecordName: record.Name, Action: "upsert", Status: DetailStatusSkipped, Message: "group disabled, skipped"})
		return
	}
	if err := sink.UpsertGroup(ctx, record); err != nil {
		engine.logger.Errorf("directorysync group upsert failed, code=%s, error=%v", record.Code, err)
		report.addDetail(SyncDetail{RecordType: RecordTypeGroup, RecordKey: record.Code, RecordName: record.Name, Action: "upsert", Status: DetailStatusFailed, Message: err.Error()})
		return
	}
	report.addDetail(SyncDetail{RecordType: RecordTypeGroup, RecordKey: record.Code, RecordName: record.Name, Action: "upsert", Status: DetailStatusSuccess, Message: "group synchronized"})
}

func (engine *Engine) syncDepartment(ctx context.Context, report *SyncReport, record DepartmentRecord) {
	if !record.Enabled {
		report.addDetail(SyncDetail{RecordType: RecordTypeDepartment, RecordKey: record.Code, RecordName: record.Name, Action: "upsert", Status: DetailStatusSkipped, Message: "department disabled, skipped"})
		return
	}
	bootstrapRecord := sanitizeDepartmentForBootstrap(record)
	if err := engine.sink.UpsertDepartment(ctx, bootstrapRecord); err != nil {
		engine.logger.Errorf("directorysync department upsert failed, code=%s, error=%v", record.Code, err)
		report.addDetail(SyncDetail{RecordType: RecordTypeDepartment, RecordKey: record.Code, RecordName: record.Name, Action: "upsert", Status: DetailStatusFailed, Message: err.Error()})
		return
	}
	report.addDetail(SyncDetail{RecordType: RecordTypeDepartment, RecordKey: record.Code, RecordName: record.Name, Action: "upsert", Status: DetailStatusSuccess, Message: "department synchronized"})
}

func (engine *Engine) refreshDepartmentRelations(ctx context.Context, report *SyncReport, record DepartmentRecord) {
	if !record.Enabled || !departmentHasDeferredRelations(record) {
		return
	}
	if err := engine.sink.UpsertDepartment(ctx, record); err != nil {
		engine.logger.Errorf("directorysync department relation refresh failed, code=%s, error=%v", record.Code, err)
		report.addDetail(SyncDetail{RecordType: RecordTypeDepartment, RecordKey: record.Code, RecordName: record.Name, Action: "refresh_relations", Status: DetailStatusFailed, Message: err.Error()})
	}
}

func sanitizeDepartmentForBootstrap(record DepartmentRecord) DepartmentRecord {
	if len(record.Attributes) == 0 {
		return record
	}
	cloned := record
	cloned.Attributes = cloneStringMap(record.Attributes)
	delete(cloned.Attributes, "manager_user_id")
	delete(cloned.Attributes, "manager_uid")
	if len(cloned.Attributes) == 0 {
		cloned.Attributes = nil
	}
	return cloned
}

func departmentHasDeferredRelations(record DepartmentRecord) bool {
	if len(record.Attributes) == 0 {
		return false
	}
	return record.Attributes["manager_user_id"] != "" || record.Attributes["manager_uid"] != ""
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (engine *Engine) syncPosition(ctx context.Context, report *SyncReport, record PositionRecord) {
	if !record.Enabled {
		report.addDetail(SyncDetail{RecordType: RecordTypePosition, RecordKey: record.Code, RecordName: record.Name, Action: "upsert", Status: DetailStatusSkipped, Message: "position disabled, skipped"})
		return
	}
	if err := engine.sink.UpsertPosition(ctx, record); err != nil {
		engine.logger.Errorf("directorysync position upsert failed, code=%s, error=%v", record.Code, err)
		report.addDetail(SyncDetail{RecordType: RecordTypePosition, RecordKey: record.Code, RecordName: record.Name, Action: "upsert", Status: DetailStatusFailed, Message: err.Error()})
		return
	}
	report.addDetail(SyncDetail{RecordType: RecordTypePosition, RecordKey: record.Code, RecordName: record.Name, Action: "upsert", Status: DetailStatusSuccess, Message: "position synchronized"})
}

func (engine *Engine) syncUser(ctx context.Context, report *SyncReport, record UserRecord, options SyncOptions) {
	if !record.Enabled {
		if !options.IncludeDisabled {
			report.addDetail(SyncDetail{RecordType: RecordTypeUser, RecordKey: record.UserID, RecordName: record.DisplayName, Action: "disable", Status: DetailStatusSkipped, Message: "disabled user skipped"})
			return
		}
		if err := engine.sink.DisableUser(ctx, record.UserID); err != nil {
			engine.logger.Errorf("directorysync disable user failed, user_id=%s, error=%v", record.UserID, err)
			report.addDetail(SyncDetail{RecordType: RecordTypeUser, RecordKey: record.UserID, RecordName: record.DisplayName, Action: "disable", Status: DetailStatusFailed, Message: err.Error()})
			return
		}
		report.addDetail(SyncDetail{RecordType: RecordTypeUser, RecordKey: record.UserID, RecordName: record.DisplayName, Action: "disable", Status: DetailStatusSuccess, Message: "user disabled in directory"})
		return
	}
	if err := engine.sink.UpsertUser(ctx, record); err != nil {
		engine.logger.Errorf("directorysync user upsert failed, user_id=%s, error=%v", record.UserID, err)
		report.addDetail(SyncDetail{RecordType: RecordTypeUser, RecordKey: record.UserID, RecordName: record.DisplayName, Action: "upsert", Status: DetailStatusFailed, Message: err.Error()})
		return
	}
	report.addDetail(SyncDetail{RecordType: RecordTypeUser, RecordKey: record.UserID, RecordName: record.DisplayName, Action: "upsert", Status: DetailStatusSuccess, Message: "user synchronized"})
	if err := engine.sink.BindUserDepartments(ctx, record.UserID, record.DepartmentCodes); err != nil {
		engine.logger.Errorf("directorysync bind departments failed, user_id=%s, error=%v", record.UserID, err)
		report.addDetail(SyncDetail{RecordType: RecordTypeUser, RecordKey: record.UserID, RecordName: record.DisplayName, Action: "bind_departments", Status: DetailStatusFailed, Message: err.Error()})
	} else {
		report.addDetail(SyncDetail{RecordType: RecordTypeUser, RecordKey: record.UserID, RecordName: record.DisplayName, Action: "bind_departments", Status: DetailStatusSuccess, Message: "user departments synchronized"})
	}
	if err := engine.sink.BindUserPositions(ctx, record.UserID, record.PositionCodes); err != nil {
		engine.logger.Errorf("directorysync bind positions failed, user_id=%s, error=%v", record.UserID, err)
		report.addDetail(SyncDetail{RecordType: RecordTypeUser, RecordKey: record.UserID, RecordName: record.DisplayName, Action: "bind_positions", Status: DetailStatusFailed, Message: err.Error()})
	} else {
		report.addDetail(SyncDetail{RecordType: RecordTypeUser, RecordKey: record.UserID, RecordName: record.DisplayName, Action: "bind_positions", Status: DetailStatusSuccess, Message: "user positions synchronized"})
	}
	if options.ResetPassword && record.InitialPassword != "" {
		if err := engine.sink.UpdatePassword(ctx, record.UserID, record.InitialPassword); err != nil {
			engine.logger.Errorf("directorysync update password failed, user_id=%s, error=%v", record.UserID, err)
			report.addDetail(SyncDetail{RecordType: RecordTypeUser, RecordKey: record.UserID, RecordName: record.DisplayName, Action: "update_password", Status: DetailStatusFailed, Message: err.Error()})
			return
		}
		report.addDetail(SyncDetail{RecordType: RecordTypeUser, RecordKey: record.UserID, RecordName: record.DisplayName, Action: "update_password", Status: DetailStatusSuccess, Message: "directory password initialized"})
	}
}
