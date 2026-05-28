package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/workflow"
)

type WorkflowHandler struct {
	store  *repository.Store
	engine *workflow.Engine
}

func NewWorkflowHandler(store *repository.Store, engine *workflow.Engine) *WorkflowHandler {
	return &WorkflowHandler{
		store:  store,
		engine: engine,
	}
}

// List 获取工作流列表
func (h *WorkflowHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))

	list, total, err := h.store.Workflow.List(page, pageSize)
	if err != nil {
		response.ServerError(c)
		return
	}

	response.OK(c, gin.H{
		"list":  list,
		"total": total,
	})
}

// Detail 获取工作流详情
func (h *WorkflowHandler) Detail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	workflow, err := h.store.Workflow.FindByID(id)
	if err != nil {
		response.Fail(c, http.StatusNotFound, "workflow not found")
		return
	}

	response.OK(c, workflow)
}

// Create 创建工作流
func (h *WorkflowHandler) Create(c *gin.Context) {
	var req model.Workflow
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	// 设置默认状态
	if req.Status == 0 {
		req.Status = 1
	}

	if err := h.store.Workflow.Create(&req); err != nil {
		response.ServerError(c)
		return
	}

	response.OK(c, req)
}

// Update 更新工作流
func (h *WorkflowHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	var req model.Workflow
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	// 检查工作流是否存在
	existing, err := h.store.Workflow.FindByID(id)
	if err != nil {
		response.Fail(c, http.StatusNotFound, "workflow not found")
		return
	}

	// 更新字段
	existing.Name = req.Name
	existing.Description = req.Description
	existing.Status = req.Status
	existing.TriggerType = req.TriggerType
	existing.TriggerConfig = req.TriggerConfig
	existing.Nodes = req.Nodes
	existing.Edges = req.Edges

	if err := h.store.Workflow.Update(existing); err != nil {
		response.ServerError(c)
		return
	}

	response.OK(c, existing)
}

// Delete 删除工作流
func (h *WorkflowHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.store.Workflow.Delete(id); err != nil {
		response.ServerError(c)
		return
	}

	response.OK(c, gin.H{"message": "workflow deleted"})
}

// Execute 手动执行工作流
func (h *WorkflowHandler) Execute(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Input map[string]interface{} `json:"input"`
	}
	c.ShouldBindJSON(&req)

	// 创建执行记录并异步执行
	execution := &model.WorkflowExecution{
		WorkflowID: id,
		Status:     "running",
		StartedAt:  time.Now(),
	}

	// 先创建执行记录
	if err := h.store.WorkflowExecution.Create(execution); err != nil {
		response.ServerError(c)
		return
	}

	// 异步执行工作流
	go func() {
		// 使用独立的 context，不依赖 HTTP 请求的 context
		ctx := context.Background()
		_, err := h.engine.Execute(ctx, id, req.Input)
		if err != nil {
			// 错误已经在 Engine 中记录
		}
	}()

	response.OK(c, gin.H{
		"id":      execution.ID,
		"message": "workflow execution started",
	})
}

// Executions 获取工作流执行历史
func (h *WorkflowHandler) Executions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))

	list, total, err := h.store.WorkflowExecution.ListByWorkflowID(id, page, pageSize)
	if err != nil {
		response.ServerError(c)
		return
	}

	response.OK(c, gin.H{
		"list":  list,
		"total": total,
	})
}

// ExecutionLogs 获取执行日志
func (h *WorkflowHandler) ExecutionLogs(c *gin.Context) {
	execID, err := strconv.ParseInt(c.Param("execId"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid execution id")
		return
	}

	logs, err := h.store.WorkflowNodeExecution.ListByExecutionID(execID)
	if err != nil {
		response.ServerError(c)
		return
	}

	response.OK(c, logs)
}
