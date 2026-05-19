package repository

import (
	"errors"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) FindByUsername(username string) (*model.User, error) {
	var user model.User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByID(id uint) (*model.User, error) {
	var user model.User
	err := r.db.First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) ExistsByUsernameOrEmail(username, email string) (bool, error) {
	var count int64
	err := r.db.Model(&model.User{}).Where("username = ? OR email = ?", username, email).Count(&count).Error
	return count > 0, err
}

func (r *UserRepository) Create(user *model.User) error {
	return r.db.Create(user).Error
}

func (r *UserRepository) CountActiveAdmins() (int64, error) {
	var cnt int64
	err := r.db.Model(&model.User{}).Where("role = ? AND status = ?", "admin", 1).Count(&cnt).Error
	return cnt, err
}

func (r *UserRepository) FirstOrCreate(user *model.User) error {
	return r.db.Where("username = ?", user.Username).FirstOrCreate(user, user).Error
}

func (r *UserRepository) UpdateFields(id uint, updates map[string]interface{}) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

func (r *UserRepository) UpdatePassword(id uint, hash string) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("password", hash).Error
}

func (r *UserRepository) Delete(id uint) error {
	return r.db.Delete(&model.User{}, id).Error
}

type UserListFilter struct {
	Keyword  string
	Role     string
	Page     int
	PageSize int
}

func (r *UserRepository) List(filter UserListFilter) ([]model.User, int64, error) {
	q := r.db.Model(&model.User{})
	if filter.Keyword != "" {
		like := "%" + filter.Keyword + "%"
		q = q.Where("username LIKE ? OR email LIKE ? OR nickname LIKE ?", like, like, like)
	}
	if filter.Role != "" {
		q = q.Where("role = ?", filter.Role)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.User
	offset := (filter.Page - 1) * filter.PageSize
	err := q.Order("id desc").Offset(offset).Limit(filter.PageSize).Find(&list).Error
	return list, total, err
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
