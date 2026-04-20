package directorysync

import "context"

// Source 由业务系统实现，用于把业务模型转换为通用同步记录。
type Source interface {
	ListDepartments(ctx context.Context) ([]DepartmentRecord, error)
	ListPositions(ctx context.Context) ([]PositionRecord, error)
	ListUsers(ctx context.Context) ([]UserRecord, error)
}

// GroupSource 是可选扩展接口，用于提供 Flowable 等系统需要的 LDAP 组投影。
type GroupSource interface {
	ListGroups(ctx context.Context) ([]GroupRecord, error)
}
