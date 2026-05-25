package user

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"opinion-analysis/pkg/response"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service"
	"opinion-analysis/src/service/digest"
	"opinion-analysis/src/service/tagger"
)

// SyncPlatformDataRequest 同步平台数据请求（支持多平台）
type SyncPlatformDataRequest struct {
	Platforms []string `json:"platforms" binding:"required,min=1"` // 平台列表：["xhs", "dy", "bili"]
}

// SyncPlatformData 手动触发平台数据同步（支持多平台）
// @Summary 同步平台数据到articles表
// @Tags Platform
// @Accept json
// @Produce json
// @Param request body SyncPlatformDataRequest true "同步请求"
// @Success 200 {object} map[string]service.SyncResult
// @Router /api/platform/sync [post]
func SyncPlatformData(c *gin.Context) {
	var req SyncPlatformDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}

	db, exists := c.Get("db")
	if !exists {
		response.Fail(c, 500, "database connection not found")
		return
	}

	syncService := service.NewPlatformSyncService(db.(*gorm.DB))

	// 设置摘要生成器（如果Redis可用）
	if rdb, ok := c.Get("redis"); ok && rdb != nil {
		if taggerSvc, ok := c.Get("tagger"); ok && taggerSvc != nil {
			digestRepo := repository.NewDigestRepository(rdb.(*redis.Client))
			digestGen := digest.NewGenerator(db.(*gorm.DB), digestRepo, taggerSvc.(*tagger.Service))
			syncService.SetDigestGenerator(digestGen)
		}
	}

	// 执行多平台同步
	results, err := syncService.SyncPlatforms(c.Request.Context(), req.Platforms)
	if err != nil {
		response.Fail(c, 500, "同步失败: "+err.Error())
		return
	}

	response.OK(c, results)
}

// SyncAllPlatforms 同步所有平台数据
// @Summary 同步所有平台数据
// @Tags Platform
// @Accept json
// @Produce json
// @Success 200 {object} map[string]service.SyncResult
// @Router /api/platform/sync/all [post]
func SyncAllPlatforms(c *gin.Context) {
	db, exists := c.Get("db")
	if !exists {
		response.Fail(c, 500, "database connection not found")
		return
	}

	syncService := service.NewPlatformSyncService(db.(*gorm.DB))

	// 设置摘要生成器（如果Redis可用）
	if rdb, ok := c.Get("redis"); ok && rdb != nil {
		if taggerSvc, ok := c.Get("tagger"); ok && taggerSvc != nil {
			digestRepo := repository.NewDigestRepository(rdb.(*redis.Client))
			digestGen := digest.NewGenerator(db.(*gorm.DB), digestRepo, taggerSvc.(*tagger.Service))
			syncService.SetDigestGenerator(digestGen)
		}
	}

	results, err := syncService.SyncAllPlatforms(c.Request.Context())
	if err != nil {
		response.Fail(c, 500, "同步失败: "+err.Error())
		return
	}

	response.OK(c, results)
}

// GetSyncProgress 获取同步进度
// @Summary 获取同步进度
// @Tags Platform
// @Accept json
// @Produce json
// @Param platform query string false "平台类型（不传则返回所有平台）"
// @Success 200 {object} interface{}
// @Router /api/platform/sync/progress [get]
func GetSyncProgress(c *gin.Context) {
	platform := c.Query("platform")

	db, exists := c.Get("db")
	if !exists {
		response.Fail(c, 500, "database connection not found")
		return
	}

	syncService := service.NewPlatformSyncService(db.(*gorm.DB))

	if platform != "" {
		// 获取单个平台进度
		progress := syncService.GetSyncProgress(platform)
		if progress == nil {
			response.OK(c, gin.H{
				"platform": platform,
				"status":   "idle",
			})
			return
		}
		response.OK(c, progress.GetSnapshot())
	} else {
		// 获取所有平台进度
		progresses := syncService.GetAllSyncProgress()
		response.OK(c, progresses)
	}
}

// GetPlatformList 获取所有支持的平台列表
// @Summary 获取平台列表
// @Tags Platform
// @Accept json
// @Produce json
// @Success 200 {object} []service.PlatformInfo
// @Router /api/platform/list [get]
func GetPlatformList(c *gin.Context) {
	db, exists := c.Get("db")
	if !exists {
		response.Fail(c, 500, "database connection not found")
		return
	}

	syncService := service.NewPlatformSyncService(db.(*gorm.DB))
	platforms := syncService.GetPlatformList()

	response.OK(c, platforms)
}

// GetSyncStatus 获取同步状态（简化版）
// @Summary 获取所有平台的同步状态
// @Tags Platform
// @Accept json
// @Produce json
// @Success 200 {object} []service.PlatformInfo
// @Router /api/platform/sync/status [get]
func GetSyncStatus(c *gin.Context) {
	db, exists := c.Get("db")
	if !exists {
		response.Fail(c, 500, "database connection not found")
		return
	}

	syncService := service.NewPlatformSyncService(db.(*gorm.DB))
	platforms := syncService.GetPlatformList()

	response.OK(c, platforms)
}
