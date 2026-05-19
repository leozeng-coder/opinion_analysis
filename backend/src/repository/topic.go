package repository

import (
	"gorm.io/gorm"
	"opinion-analysis/src/model"
)

type TopicRepository struct {
	db *gorm.DB
}

func NewTopicRepository(db *gorm.DB) *TopicRepository {
	return &TopicRepository{db: db}
}

func (r *TopicRepository) List(page, pageSize int) ([]model.Topic, int64, error) {
	var total int64
	r.db.Model(&model.Topic{}).Count(&total)
	var list []model.Topic
	offset := (page - 1) * pageSize
	err := r.db.Order("heat_score desc").Offset(offset).Limit(pageSize).Find(&list).Error
	return list, total, err
}

func (r *TopicRepository) FindByID(id string) (*model.Topic, error) {
	var topic model.Topic
	err := r.db.First(&topic, id).Error
	if err != nil {
		return nil, err
	}
	return &topic, nil
}
