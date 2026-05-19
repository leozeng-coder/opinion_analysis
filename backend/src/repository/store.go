package repository

import "gorm.io/gorm"

// Store aggregates domain repositories.
type Store struct {
	User       *UserRepository
	Article    *ArticleRepository
	Topic      *TopicRepository
	Alert      *AlertRepository
	Crawler    *CrawlerRepository
	Chat       *ChatRepository
	System     *SystemRepository
	Audit      *AuditRepository
	DataSource *DataSourceRepository
	RAG        *RAGRepository
}

func NewStore(db *gorm.DB) *Store {
	return &Store{
		User:       NewUserRepository(db),
		Article:    NewArticleRepository(db),
		Topic:      NewTopicRepository(db),
		Alert:      NewAlertRepository(db),
		Crawler:    NewCrawlerRepository(db),
		Chat:       NewChatRepository(db),
		System:     NewSystemRepository(db),
		Audit:      NewAuditRepository(db),
		DataSource: NewDataSourceRepository(db),
		RAG:        NewRAGRepository(db),
	}
}
