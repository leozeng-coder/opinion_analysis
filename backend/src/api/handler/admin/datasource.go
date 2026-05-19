package admin

import (
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"
	"opinion-analysis/pkg/response"
	"opinion-analysis/pkg/utils"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
)

type DataSourceHandler struct {
	sources *repository.DataSourceRepository
}

func NewDataSourceHandler(store *repository.Store) *DataSourceHandler {
	return &DataSourceHandler{sources: store.DataSource}
}

func maskDataSource(ds model.DataSource) model.DataSource {
	if ds.Config == "" {
		return ds
	}
	ds.Config = utils.MaskSensitive([]byte(ds.Config))
	return ds
}

func (h *DataSourceHandler) List(c *gin.Context) {
	list, err := h.sources.List()
	if err != nil {
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
	Name   string `json:"name" binding:"required"`
	Type   string `json:"type" binding:"required"`
	URL    string `json:"url"`
	Config string `json:"config"`
	Status *int8  `json:"status"`
}

func validateDSReq(req dataSourceReq) error {
	if req.Config != "" {
		var v any
		return json.Unmarshal([]byte(req.Config), &v)
	}
	return nil
}

func (h *DataSourceHandler) Create(c *gin.Context) {
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
	if err := h.sources.Create(&ds); err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, maskDataSource(ds))
}

func (h *DataSourceHandler) Update(c *gin.Context) {
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
	if err := h.sources.Update(uint(id), updates); err != nil {
		response.ServerError(c)
		return
	}
	out, err := h.sources.FindByID(uint(id))
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, maskDataSource(*out))
}

func (h *DataSourceHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.Fail(c, 400, "invalid id")
		return
	}
	if err := h.sources.Delete(uint(id)); err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, nil)
}
