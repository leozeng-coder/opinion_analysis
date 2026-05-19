package model

// AllModels returns GORM entities for AutoMigrate.
func AllModels() []any {
	return []any{
		&User{},
		&DataSource{},
		&Article{},
		&Topic{},
		&AlertRule{},
		&AlertRecord{},
		&CrawlerSpiderConfig{},
		&CrawlerRunLog{},
		&Report{},
		&SystemSetting{},
		&AuditLog{},
		&RagSyncLog{},
		&ChatSession{},
		&ChatMessage{},
	}
}
