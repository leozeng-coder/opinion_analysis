package model

import "time"

// PlatformSyncOffset 平台同步偏移量（每平台一行）。
//
// 记录「已成功同步进 articles 的源表最大主键 id」。同步时只处理源表中
// id > LastSourceID 的行，成功后把偏移量推进到本次扫描时源表的 max(id)。
//
// 该机制与具体平台完全无关：新增平台只需实现 PlatformSyncer 接口并在
// SyncerFactory 注册，即可自动获得「增量 + 不漏行」能力，无需改动偏移量逻辑。
type PlatformSyncOffset struct {
	Platform     string    `gorm:"primaryKey;size:32" json:"platform"`
	SourceTable  string    `gorm:"size:64" json:"sourceTable"`
	LastSourceID uint      `gorm:"not null;default:0" json:"lastSourceId"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func (PlatformSyncOffset) TableName() string { return "platform_sync_offset" }
