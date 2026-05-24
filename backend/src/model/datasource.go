package model

import (
	"time"

	"gorm.io/gorm"
)

// DataSource 舆情数据来源
type DataSource struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	Name      string         `gorm:"size:128;not null" json:"name"`
	Type      string         `gorm:"size:32;not null" json:"type"` // crawler | api | manual
	URL       string         `gorm:"size:512" json:"url"`
	Config    string         `gorm:"type:json" json:"config"`
	Status    int8           `gorm:"default:1" json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
