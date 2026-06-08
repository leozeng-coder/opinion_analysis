package user

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/service/report"
)

type ReportHandler struct {
	reportSvc *report.Service
}

func NewReportHandler(reportSvc *report.Service) *ReportHandler {
	return &ReportHandler{reportSvc: reportSvc}
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
