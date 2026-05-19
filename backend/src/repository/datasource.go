package repository

import (
	"gorm.io/gorm"
	"opinion-analysis/src/model"
)

type DataSourceRepository struct {
	db *gorm.DB
}

func NewDataSourceRepository(db *gorm.DB) *DataSourceRepository {
	return &DataSourceRepository{db: db}
}

func (r *DataSourceRepository) List() ([]model.DataSource, error) {
	var list []model.DataSource
	err := r.db.Order("id asc").Find(&list).Error
	return list, err
}

func (r *DataSourceRepository) Create(ds *model.DataSource) error {
	return r.db.Create(ds).Error
}

func (r *DataSourceRepository) Update(id uint, updates map[string]interface{}) error {
	return r.db.Model(&model.DataSource{}).Where("id = ?", id).Updates(updates).Error
}

func (r *DataSourceRepository) FindByID(id uint) (*model.DataSource, error) {
	var out model.DataSource
	err := r.db.First(&out, id).Error
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *DataSourceRepository) Delete(id uint) error {
	return r.db.Delete(&model.DataSource{}, id).Error
}
