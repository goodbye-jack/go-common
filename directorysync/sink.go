package directorysync

import "context"

// Sink 由具体目录实现，用于消费通用同步记录。
type Sink interface {
	UpsertDepartment(ctx context.Context, record DepartmentRecord) error
	UpsertPosition(ctx context.Context, record PositionRecord) error
	UpsertUser(ctx context.Context, record UserRecord) error
	BindUserDepartments(ctx context.Context, userID string, departmentCodes []string) error
	BindUserPositions(ctx context.Context, userID string, positionCodes []string) error
	DisableUser(ctx context.Context, userID string) error
	UpdatePassword(ctx context.Context, userID, plainPassword string) error
}

// GroupSink 是可选扩展接口，用于消费 LDAP 组投影记录。
type GroupSink interface {
	UpsertGroup(ctx context.Context, record GroupRecord) error
}
