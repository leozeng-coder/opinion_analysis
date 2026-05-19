package model

import (
	"time"

	"gorm.io/gorm"
)

// AlertRule 预警规则
type AlertRule struct {
	ID         uint           `gorm:"primarykey" json:"id"`
	Name       string         `gorm:"size:128;not null" json:"name"`
	Keywords   string         `gorm:"type:json" json:"keywords"`
	Sentiment  string         `gorm:"size:16" json:"sentiment"`
	Threshold  int            `json:"threshold"`
	Interval   int            `json:"interval"`
	NotifyType string         `gorm:"size:32" json:"notifyType"`
	NotifyConf string         `gorm:"type:json" json:"notifyConf"`
	Status     int8           `gorm:"default:1" json:"status"`
	CreatedBy  uint           `json:"createdBy"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

// AlertRecord 预警记录
type AlertRecord struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	RuleID    uint      `gorm:"index" json:"ruleId"`
	Rule      AlertRule `gorm:"foreignKey:RuleID" json:"rule,omitempty"`
	Title     string    `gorm:"size:512" json:"title"`
	Content   string    `gorm:"type:text" json:"content"`
	Status    string    `gorm:"size:16;default:pending" json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}
