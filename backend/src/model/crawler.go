package model

import "time"

// CrawlerSpiderConfig 爬虫调度配置（与 Python scheduler 共用表）
type CrawlerSpiderConfig struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	SpiderKey       string    `gorm:"uniqueIndex;size:32;not null" json:"spiderKey"`
	DisplayName     string    `gorm:"size:64" json:"displayName"`
	IntervalMinutes int       `gorm:"not null" json:"intervalMinutes"`
	Enabled         int8      `gorm:"default:1" json:"enabled"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// CrawlerRunLog 立即执行记录（API 触发的子进程）
type CrawlerRunLog struct {
	ID             uint       `gorm:"primarykey" json:"id"`
	Spiders        string     `gorm:"size:256" json:"spiders"`
	Mode           string     `gorm:"size:16" json:"mode"`
	Params         string     `gorm:"type:longtext" json:"params"`
	Status         string     `gorm:"size:16;index" json:"status"`
	Message        string     `gorm:"type:text" json:"message"`
	Progress       int        `gorm:"default:0" json:"progress"`
	ProgressDetail string     `gorm:"type:text" json:"progressDetail"`
	TriggeredBy    uint       `json:"triggeredBy"`
	StartedAt      time.Time  `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
}
