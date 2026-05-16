package api

import (
	"context"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"opinion-analysis/config"
	"opinion-analysis/internal/api/handler"
	"opinion-analysis/internal/middleware"
	"opinion-analysis/internal/service/tagger"
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
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	// 初始化 handlers
	authH := handler.NewAuthHandler(db)
	articleH := handler.NewArticleHandler(db)
	topicH := handler.NewTopicHandler(db)
	alertH := handler.NewAlertHandler(db)
	crawlerH := handler.NewCrawlerHandler(db)
	taggerSvc := tagger.New(db, config.Cfg.Tagger)

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
				alerts.POST("/rules", middleware.RequireRole("admin", "analyst"), alertH.CreateRule)
				alerts.DELETE("/rules/:id", middleware.RequireRole("admin", "analyst"), alertH.DeleteRule)
				alerts.GET("/records", alertH.ListRecords)
			}

			// 爬虫
			crawler := authorized.Group("/crawler")
			{
				crawler.GET("/spiders", crawlerH.ListSpiders)
				crawler.PUT("/spiders", crawlerH.PutSpiders)
				crawler.POST("/run", crawlerH.RunNow)
				crawler.GET("/runs", crawlerH.ListRuns)
				crawler.GET("/progress/:id", crawlerH.GetRunProgress)
				crawler.GET("/runs/:id", crawlerH.GetRun)
			}

			// AI 打标后台任务管理（管理员用）
			taggerGroup := authorized.Group("/tagger")
			{
				// 手动触发一轮（用于调试，不阻塞）
				taggerGroup.POST("/run", middleware.RequireRole("admin", "analyst"), func(c *gin.Context) {
					go func() {
						n, err := taggerSvc.RunOnce(context.Background())
						if err != nil {
							logger.Warn("manual tagger run failed", zap.Error(err))
						} else {
							logger.Info("manual tagger run done", zap.Int("tagged", n))
						}
					}()
					c.JSON(http.StatusOK, gin.H{"message": "tagger started in background"})
				})
				// 查询当前待打标条数
				taggerGroup.GET("/pending", middleware.RequireRole("admin", "analyst"), func(c *gin.Context) {
					var count int64
					db.Table("articles").Where("ai_tags IS NULL AND deleted_at IS NULL").Count(&count)
					c.JSON(http.StatusOK, gin.H{"pending": count})
				})
			}
		}
	}

	return r
}
