package model

import (
	"time"

	"gorm.io/gorm"
)

// ArticleComment 文章评论（从平台评论表同步而来）
type ArticleComment struct {
	ID                uint           `gorm:"primarykey" json:"id"`
	ArticleID         uint           `gorm:"index;not null" json:"articleId"`
	PlatformCommentID string         `gorm:"size:255;uniqueIndex" json:"platformCommentId"`
	ParentID          *uint          `gorm:"index" json:"parentId"`
	Content           string         `gorm:"type:text" json:"content"`
	Nickname          string         `gorm:"size:128" json:"nickname"`
	LikeCount         int            `gorm:"default:0" json:"likeCount"`
	IPLocation        string         `gorm:"size:64" json:"ipLocation"`
	PublishedAt       time.Time      `json:"publishedAt"`
	CreatedAt         time.Time      `json:"createdAt"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

func (ArticleComment) TableName() string { return "article_comments" }
