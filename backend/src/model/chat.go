package model

import (
	"time"

	"gorm.io/gorm"
)

// ChatSession AI 对话会话（按用户隔离）
type ChatSession struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	UserID    uint           `gorm:"index;not null" json:"userId"`
	Title     string         `gorm:"size:256;not null" json:"title"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// ChatMessage AI 对话消息
type ChatMessage struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	SessionID uint      `gorm:"index;not null" json:"sessionId"`
	Role      string    `gorm:"size:16;not null" json:"role"`
	Content   string    `gorm:"type:longtext;not null" json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}
