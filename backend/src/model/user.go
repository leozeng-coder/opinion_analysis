package model

import (
	"time"

	"gorm.io/gorm"
)

// User 系统用户
type User struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	Username  string         `gorm:"uniqueIndex;size:64;not null" json:"username"`
	Password  string         `gorm:"size:256;not null" json:"-"`
	Email     string         `gorm:"uniqueIndex;size:128" json:"email"`
	Nickname  string         `gorm:"size:64" json:"nickname"`
	Role      string         `gorm:"size:32;default:viewer" json:"role"` // admin | analyst | viewer
	Status    int8           `gorm:"default:1" json:"status"`            // 1=active 0=disabled
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
