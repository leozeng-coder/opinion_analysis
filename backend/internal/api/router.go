package api

import (
	"context"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"opinion-analysis/internal/api/handler"
	"opinion-analysis/internal/middleware"
	"opinion-analysis/internal/service/tagger"
	"opinion-analysis/pkg/response"
)

func NewRouter(db *gorm.DB, logger *zap.Logger, taggerSvc *tagger.Service) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger(logger))

	// CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:3000", "http://localhost:5174"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// 健康检查
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	// 初始化 handlers
	authH := handler.NewAuthHandler(db)
	articleH := handler.NewArticleHandler(db)
	topicH := handler.NewTopicHandler(db)
	alertH := handler.NewAlertHandler(db)
	crawlerH := handler.NewCrawlerHandler(db)
	aiChatH := handler.NewAIChatHandler(db, taggerSvc)

	// Admin handlers
	adminUserH := handler.NewAdminUserHandler(db)
	adminSettingH := handler.NewAdminSettingHandler(db)
	adminSystemH := handler.NewAdminSystemHandler(db, taggerSvc)
	adminRagH := handler.NewAdminRagHandler(db)
	adminDSH := handler.NewAdminDataSourceHandler(db)
	adminAuditH := handler.NewAdminAuditHandler(db)

	apiGroup := r.Group("/api")
	{
		// 认证（公开）
		auth := apiGroup.Group("/auth")
		{
			auth.POST("/login", authH.Login)
			auth.POST("/register", authH.Register)
		}

		// 需要登录
		authorized := apiGroup.Group("", middleware.Auth())
		{
			authorized.GET("/auth/profile", authH.Profile)

			// 舆情数据
			articles := authorized.Group("/articles")
			{
				articles.GET("", articleH.List)
				articles.GET("/stats", articleH.Stats)
				articles.GET("/platforms", articleH.Platforms)
				articles.GET("/tags", articleH.Tags)
				articles.GET("/:id", articleH.Detail)
			}

			// 热点话题
			topics := authorized.Group("/topics")
			{
				topics.GET("", topicH.List)
				topics.GET("/:id", topicH.Detail)
			}

			// 预警管理
			alerts := authorized.Group("/alerts")
			{
				alerts.GET("/rules", alertH.ListRules)
				alerts.POST("/rules",
					middleware.RequireRole("admin", "analyst"),
					middleware.Audit(db, "alert_rule", "create"),
					alertH.CreateRule)
				alerts.DELETE("/rules/:id",
					middleware.RequireRole("admin", "analyst"),
					middleware.Audit(db, "alert_rule", "delete"),
					alertH.DeleteRule)
				alerts.GET("/records", alertH.ListRecords)
			}

			// 爬虫
			crawler := authorized.Group("/crawler")
			{
				crawler.GET("/spiders", crawlerH.ListSpiders)
				crawler.PUT("/spiders",
					middleware.Audit(db, "crawler", "update_spiders"),
					crawlerH.PutSpiders)
				crawler.POST("/run",
					middleware.Audit(db, "crawler", "run"),
					crawlerH.RunNow)
				crawler.GET("/runs", crawlerH.ListRuns)
				crawler.GET("/progress/:id", crawlerH.GetRunProgress)
				crawler.GET("/runs/:id", crawlerH.GetRun)
			}

			// 智能助手对话（与 tagger 共用大模型配置）
			authorized.POST("/ai/chat", aiChatH.Chat)

			// AI 打标后台任务管理（管理员用）
			taggerGroup := authorized.Group("/tagger")
			{
				taggerGroup.POST("/run",
					middleware.RequireRole("admin", "analyst"),
					middleware.Audit(db, "tagger", "run"),
					func(c *gin.Context) {
						go func() {
							n, err := taggerSvc.RunOnce(context.Background())
							if err != nil {
								logger.Warn("manual tagger run failed", zap.Error(err))
							} else {
								logger.Info("manual tagger run done", zap.Int("tagged", n))
							}
						}()
						response.OK(c, gin.H{"message": "tagger started in background"})
					})
				taggerGroup.GET("/pending", middleware.RequireRole("admin", "analyst"), func(c *gin.Context) {
					var count int64
					db.Table("articles").Where("ai_tags IS NULL AND deleted_at IS NULL").Count(&count)
					response.OK(c, gin.H{"pending": count})
				})
			}

			// 管理后台（仅 admin 角色）
			admin := authorized.Group("/admin", middleware.RequireAdmin())
			{
				admin.GET("/users", adminUserH.List)
				admin.PUT("/users/:id",
					middleware.Audit(db, "user", "update"),
					adminUserH.Update)
				admin.POST("/users/:id/reset-password",
					middleware.Audit(db, "user", "reset_password"),
					adminUserH.ResetPassword)
				admin.DELETE("/users/:id",
					middleware.Audit(db, "user", "delete"),
					adminUserH.Delete)

				admin.GET("/settings", adminSettingH.List)
				admin.PUT("/settings/:key",
					middleware.Audit(db, "system_setting", "update"),
					adminSettingH.Update)

				admin.GET("/system/config", adminSystemH.Config)
				admin.PUT("/system/tagger",
					middleware.Audit(db, "system_config", "update_tagger"),
					adminSystemH.UpdateTagger)
				admin.GET("/system/health", adminSystemH.Health)

				admin.GET("/rag/status", adminRagH.Status)
				admin.GET("/rag/runs", adminRagH.ListRuns)
				admin.POST("/rag/sync",
					middleware.Audit(db, "rag", "sync"),
					adminRagH.TriggerSync)

				admin.GET("/data-sources", adminDSH.List)
				admin.POST("/data-sources",
					middleware.Audit(db, "data_source", "create"),
					adminDSH.Create)
				admin.PUT("/data-sources/:id",
					middleware.Audit(db, "data_source", "update"),
					adminDSH.Update)
				admin.DELETE("/data-sources/:id",
					middleware.Audit(db, "data_source", "delete"),
					adminDSH.Delete)

				admin.GET("/audit-logs", adminAuditH.List)
			}
		}
	}

	return r
}
