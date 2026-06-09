package user

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/model"
	"opinion-analysis/src/service/report"
)

type ReportHandler struct {
	reportSvc *report.Service
	db        *gorm.DB
}

func NewReportHandler(reportSvc *report.Service, db *gorm.DB) *ReportHandler {
	return &ReportHandler{reportSvc: reportSvc, db: db}
}

// Download 下载分析报告
func (h *ReportHandler) Download(c *gin.Context) {
	reportID := c.Param("reportId")
	if reportID == "" {
		response.Fail(c, http.StatusBadRequest, "reportId is required")
		return
	}

	meta, err := h.reportSvc.Get(c.Request.Context(), reportID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "报告不存在或已过期"})
		return
	}

	filename := fmt.Sprintf("report-%d", meta.CrawlerRunID)
	switch meta.Format {
	case report.FormatHTML:
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.html"`, filename))
	default:
		c.Header("Content-Type", "text/markdown; charset=utf-8")
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.md"`, filename))
	}

	c.String(http.StatusOK, meta.Content)
}

// Info 查询报告元数据（供前端展示下载按钮）
func (h *ReportHandler) Info(c *gin.Context) {
	reportID := c.Param("reportId")
	if reportID == "" {
		response.Fail(c, http.StatusBadRequest, "reportId is required")
		return
	}

	meta, err := h.reportSvc.Get(c.Request.Context(), reportID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "报告不存在或已过期"})
		return
	}

	response.OK(c, gin.H{
		"reportId":     reportID,
		"format":       meta.Format,
		"crawlerRunId": meta.CrawlerRunID,
		"articleCount": meta.ArticleCount,
		"commentCount": meta.CommentCount,
		"platforms":    meta.Platforms,
		"topics":       meta.Topics,
		"createdAt":    meta.CreatedAt,
		"downloadUrl":  "/api/reports/" + reportID + "/download",
	})
}

// Regenerate 根据工作流执行记录重新生成报告（复用原始爬取数据和节点配置）
func (h *ReportHandler) Regenerate(c *gin.Context) {
	var req struct {
		ExecutionID int64  `json:"executionId" binding:"required"`
		Format      string `json:"format"`
		HTMLTheme   string `json:"htmlTheme"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "executionId is required")
		return
	}

	// 1. 查找执行记录，获取 workflowId
	var execution model.WorkflowExecution
	if err := h.db.First(&execution, req.ExecutionID).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "execution not found")
		return
	}

	// 2. 查找该执行中 analysis_report 节点的日志，提取 articleIds
	var nodeLogs []model.WorkflowNodeExecution
	h.db.Where("execution_id = ?", req.ExecutionID).Order("started_at ASC").Find(&nodeLogs)

	var articleIDs []int64
	var platforms []string
	var topics []string
	var crawlerRunID uint
	var reportNodeConfig map[string]interface{}

	// 从节点日志中找到 report 节点的 input（包含 articleIds）
	for _, log := range nodeLogs {
		if log.Input == nil {
			continue
		}
		var input map[string]interface{}
		if err := json.Unmarshal(log.Input, &input); err != nil {
			continue
		}
		// 找到包含 reportId 输出的节点（即 analysis_report 节点）
		if log.Output != nil {
			var output map[string]interface{}
			if err := json.Unmarshal(log.Output, &output); err == nil {
				if _, hasReport := output["reportId"]; hasReport {
					articleIDs = unpackArticleIDsFromJSON(input["articleIds"])
					platforms = unpackStringSlice(input["syncPlatformCodes"])
					if len(platforms) == 0 {
						platforms = unpackStringSlice(input["platforms"])
					}
					topics = unpackStringSlice(input["topics"])
					if rid, ok := input["crawlerRunId"].(float64); ok {
						crawlerRunID = uint(rid)
					}
					break
				}
			}
		}
	}

	if len(articleIDs) == 0 {
		response.Fail(c, http.StatusBadRequest, "no articleIds found in execution logs")
		return
	}

	// 3. 从 workflow 的 nodes 定义中提取 analysis_report 节点的 config
	var workflow model.Workflow
	if err := h.db.First(&workflow, execution.WorkflowID).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "workflow not found")
		return
	}

	var nodeList []map[string]interface{}
	if err := json.Unmarshal(workflow.Nodes, &nodeList); err == nil {
		for _, node := range nodeList {
			nodeType := ""
			if t, ok := node["type"].(string); ok {
				nodeType = t
			}
			if nodeType == "" {
				if data, ok := node["data"].(map[string]interface{}); ok {
					if t, ok := data["nodeType"].(string); ok {
						nodeType = t
					}
				}
			}
			if nodeType == "analysis_report" {
				if cfg, ok := node["config"].(map[string]interface{}); ok {
					reportNodeConfig = cfg
				} else if data, ok := node["data"].(map[string]interface{}); ok {
					if cfg, ok := data["config"].(map[string]interface{}); ok {
						reportNodeConfig = cfg
					}
				}
				break
			}
		}
	}

	// 4. 确定报告参数（请求参数 > 节点配置 > 默认值）
	format := req.Format
	if format == "" && reportNodeConfig != nil {
		if f, ok := reportNodeConfig["format"].(string); ok {
			format = f
		}
	}
	if format == "" {
		format = "html"
	}

	htmlTheme := req.HTMLTheme
	if htmlTheme == "" && reportNodeConfig != nil {
		if t, ok := reportNodeConfig["htmlTheme"].(string); ok {
			htmlTheme = t
		}
	}
	if htmlTheme == "" {
		htmlTheme = "random"
	}

	sampleSize := 8
	maxGroups := 5
	if reportNodeConfig != nil {
		if s, ok := reportNodeConfig["sampleSize"].(float64); ok && int(s) > 0 {
			sampleSize = int(s)
		}
		if m, ok := reportNodeConfig["maxGroups"].(float64); ok && int(m) > 0 {
			maxGroups = int(m)
		}
	}

	// 5. 生成报告
	reportID, err := h.reportSvc.Generate(
		c.Request.Context(),
		articleIDs,
		crawlerRunID,
		platforms,
		topics,
		report.Format(format),
		htmlTheme,
		sampleSize,
		maxGroups,
	)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "regenerate report failed: "+err.Error())
		return
	}

	response.OK(c, gin.H{
		"reportId":     reportID,
		"format":       format,
		"articleCount": len(articleIDs),
		"downloadUrl":  "/api/reports/" + reportID + "/download",
		"previewUrl":   "/api/reports/" + reportID + "/download",
	})
}

func unpackArticleIDsFromJSON(val interface{}) []int64 {
	if val == nil {
		return nil
	}
	arr, ok := val.([]interface{})
	if !ok {
		return nil
	}
	result := make([]int64, 0, len(arr))
	for _, v := range arr {
		switch id := v.(type) {
		case float64:
			result = append(result, int64(id))
		case int64:
			result = append(result, id)
		}
	}
	return result
}

func unpackStringSlice(val interface{}) []string {
	if val == nil {
		return nil
	}
	arr, ok := val.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
