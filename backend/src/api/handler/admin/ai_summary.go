package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/digest"
	"opinion-analysis/src/service/tagger"
)

type AISummaryHandler struct {
	gen    *digest.Generator
	digest *repository.DigestRepository
}

func NewAISummaryHandler(db *gorm.DB, taggerSvc *tagger.Service, digestRepo *repository.DigestRepository) *AISummaryHandler {
	return &AISummaryHandler{
		gen:    digest.NewGenerator(db, digestRepo, taggerSvc),
		digest: digestRepo,
	}
}

// Get returns the current stored daily digest.
func (h *AISummaryHandler) Get(c *gin.Context) {
	if h.digest == nil {
		response.Fail(c, http.StatusServiceUnavailable, "Redis 未配置，摘要功能不可用")
		return
	}
	d, err := h.digest.Get("")
	if err != nil {
		response.ServerError(c)
		return
	}
	if d == nil {
		response.OK(c, nil)
		return
	}
	response.OK(c, gin.H{
		"date":     d.Date,
		"text":     d.Text,
		"keywords": d.Keywords,
	})
}

type regenerateRequest struct {
	Topics    []string `json:"topics"`
	Platforms []string `json:"platforms"`
	StartDate string   `json:"startDate"`
	EndDate   string   `json:"endDate"`
	Limit     int      `json:"limit"`
}

// Regenerate triggers a fresh digest generation with optional filters and returns the result.
func (h *AISummaryHandler) Regenerate(c *gin.Context) {
	if h.gen == nil {
		response.Fail(c, http.StatusServiceUnavailable, "摘要生成器未初始化")
		return
	}
	if h.digest == nil {
		response.Fail(c, http.StatusServiceUnavailable, "Redis 未配置，摘要功能不可用")
		return
	}

	var req regenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	opts := digest.FilterOptions{
		Topics:    req.Topics,
		Platforms: req.Platforms,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		Limit:     req.Limit,
	}

	if err := h.gen.GenerateWithFilters(c.Request.Context(), opts); err != nil {
		msg := err.Error()
		switch msg {
		case "no articles found for given filters":
			response.Fail(c, http.StatusNotFound, "该条件下未找到相关文章，请调整话题、平台或时间范围")
		case "LLM API key not configured", "api key not configured":
			response.Fail(c, http.StatusBadRequest, "大模型 API Key 未配置，请先在「配置」页面完成设置")
		default:
			response.Fail(c, http.StatusInternalServerError, "生成失败："+msg)
		}
		return
	}

	d, err := h.digest.Get("")
	if err != nil || d == nil {
		response.OK(c, gin.H{"message": "生成成功"})
		return
	}
	response.OK(c, gin.H{
		"date":     d.Date,
		"text":     d.Text,
		"keywords": d.Keywords,
	})
}
