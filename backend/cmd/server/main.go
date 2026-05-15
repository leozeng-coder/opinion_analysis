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
	// 迁移旧 key（rss/zhihu/tieba/search）到新 key（broad-topic/deep-sentiment）
	oldKeys := []string{"rss", "zhihu", "tieba", "search"}
	for _, old := range oldKeys {
		db.Where("spider_key = ?", old).Delete(&model.CrawlerSpiderConfig{})
	}

	desired := []struct {
		Key         string
		DisplayName string
		Interval    int
		Enabled     int8
	}{
		{"broad-topic", "新闻收集 + AI关键词提取", 60, 1},
		{"deep-sentiment", "深度情感爬取", 180, 0},
	}

	for _, d := range desired {
		var existing model.CrawlerSpiderConfig
		err := db.Where("spider_key = ?", d.Key).First(&existing).Error
		if err == nil {
			continue // 已存在，不覆盖用户配置
		}
		seed := model.CrawlerSpiderConfig{
			SpiderKey:       d.Key,
			DisplayName:     d.DisplayName,
			IntervalMinutes: d.Interval,
			Enabled:         d.Enabled,
		}
		if err := db.Create(&seed).Error; err != nil {
			log.Fatalf("seed crawler spider config %q: %v", d.Key, err)
		}
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
