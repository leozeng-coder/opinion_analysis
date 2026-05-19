package model

import "time"

// SystemSetting 系统键值配置
type SystemSetting struct {
	Key       string    `gorm:"primarykey;size:64" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	Desc      string    `gorm:"size:256" json:"desc"`
	UpdatedAt time.Time `json:"updatedAt"`
	UpdatedBy uint      `json:"updatedBy"`
}

func (SystemSetting) TableName() string { return "system_settings" }

// AuditLog 写操作审计日志
type AuditLog struct {
	ID         uint      `gorm:"primarykey" json:"id"`
	ActorID    uint      `gorm:"index" json:"actorId"`
	ActorName  string    `gorm:"size:64" json:"actorName"`
	Action     string    `gorm:"size:32;index" json:"action"`
	Resource   string    `gorm:"size:64;index" json:"resource"`
	ResourceID string    `gorm:"size:64" json:"resourceId"`
	Method     string    `gorm:"size:8" json:"method"`
	Path       string    `gorm:"size:256" json:"path"`
	Status     int       `json:"status"`
	Payload    string    `gorm:"type:text" json:"payload"`
	IP         string    `gorm:"size:64" json:"ip"`
	UserAgent  string    `gorm:"size:256" json:"userAgent"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`
}

func (AuditLog) TableName() string { return "audit_logs" }
