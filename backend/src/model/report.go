package model

import (
	"time"

	"gorm.io/gorm"
)

// Report 分析报告
type Report struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	Title     string         `gorm:"size:256;not null" json:"title"`
	Type      string         `gorm:"size:32" json:"type"`
	StartAt   time.Time      `json:"startAt"`
	EndAt     time.Time      `json:"endAt"`
	Content   string         `gorm:"type:longtext" json:"content"`
	CreatedBy uint           `json:"createdBy"`
	Creator   User           `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
