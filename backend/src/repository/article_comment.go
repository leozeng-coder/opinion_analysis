package repository

import (
	"opinion-analysis/src/model"

	"gorm.io/gorm"
)

type ArticleCommentRepository struct {
	db *gorm.DB
}

func NewArticleCommentRepository(db *gorm.DB) *ArticleCommentRepository {
	return &ArticleCommentRepository{db: db}
}

func (r *ArticleCommentRepository) FindByArticle(articleID uint) ([]model.ArticleComment, error) {
	var list []model.ArticleComment
	err := r.db.Where("article_id = ?", articleID).Order("published_at asc").Find(&list).Error
	return list, err
}

func (r *ArticleCommentRepository) FindByID(id uint) (*model.ArticleComment, error) {
	var c model.ArticleComment
	err := r.db.First(&c, id).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *ArticleCommentRepository) UpdateContent(id uint, content string) error {
	return r.db.Model(&model.ArticleComment{}).Where("id = ?", id).Update("content", content).Error
}

func (r *ArticleCommentRepository) Delete(id uint) error {
	return r.db.Delete(&model.ArticleComment{}, id).Error
}
