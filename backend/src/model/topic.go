package model

import (
	"time"

	"gorm.io/gorm"
)

// Topic 热点话题
type Topic struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	Name         string         `gorm:"size:256;not null" json:"name"`
	Keywords     string         `gorm:"type:json" json:"keywords"`
	HeatScore    float64        `gorm:"index" json:"heatScore"`
	ArticleCount int            `json:"articleCount"`
	Trend        string         `gorm:"size:16" json:"trend"` // rising | stable | falling
	StartAt      time.Time      `json:"startAt"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}
