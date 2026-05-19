package repository

import (
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
)

type ChatRepository struct {
	db *gorm.DB
}

func NewChatRepository(db *gorm.DB) *ChatRepository {
	return &ChatRepository{db: db}
}

func (r *ChatRepository) CreateSession(session *model.ChatSession) error {
	return r.db.Create(session).Error
}

func (r *ChatRepository) ListSessionsByUser(userID uint, limit int) ([]model.ChatSession, error) {
	var sessions []model.ChatSession
	err := r.db.Where("user_id = ?", userID).Order("updated_at DESC").Limit(limit).Find(&sessions).Error
	return sessions, err
}

func (r *ChatRepository) FindSessionForUser(sessionID, userID uint) (*model.ChatSession, error) {
	var session model.ChatSession
	err := r.db.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *ChatRepository) ListMessages(sessionID uint) ([]model.ChatMessage, error) {
	var messages []model.ChatMessage
	err := r.db.Where("session_id = ?", sessionID).Order("created_at ASC").Find(&messages).Error
	return messages, err
}

func (r *ChatRepository) DeleteMessagesBySession(sessionID uint) error {
	return r.db.Where("session_id = ?", sessionID).Delete(&model.ChatMessage{}).Error
}

func (r *ChatRepository) DeleteSession(session *model.ChatSession) error {
	return r.db.Delete(session).Error
}

func (r *ChatRepository) RenameSession(sessionID, userID uint, title string) (int64, error) {
	result := r.db.Model(&model.ChatSession{}).
		Where("id = ? AND user_id = ?", sessionID, userID).
		Updates(map[string]interface{}{
			"title":      title,
			"updated_at": time.Now(),
		})
	return result.RowsAffected, result.Error
}

func (r *ChatRepository) CreateMessage(msg *model.ChatMessage) error {
	return r.db.Create(msg).Error
}

func (r *ChatRepository) UpdateSession(sessionID uint, updates map[string]interface{}) error {
	return r.db.Model(&model.ChatSession{}).Where("id = ?", sessionID).Updates(updates).Error
}
