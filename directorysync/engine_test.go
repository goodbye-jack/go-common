package directorysync

import (
	"context"
	"errors"
	"testing"
)

type stubSource struct {
	departments []DepartmentRecord
	positions   []PositionRecord
	users       []UserRecord
	groups      []GroupRecord
}

func (source *stubSource) ListDepartments(context.Context) ([]DepartmentRecord, error) {
	return append([]DepartmentRecord(nil), source.departments...), nil
}

func (source *stubSource) ListPositions(context.Context) ([]PositionRecord, error) {
	return append([]PositionRecord(nil), source.positions...), nil
}

func (source *stubSource) ListUsers(context.Context) ([]UserRecord, error) {
	return append([]UserRecord(nil), source.users...), nil
}

func (source *stubSource) ListGroups(context.Context) ([]GroupRecord, error) {
	return append([]GroupRecord(nil), source.groups...), nil
}

type stubSink struct {
	calls []string
}

func (sink *stubSink) UpsertDepartment(_ context.Context, record DepartmentRecord) error {
	sink.calls = append(sink.calls, "department:"+record.Code)
	return nil
}
func (sink *stubSink) UpsertPosition(_ context.Context, record PositionRecord) error {
	sink.calls = append(sink.calls, "position:"+record.Code)
	return nil
}
func (sink *stubSink) UpsertUser(_ context.Context, record UserRecord) error {
	sink.calls = append(sink.calls, "user:"+record.UserID)
	return nil
}
func (sink *stubSink) BindUserDepartments(_ context.Context, userID string, _ []string) error {
	sink.calls = append(sink.calls, "bind_department:"+userID)
	return nil
}
func (sink *stubSink) BindUserPositions(_ context.Context, userID string, _ []string) error {
	sink.calls = append(sink.calls, "bind_position:"+userID)
	return nil
}
func (sink *stubSink) DisableUser(_ context.Context, userID string) error {
	sink.calls = append(sink.calls, "disable:"+userID)
	return nil
}
func (sink *stubSink) UpdatePassword(_ context.Context, userID, _ string) error {
	sink.calls = append(sink.calls, "password:"+userID)
	return nil
}

func (sink *stubSink) UpsertGroup(_ context.Context, record GroupRecord) error {
	sink.calls = append(sink.calls, "group:"+record.Code)
	return nil
}

func TestRunFullSyncDryRun(t *testing.T) {
	engine := NewEngine(&stubSource{
		departments: []DepartmentRecord{{Code: "province:nmg", Name: "内蒙古自治区", Enabled: true}},
		positions:   []PositionRecord{{Code: "RELIC_KEEPER", Name: "文保员", Enabled: true}},
		users: []UserRecord{{
			UserID:          "keeper01",
			DisplayName:     "文保员甲",
			DepartmentCodes: []string{"province:nmg"},
			PositionCodes:   []string{"RELIC_KEEPER"},
			Enabled:         true,
		}},
	}, &stubSink{}, nil)
	report, err := engine.RunFullSync(context.Background(), SyncOptions{DryRun: true, BatchNo: "batch-1"})
	if err != nil {
		t.Fatalf("RunFullSync dry run returned error: %v", err)
	}
	if report.Status != "dry_run" {
		t.Fatalf("expected dry_run status, got %s", report.Status)
	}
	if report.DepartmentTotal != 1 || report.PositionTotal != 1 || report.UserTotal != 1 {
		t.Fatalf("unexpected totals: %+v", report)
	}
}

func TestRunFullSyncValidationFailure(t *testing.T) {
	engine := NewEngine(&stubSource{users: []UserRecord{{UserID: "user01", DisplayName: "", Enabled: true}}}, &stubSink{}, nil)
	report, err := engine.RunFullSync(context.Background(), SyncOptions{BatchNo: "batch-2"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var validationErr ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if report.Status != "failed" {
		t.Fatalf("expected failed status, got %s", report.Status)
	}
}

func TestRunFullSyncWithGroupProjection(t *testing.T) {
	sink := &stubSink{}
	engine := NewEngine(&stubSource{
		departments: []DepartmentRecord{{Code: "dept:root", Name: "根部门", Enabled: true}},
		positions:   []PositionRecord{{Code: "RELIC_KEEPER", Name: "文保员", Enabled: true}},
		users: []UserRecord{{
			UserID:          "keeper01",
			DisplayName:     "文保员甲",
			DepartmentCodes: []string{"dept:root"},
			PositionCodes:   []string{"RELIC_KEEPER"},
			Enabled:         true,
		}},
		groups: []GroupRecord{
			{Code: "dept:dept:root", Name: "部门-根部门", MemberUserIDs: []string{"keeper01"}, Enabled: true},
			{Code: "pos:RELIC_KEEPER", Name: "岗位-文保员", MemberUserIDs: []string{"keeper01"}, Enabled: true},
		},
	}, sink, nil)
	report, err := engine.RunFullSync(context.Background(), SyncOptions{BatchNo: "batch-group"})
	if err != nil {
		t.Fatalf("RunFullSync group projection returned error: %v", err)
	}
	if report.Status != "success" {
		t.Fatalf("expected success status, got %s", report.Status)
	}
	if report.GroupTotal != 2 {
		t.Fatalf("expected 2 groups, got %d", report.GroupTotal)
	}
	if len(sink.calls) == 0 || sink.calls[len(sink.calls)-1] != "group:pos:RELIC_KEEPER" {
		t.Fatalf("expected group sync calls, got %+v", sink.calls)
	}
}
