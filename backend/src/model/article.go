package model

import (
	"time"

	"gorm.io/gorm"
)

// Article 舆情文章/信息
type Article struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	SourceID    uint           `gorm:"index" json:"sourceId"`
	Source      DataSource     `gorm:"foreignKey:SourceID" json:"source,omitempty"`
	Title       string         `gorm:"size:512" json:"title"`
	Content     string         `gorm:"type:longtext" json:"content"`
	Author      string         `gorm:"size:128" json:"author"`
	OriginURL   string         `gorm:"size:1024" json:"originUrl"`
	Platform       string         `gorm:"size:32;index" json:"platform"`
	PlatformItemID string         `gorm:"size:255;index" json:"platformItemId"`
	Topic          string         `gorm:"size:64;index" json:"topic"`
	Sentiment      string         `gorm:"size:16;index" json:"sentiment"`
	SentScore   float64        `json:"sentScore"`
	Keywords    string         `gorm:"type:json" json:"keywords"`
	AITags      *string        `gorm:"column:ai_tags;type:json" json:"aiTags"`
	PublishedAt time.Time      `gorm:"index" json:"publishedAt"`
	// 向量知识库同步：content 变更时需重算 embedding
	EmbeddingContentHash *string        `gorm:"size:64;column:embedding_content_hash" json:"-"`
	EmbeddingSyncedAt    *time.Time     `gorm:"column:embedding_synced_at" json:"-"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
	DeletedAt            gorm.DeletedAt `gorm:"index" json:"-"`
}
