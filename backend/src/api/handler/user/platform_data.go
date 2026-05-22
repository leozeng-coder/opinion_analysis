package user

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/pkg/response"
)

// GetPlatformData 获取平台数据
// @Summary 获取平台数据
// @Tags Platform
// @Accept json
// @Produce json
// @Param platform query string false "平台类型 (xhs/dy/bili/wb/ks/tieba/zhihu)"
// @Param startDate query string false "开始日期 (YYYY-MM-DD)"
// @Param endDate query string false "结束日期 (YYYY-MM-DD)"
// @Param page query int true "页码"
// @Param pageSize query int true "每页数量"
// @Success 200 {object} model.PlatformDataResponse
// @Router /api/platform/data [get]
func GetPlatformData(c *gin.Context) {
	var query model.PlatformDataQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}

	// 获取数据库连接
	db, exists := c.Get("db")
	if !exists {
		response.Fail(c, 500, "database connection not found")
		return
	}

	repo := repository.NewPlatformDataRepository(db.(*gorm.DB))
	items, total, err := repo.QueryPlatformData(c.Request.Context(), query)
	if err != nil {
		// 返回详细错误信息以便调试
		response.Fail(c, 500, "query error: "+err.Error())
		return
	}

	response.OK(c, model.PlatformDataResponse{
		Data:  items,
		Total: total,
	})
}
