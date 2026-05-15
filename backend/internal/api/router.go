package api

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"opinion-analysis/internal/api/handler"
	"opinion-analysis/internal/middleware"
)

func NewRouter(db *gorm.DB, logger *zap.Logger) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger(logger))

	// CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// 健康检查
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	// 初始化 handlers
	authH := handler.NewAuthHandler(db)
	articleH := handler.NewArticleHandler(db)
	topicH := handler.NewTopicHandler(db)
	alertH := handler.NewAlertHandler(db)
	crawlerH := handler.NewCrawlerHandler(db)

	api := r.Group("/api")
	{
		// 认证（公开）
		auth := api.Group("/auth")
		{
			auth.POST("/login", authH.Login)
			auth.POST("/register", authH.Register)
		}

		// 需要登录
		authorized := api.Group("", middleware.Auth())
		{
			authorized.GET("/auth/profile", authH.Profile)

			// 舆情数据
			articles := authorized.Group("/articles")
			{
				articles.GET("", articleH.List)
				articles.GET("/stats", articleH.Stats)
				articles.GET("/platforms", articleH.Platforms)
				articles.GET("/:id", articleH.Detail)
			}

			// 热点话题
			topics := authorized.Group("/topics")
			{
				topics.GET("", topicH.List)
				topics.GET("/:id", topicH.Detail)
			}

			// 预警管理（需要分析师或管理员）
			alerts := authorized.Group("/alerts")
			{
				alerts.GET("/rules", alertH.ListRules)
				alerts.POST("/rules", middleware.RequireRole("admin", "analyst"), alertH.CreateRule)
				alerts.DELETE("/rules/:id", middleware.RequireRole("admin", "analyst"), alertH.DeleteRule)
				alerts.GET("/records", alertH.ListRecords)
			}

			// 爬虫（定时配置 + 立即执行）
			crawler := authorized.Group("/crawler")
			{
				crawler.GET("/spiders", crawlerH.ListSpiders)
				crawler.PUT("/spiders", crawlerH.PutSpiders)
				crawler.POST("/run", crawlerH.RunNow)
				crawler.GET("/runs", crawlerH.ListRuns)
				// 单独路径，避免与 /runs/:id 在部分环境下的匹配歧义
				crawler.GET("/progress/:id", crawlerH.GetRunProgress)
				crawler.GET("/runs/:id", crawlerH.GetRun)
			}
		}
	}

	return r
}
