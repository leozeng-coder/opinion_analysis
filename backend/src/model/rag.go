package model

import "time"

// RagSyncLog 向量知识库增量同步任务记录
type RagSyncLog struct {
	ID                uint       `gorm:"primarykey" json:"id"`
	Status            string     `gorm:"size:16;index" json:"status"`
	Progress          int        `gorm:"default:0" json:"progress"`
	ProgressDetail    string     `gorm:"type:text" json:"progressDetail"`
	Message           string     `gorm:"type:text" json:"message"`
	ArticlesProcessed int        `json:"articlesProcessed"`
	ChunksUpserted    int        `json:"chunksUpserted"`
	ChunksDeleted     int        `json:"chunksDeleted"`
	Mode              string     `gorm:"size:16" json:"mode"`
	StartedAt         time.Time  `json:"startedAt"`
	FinishedAt        *time.Time `json:"finishedAt,omitempty"`
}

func (RagSyncLog) TableName() string { return "rag_sync_logs" }
