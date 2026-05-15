package main

import (
	"fmt"
	"log"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"opinion-analysis/config"
	"opinion-analysis/internal/api"
	"opinion-analysis/internal/api/handler"
	"opinion-analysis/internal/model"
)

func seedCrawlerSpiderConfigs(db *gorm.DB) {
	var n int64
	if err := db.Model(&model.CrawlerSpiderConfig{}).Count(&n).Error; err != nil {
		log.Fatalf("count crawler spider config: %v", err)
	}
	if n > 0 {
		return
	}
	seeds := []model.CrawlerSpiderConfig{
		{SpiderKey: "rss", DisplayName: "RSS", IntervalMinutes: 30, Enabled: 1},
		{SpiderKey: "zhihu", DisplayName: "知乎", IntervalMinutes: 60, Enabled: 1},
		{SpiderKey: "tieba", DisplayName: "贴吧", IntervalMinutes: 120, Enabled: 1},
	}
	if err := db.Create(&seeds).Error; err != nil {
		log.Fatalf("seed crawler spider config: %v", err)
	}
}

func main() {
	config.Load("config/config.yaml")

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	db, err := gorm.Open(mysql.Open(config.Cfg.Database.DSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(config.Cfg.Database.MaxOpenConn)
	sqlDB.SetMaxIdleConns(config.Cfg.Database.MaxIdleConn)

	// 自动迁移
	if err := db.AutoMigrate(
		&model.User{},
		&model.DataSource{},
		&model.Article{},
		&model.Topic{},
		&model.AlertRule{},
		&model.AlertRecord{},
		&model.CrawlerSpiderConfig{},
		&model.CrawlerRunLog{},
		&model.Report{},
	); err != nil {
		log.Fatalf("failed to migrate: %v", err)
	}

	seedCrawlerSpiderConfigs(db)

	if n, err := handler.RecoverStaleCrawlerRuns(db); err != nil {
		log.Fatalf("recover stale crawler runs: %v", err)
	} else if n > 0 {
		log.Printf("[crawler task] startup_recovered stale_run_count=%d (marked failed, see crawler_run_logs.message)", n)
	}

	r := api.NewRouter(db, logger)

	addr := fmt.Sprintf(":%s", config.Cfg.Server.Port)
	logger.Info("server starting", zap.String("addr", addr))
	if err := r.Run(addr); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
