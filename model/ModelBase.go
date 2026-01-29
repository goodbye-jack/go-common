package model

import (
	"gorm.io/gorm"
	"time"
)

type ModelBase struct {
	ID        uint           `gorm:"primaryKey" json:"id"`             // 主键
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"` // 创建时间
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"` // 更新时间
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`                   // 软删除
} // 🔴 重点：这个标签标记该模型需要自动迁移
