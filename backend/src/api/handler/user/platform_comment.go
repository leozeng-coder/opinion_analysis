package user

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/pkg/response"
)

// GetPlatformComments 获取平台评论
// @Summary 获取平台评论
// @Tags Platform
// @Accept json
// @Produce json
// @Param platform query string true "平台类型 (xhs/dy/bili/wb/ks/tieba/zhihu)"
// @Param itemId query int true "内容ID"
// @Param page query int true "页码"
// @Param pageSize query int true "每页数量"
// @Success 200 {object} model.PlatformCommentResponse
// @Router /api/platform/comments [get]
func GetPlatformComments(c *gin.Context) {
	var query model.PlatformCommentQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}

	db, exists := c.Get("db")
	if !exists {
		response.Fail(c, 500, "database connection not found")
		return
	}

	repo := repository.NewPlatformCommentRepository(db.(*gorm.DB))
	items, total, err := repo.QueryPlatformComments(c.Request.Context(), query)
	if err != nil {
		response.Fail(c, 500, "query error: "+err.Error())
		return
	}

	response.OK(c, model.PlatformCommentResponse{
		Data:  items,
		Total: total,
	})
}
