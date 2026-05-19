package api

import (
	"context"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	adminhandler "opinion-analysis/src/api/handler/admin"
	userhandler "opinion-analysis/src/api/handler/user"
	"opinion-analysis/src/middleware"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/ragprocess"
	"opinion-analysis/src/service/tagger"
	"opinion-analysis/pkg/response"
)

func NewRouter(db *gorm.DB, logger *zap.Logger, taggerSvc *tagger.Service, ragProc *ragprocess.Manager) *gin.Engine {
	store := repository.NewStore(db)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger(logger))

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:3000", "http://localhost:5174"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	authH := userhandler.NewAuthHandler(store)
	articleH := userhandler.NewArticleHandler(store)
	topicH := userhandler.NewTopicHandler(store)
	alertH := userhandler.NewAlertHandler(store)
	crawlerH := userhandler.NewCrawlerHandler(store)
	aiChatH := userhandler.NewAIChatHandler(taggerSvc)
	chatSessionH := userhandler.NewChatSessionHandler(store, taggerSvc)

	adminUserH := adminhandler.NewUserHandler(store)
	adminSettingH := adminhandler.NewSettingHandler(store)
	adminSystemH := adminhandler.NewSystemHandler(db, store, taggerSvc)
	adminRagH := adminhandler.NewRAGHandler(store, ragProc)
	adminDSH := adminhandler.NewDataSourceHandler(store)
	adminAuditH := adminhandler.NewAuditHandler(store)

	apiGroup := r.Group("/api")
	{
		auth := apiGroup.Group("/auth")
		{
			auth.POST("/login", authH.Login)
			auth.POST("/register", authH.Register)
		}

		authorized := apiGroup.Group("", middleware.Auth())
		{
			authorized.GET("/auth/profile", authH.Profile)

			articles := authorized.Group("/articles")
			{
				articles.GET("", articleH.List)
				articles.GET("/stats", articleH.Stats)
				articles.GET("/platforms", articleH.Platforms)
				articles.GET("/tags", articleH.Tags)
				articles.GET("/:id", articleH.Detail)
			}

			topics := authorized.Group("/topics")
			{
				topics.GET("", topicH.List)
				topics.GET("/:id", topicH.Detail)
			}

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

			authorized.POST("/ai/chat", aiChatH.Chat)

			aiSessions := authorized.Group("/ai/sessions")
			{
				aiSessions.GET("", chatSessionH.ListSessions)
				aiSessions.POST("", chatSessionH.CreateSession)
				aiSessions.POST("/chat", chatSessionH.Chat)
				aiSessions.GET("/:id", chatSessionH.GetSession)
				aiSessions.DELETE("/:id", chatSessionH.DeleteSession)
				aiSessions.PATCH("/:id", chatSessionH.RenameSession)
			}

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
					count, err := store.Article.CountPendingTagging()
					if err != nil {
						response.ServerError(c)
						return
					}
					response.OK(c, gin.H{"pending": count})
				})
			}

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
				admin.GET("/system/settings/history", adminSystemH.ListSettingHistory)
				admin.DELETE("/system/settings/history/:id",
					middleware.Audit(db, "system_config", "delete_setting_history"),
					adminSystemH.DeleteSettingHistory)
				admin.POST("/system/settings/history/:id/reapply",
					middleware.Audit(db, "system_config", "reapply_setting_history"),
					adminSystemH.ReapplySettingHistory)

				admin.GET("/rag/status", adminRagH.Status)
				admin.GET("/rag/runs", adminRagH.ListRuns)
				admin.POST("/rag/sync",
					middleware.Audit(db, "rag", "sync"),
					adminRagH.TriggerSync)
				admin.GET("/rag/config", adminRagH.GetConfig)
				admin.PUT("/rag/config",
					middleware.Audit(db, "rag", "update_config"),
					adminRagH.UpdateConfig)
				admin.POST("/rag/milvus/rebuild",
					middleware.Audit(db, "rag", "rebuild_milvus"),
					adminRagH.RebuildMilvus)
				admin.POST("/rag/restart",
					middleware.Audit(db, "rag", "restart_service"),
					adminRagH.RestartService)
				admin.GET("/rag/articles", adminRagH.ListKBArticles)
				admin.DELETE("/rag/articles/:id/embedding",
					middleware.Audit(db, "rag", "delete_embedding"),
					adminRagH.DeleteArticleEmbedding)

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
