package repository

import (
	"gorm.io/gorm"
	"opinion-analysis/src/model"
)

type WorkflowRepository struct {
	db *gorm.DB
}

func NewWorkflowRepository(db *gorm.DB) *WorkflowRepository {
	return &WorkflowRepository{db: db}
}

// List 获取工作流列表
func (r *WorkflowRepository) List(page, pageSize int) ([]model.Workflow, int64, error) {
	var workflows []model.Workflow
	var total int64

	offset := (page - 1) * pageSize
	err := r.db.Model(&model.Workflow{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = r.db.Order("created_at desc").Offset(offset).Limit(pageSize).Find(&workflows).Error
	return workflows, total, err
}

// FindByID 根据ID查询工作流
func (r *WorkflowRepository) FindByID(id int64) (*model.Workflow, error) {
	var workflow model.Workflow
	err := r.db.First(&workflow, id).Error
	if err != nil {
		return nil, err
	}
	return &workflow, nil
}

// Create 创建工作流
func (r *WorkflowRepository) Create(workflow *model.Workflow) error {
	return r.db.Create(workflow).Error
}

// Update 更新工作流
func (r *WorkflowRepository) Update(workflow *model.Workflow) error {
	return r.db.Save(workflow).Error
}

// Delete 删除工作流
func (r *WorkflowRepository) Delete(id int64) error {
	return r.db.Delete(&model.Workflow{}, id).Error
}

// FindActiveScheduledWorkflows 查询所有启用的定时工作流
func (r *WorkflowRepository) FindActiveScheduledWorkflows() ([]model.Workflow, error) {
	var workflows []model.Workflow
	err := r.db.Where("status = ? AND trigger_type = ?", 1, "schedule").Find(&workflows).Error
	return workflows, err
}

// WorkflowExecutionRepository 工作流执行记录仓储
type WorkflowExecutionRepository struct {
	db *gorm.DB
}

func NewWorkflowExecutionRepository(db *gorm.DB) *WorkflowExecutionRepository {
	return &WorkflowExecutionRepository{db: db}
}

// Create 创建执行记录
func (r *WorkflowExecutionRepository) Create(execution *model.WorkflowExecution) error {
	return r.db.Create(execution).Error
}

// Update 更新执行记录
func (r *WorkflowExecutionRepository) Update(execution *model.WorkflowExecution) error {
	// 只更新非零值字段
	return r.db.Model(&model.WorkflowExecution{}).
		Where("id = ?", execution.ID).
		Updates(map[string]interface{}{
			"status":      execution.Status,
			"finished_at": execution.FinishedAt,
			"error_msg":   execution.ErrorMsg,
		}).Error
}

// FindByID 根据ID查询执行记录
func (r *WorkflowExecutionRepository) FindByID(id int64) (*model.WorkflowExecution, error) {
	var execution model.WorkflowExecution
	err := r.db.First(&execution, id).Error
	if err != nil {
		return nil, err
	}
	return &execution, nil
}

// ListByWorkflowID 查询工作流的执行历史
func (r *WorkflowExecutionRepository) ListByWorkflowID(workflowID int64, page, pageSize int) ([]model.WorkflowExecution, int64, error) {
	var executions []model.WorkflowExecution
	var total int64

	offset := (page - 1) * pageSize
	query := r.db.Model(&model.WorkflowExecution{}).Where("workflow_id = ?", workflowID)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Order("started_at desc").Offset(offset).Limit(pageSize).Find(&executions).Error
	return executions, total, err
}

// FindLatestRunning 查找最近创建的运行中的执行记录
func (r *WorkflowExecutionRepository) FindLatestRunning(workflowID int64) (*model.WorkflowExecution, error) {
	var execution model.WorkflowExecution
	err := r.db.Where("workflow_id = ? AND status = ?", workflowID, "running").
		Order("id desc").
		First(&execution).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &execution, nil
}

// WorkflowNodeExecutionRepository 节点执行记录仓储
type WorkflowNodeExecutionRepository struct {
	db *gorm.DB
}

func NewWorkflowNodeExecutionRepository(db *gorm.DB) *WorkflowNodeExecutionRepository {
	return &WorkflowNodeExecutionRepository{db: db}
}

// Create 创建节点执行记录
func (r *WorkflowNodeExecutionRepository) Create(nodeExecution *model.WorkflowNodeExecution) error {
	return r.db.Create(nodeExecution).Error
}

// Update 更新节点执行记录
func (r *WorkflowNodeExecutionRepository) Update(nodeExecution *model.WorkflowNodeExecution) error {
	// 只更新非零值字段
	return r.db.Model(&model.WorkflowNodeExecution{}).
		Where("id = ?", nodeExecution.ID).
		Updates(map[string]interface{}{
			"status":      nodeExecution.Status,
			"output":      nodeExecution.Output,
			"error_msg":   nodeExecution.ErrorMsg,
			"finished_at": nodeExecution.FinishedAt,
		}).Error
}

// ListByExecutionID 查询执行记录的所有节点执行日志
func (r *WorkflowNodeExecutionRepository) ListByExecutionID(executionID int64) ([]model.WorkflowNodeExecution, error) {
	var nodeExecutions []model.WorkflowNodeExecution
	err := r.db.Where("execution_id = ?", executionID).Order("started_at asc").Find(&nodeExecutions).Error
	return nodeExecutions, err
}
