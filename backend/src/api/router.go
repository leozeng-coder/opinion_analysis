package api

import (
	"context"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"opinion-analysis/config"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/api/handler"
	adminhandler "opinion-analysis/src/api/handler/admin"
	userhandler "opinion-analysis/src/api/handler/user"
	"opinion-analysis/src/middleware"
	milvussvc "opinion-analysis/src/service/milvus"
	"opinion-analysis/src/service/rag"
	"opinion-analysis/src/service/report"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/alertengine"
	"opinion-analysis/src/service/ragprocess"
	"opinion-analysis/src/service/tagger"
	"opinion-analysis/src/service/workflow"
	"strings"
)

func NewRouter(db *gorm.DB, rdb *redis.Client, logger *zap.Logger, taggerSvc *tagger.Service, ragProc *ragprocess.Manager, alertEngine *alertengine.Engine) *gin.Engine {
	store := repository.NewStore(db, repository.NewDigestRepository(rdb))

	// report service（分析报告，存 Redis）
	reportSvc := report.NewService(db, rdb, taggerSvc)

	// Milvus + embedding 服务初始化
	var milvusService *milvussvc.Service
	var embedClient *milvussvc.EmbedderClient
	var syncerSvc *milvussvc.Syncer
	var ragClient *rag.Client
	if config.Cfg != nil && config.Cfg.RAG.Enabled {
		milvusURI := strings.TrimSpace(config.Cfg.RAG.MilvusURI)
		if milvusURI == "" {
			milvusURI = "http://localhost:19530"
		}
		milvusCollection := strings.TrimSpace(config.Cfg.RAG.MilvusCollection)
		milvusService = milvussvc.NewService(milvusURI, milvusCollection)

		embedURL := strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL)
		embedClient = milvussvc.NewEmbedderClient(embedURL)

		syncerSvc = milvussvc.NewSyncer(db, milvusService, embedClient, store.RAG)
		// 从 DB 加载当前配置，覆盖写死的默认值
		if ragCfg, err := store.RAG.GetRagConfig(); err == nil {
			syncerSvc.UpdateConfig(milvussvc.SyncConfig{
				Enabled:         ragCfg.SyncEnabled,
				ChunkMaxChars:   ragCfg.ChunkMaxChars,
				ChunkOverlap:    ragCfg.ChunkOverlap,
				SyncIntervalSec: ragCfg.SyncIntervalSec,
				SyncBatch:       ragCfg.SyncBatch,
			})
		}
		syncerSvc.Start(context.Background())

		ragClient = rag.NewClient(embedURL, embedClient, milvusService, db)
		logger.Info("RAG enabled", zap.String("milvus", milvusURI), zap.String("embed", embedURL))
	}

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
	alertH := userhandler.NewAlertHandler(store, alertEngine)
	// crawlerH := userhandler.NewCrawlerHandler(store, alertEngine) // 已废弃，改用 MediaCrawler
	aiChatH := userhandler.NewAIChatHandler(taggerSvc, ragClient)
	chatSessionH := userhandler.NewChatSessionHandler(store, taggerSvc, ragClient)
	dashboardH := userhandler.NewDashboardHandler(store)
	reportH := userhandler.NewReportHandler(reportSvc, db)

	// MediaCrawler 代理处理器
	mediaCrawlerURL := config.Cfg.Crawler.ApiURL
	if mediaCrawlerURL == "" {
		mediaCrawlerURL = "http://127.0.0.1:8085"
	}
	mediaCrawlerSecret := config.Cfg.Crawler.ProxySecretKey
	if mediaCrawlerSecret == "" {
		mediaCrawlerSecret = "your-secret-key-change-in-production"
	}
	mediaCrawlerProxy := userhandler.NewMediaCrawlerProxyHandler(mediaCrawlerURL, mediaCrawlerSecret)

	adminUserH := adminhandler.NewUserHandler(store)
	adminSettingH := adminhandler.NewSettingHandler(store)
	adminSystemH := adminhandler.NewSystemHandler(db, store, taggerSvc, rdb)
	adminRagH := adminhandler.NewRAGHandler(store, ragProc, milvusService, embedClient, syncerSvc)
	adminDSH := adminhandler.NewDataSourceHandler(store)
	adminAuditH := adminhandler.NewAuditHandler(store)
	adminAISummaryH := adminhandler.NewAISummaryHandler(db, taggerSvc, store.Digest)

	// 工作流引擎和调度器
	workflowEngine := workflow.NewEngine(db, store, logger, taggerSvc, ragProc, syncerSvc, alertEngine, reportSvc)
	workflowScheduler := workflow.NewScheduler(store, workflowEngine, logger)
	workflowScheduler.Start()
	workflowH := handler.NewWorkflowHandler(store, workflowEngine, milvusService)

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

			authorized.GET("/dashboard", dashboardH.Overview)

			platform := authorized.Group("/platform")
			{
				platform.GET("/data", func(c *gin.Context) {
					c.Set("db", db)
					userhandler.GetPlatformData(c)
				})
				platform.GET("/comments", func(c *gin.Context) {
					c.Set("db", db)
					userhandler.GetPlatformComments(c)
				})

				// 平台数据同步接口
				platform.POST("/sync", func(c *gin.Context) {
					c.Set("db", db)
					c.Set("redis", rdb)
					c.Set("tagger", taggerSvc)
					userhandler.SyncPlatformData(c)
				})
				platform.POST("/sync/all", func(c *gin.Context) {
					c.Set("db", db)
					c.Set("redis", rdb)
					c.Set("tagger", taggerSvc)
					userhandler.SyncAllPlatforms(c)
				})
				platform.GET("/sync/progress", func(c *gin.Context) {
					c.Set("db", db)
					userhandler.GetSyncProgress(c)
				})
				platform.GET("/sync/status", func(c *gin.Context) {
					c.Set("db", db)
					userhandler.GetSyncStatus(c)
				})
				platform.GET("/list", func(c *gin.Context) {
					c.Set("db", db)
					userhandler.GetPlatformList(c)
				})
			}

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
				alerts.PUT("/rules/:id",
					middleware.RequireRole("admin", "analyst"),
					middleware.Audit(db, "alert_rule", "update"),
					alertH.UpdateRule)
				alerts.DELETE("/rules/:id",
					middleware.RequireRole("admin", "analyst"),
					middleware.Audit(db, "alert_rule", "delete"),
					alertH.DeleteRule)
				alerts.GET("/records", alertH.ListRecords)
				alerts.GET("/records/:id", alertH.GetRecordDetail)
				alerts.PATCH("/records/:id/read", alertH.MarkAsRead)
				alerts.POST("/evaluate",
					middleware.RequireRole("admin", "analyst"),
					middleware.Audit(db, "alert", "evaluate"),
					alertH.Evaluate)
			}
		}

		// MediaCrawler 爬虫路由（不需要 JWT 认证，因为 FastAPI 层已有签名认证）
		crawler := apiGroup.Group("/crawler")
		{
			// 所有 crawler 请求代理到 FastAPI (MediaCrawler)
			crawler.Any("/*proxyPath", mediaCrawlerProxy.ProxyRequest)
		}

		authorized2 := apiGroup.Group("", middleware.Auth())
		{
			authorized2.POST("/ai/chat", aiChatH.Chat)

			// 测试 SSE 流式输出
			authorized2.GET("/ai/test-stream", userhandler.TestStreamHandler)

			aiSessions := authorized2.Group("/ai/sessions")
			{
				aiSessions.GET("", chatSessionH.ListSessions)
				aiSessions.POST("", chatSessionH.CreateSession)
				aiSessions.POST("/chat", chatSessionH.Chat)
				aiSessions.POST("/chat/deep", chatSessionH.DeepChat)
				aiSessions.GET("/capabilities", chatSessionH.Capabilities)
				aiSessions.GET("/:id", chatSessionH.GetSession)
				aiSessions.DELETE("/:id", chatSessionH.DeleteSession)
				aiSessions.PATCH("/:id", chatSessionH.RenameSession)
				aiSessions.POST("/:id/regenerate", chatSessionH.RegenerateLastMessage)
			}

			taggerGroup := authorized2.Group("/tagger")
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

			admin := authorized2.Group("/admin", middleware.RequireAdmin())
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
				admin.GET("/system/smtp", adminSystemH.GetSmtp)
				admin.PUT("/system/smtp",
					middleware.Audit(db, "system_config", "update_smtp"),
					adminSystemH.UpdateSmtp)
				admin.POST("/system/smtp/test",
					middleware.Audit(db, "system_config", "test_smtp"),
					adminSystemH.TestSmtp)
				admin.GET("/system/crawler", adminSystemH.GetCrawlerConfig)
				admin.GET("/system/crawler/limits", adminSystemH.GetCrawlerLimits)
				admin.PUT("/system/crawler",
					middleware.Audit(db, "system_config", "update_crawler"),
					adminSystemH.UpdateCrawlerConfig)

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
				admin.GET("/rag/articles/:id", adminRagH.GetKBArticleDetail)
				admin.PUT("/rag/chunks",
					middleware.Audit(db, "rag", "update_chunk"),
					adminRagH.UpdateChunk)
				admin.DELETE("/rag/chunks",
					middleware.Audit(db, "rag", "delete_chunk"),
					adminRagH.DeleteChunk)
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

				admin.GET("/ai/digest", adminAISummaryH.Get)
				admin.POST("/ai/digest/regenerate",
					middleware.Audit(db, "ai_summary", "generate"),
					adminAISummaryH.Regenerate)
			}

			// 分析报告下载
			reports := authorized2.Group("/reports")
			{
				reports.GET("/:reportId", reportH.Info)
				reports.GET("/:reportId/download", reportH.Download)
				reports.POST("/regenerate", reportH.Regenerate)
			}

			// 工作流路由（需要认证）
			workflows := authorized2.Group("/workflows")
			{
				workflows.GET("", workflowH.List)
				workflows.GET("/topics", workflowH.ListTopics)
				workflows.POST("", middleware.RequireRole("admin", "analyst"), middleware.Audit(db, "workflow", "create"), workflowH.Create)
				workflows.GET("/:id", workflowH.Detail)
				workflows.PUT("/:id", middleware.RequireRole("admin", "analyst"), middleware.Audit(db, "workflow", "update"), workflowH.Update)
				workflows.DELETE("/:id", middleware.RequireRole("admin", "analyst"), middleware.Audit(db, "workflow", "delete"), workflowH.Delete)
				workflows.POST("/:id/execute", middleware.RequireRole("admin", "analyst"), middleware.Audit(db, "workflow", "execute"), workflowH.Execute)
				workflows.POST("/:id/execute-from-node", middleware.RequireRole("admin", "analyst"), middleware.Audit(db, "workflow", "execute_from_node"), workflowH.ExecuteFromNode)
				workflows.GET("/:id/executions", workflowH.Executions)
				workflows.GET("/executions/:execId/logs", workflowH.ExecutionLogs)
				workflows.POST("/executions/:execId/cancel", middleware.RequireRole("admin", "analyst"), middleware.Audit(db, "workflow", "cancel"), workflowH.CancelExecution)
			}
		}
	}

	return r
}
