package handler

import (
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"opinion-analysis/internal/model"
	"opinion-analysis/pkg/response"
	"opinion-analysis/pkg/utils"
)

type AdminDataSourceHandler struct {
	db *gorm.DB
}

func NewAdminDataSourceHandler(db *gorm.DB) *AdminDataSourceHandler {
	return &AdminDataSourceHandler{db: db}
}

func maskDataSource(ds model.DataSource) model.DataSource {
	if ds.Config == "" {
		return ds
	}
	ds.Config = utils.MaskSensitive([]byte(ds.Config))
	return ds
}

func (h *AdminDataSourceHandler) List(c *gin.Context) {
	var list []model.DataSource
	if err := h.db.Order("id asc").Find(&list).Error; err != nil {
		response.ServerError(c)
		return
	}
	masked := make([]model.DataSource, len(list))
	for i, ds := range list {
		masked[i] = maskDataSource(ds)
	}
	response.OK(c, masked)
}

type dataSourceReq struct {
	Name   string  `json:"name" binding:"required"`
	Type   string  `json:"type" binding:"required"` // weibo|weixin|news|forum|xhs|dy|ks|bili|tieba|zhihu
	URL    string  `json:"url"`
	Config string  `json:"config"`
	Status *int8   `json:"status"`
}

func validateDSReq(req dataSourceReq) error {
	if req.Config != "" {
		var v any
		return json.Unmarshal([]byte(req.Config), &v)
	}
	return nil
}

func (h *AdminDataSourceHandler) Create(c *gin.Context) {
	var req dataSourceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	if err := validateDSReq(req); err != nil {
		response.Fail(c, 400, "config must be valid JSON: "+err.Error())
		return
	}
	status := int8(1)
	if req.Status != nil {
		status = *req.Status
	}
	ds := model.DataSource{
		Name:   req.Name,
		Type:   req.Type,
		URL:    req.URL,
		Config: req.Config,
		Status: status,
	}
	if err := h.db.Create(&ds).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, maskDataSource(ds))
}

func (h *AdminDataSourceHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.Fail(c, 400, "invalid id")
		return
	}
	var req dataSourceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	if err := validateDSReq(req); err != nil {
		response.Fail(c, 400, "config must be valid JSON: "+err.Error())
		return
	}
	updates := map[string]interface{}{
		"name": req.Name,
		"type": req.Type,
		"url":  req.URL,
	}
	if req.Config != "" {
		updates["config"] = req.Config
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if err := h.db.Model(&model.DataSource{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.ServerError(c)
		return
	}
	var out model.DataSource
	h.db.First(&out, id)
	response.OK(c, maskDataSource(out))
}

func (h *AdminDataSourceHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.Fail(c, 400, "invalid id")
		return
	}
	if err := h.db.Delete(&model.DataSource{}, id).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, nil)
}
