package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"opinion-analysis/config"
	"opinion-analysis/internal/api"
	"opinion-analysis/internal/api/handler"
	"opinion-analysis/internal/model"
	"opinion-analysis/internal/service/tagger"
)

// runCrawlerSQL 执行 crawler/schema/crawler_tables.sql，幂等：已存在/重复键等错误静默跳过。
func runCrawlerSQL(db *gorm.DB) {
	wd, err := os.Getwd()
	if err != nil {
		log.Printf("[crawler-sql] getwd: %v", err)
		return
	}
	sqlPath := filepath.Join(wd, config.Cfg.Crawler.Root, "schema", "crawler_tables.sql")
	data, err := os.ReadFile(sqlPath)
	if err != nil {
		log.Printf("[crawler-sql] read %s: %v (skipped)", sqlPath, err)
		return
	}

	sqlDB, _ := db.DB()
	stmts := strings.Split(string(data), ";")
	ok, skipped, failed := 0, 0, 0
	for _, raw := range stmts {
		stmt := strings.TrimSpace(raw)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}
		if _, err := sqlDB.Exec(stmt); err != nil {
			msg := err.Error()
			// 幂等：表已存在、列已存在、索引已存在、外键缺失等均跳过
			if strings.Contains(msg, "already exists") ||
				strings.Contains(msg, "Duplicate") ||
				strings.Contains(msg, "1060") || // duplicate column
				strings.Contains(msg, "1061") || // duplicate key name
				strings.Contains(msg, "1050") || // table already exists
				strings.Contains(msg, "6125") || // FK missing unique key
				strings.Contains(msg, "1146") { // table doesn't exist (ALTER on missing table)
				skipped++
			} else {
				log.Printf("[crawler-sql] warn: %v", err)
				failed++
			}
		} else {
			ok++
		}
	}
	log.Printf("[crawler-sql] done: ok=%d skipped=%d failed=%d", ok, skipped, failed)
}

func seedSystemSettings(db *gorm.DB) {
	desired := []model.SystemSetting{
		{Key: "registration_enabled", Value: "true", Desc: "是否允许开放注册（关闭后 /api/auth/register 拒绝）"},
		// 大模型配置默认值（实际值请通过管理后台 → 系统状态 → 大模型配置维护）
		{Key: "tagger.enabled", Value: "true", Desc: "AI 自动打标后台任务是否启用"},
		{Key: "tagger.llm_api_key", Value: "", Desc: "LLM API Key（敏感）"},
		{Key: "tagger.llm_base_url", Value: "https://api.deepseek.com", Desc: "LLM API Base URL（OpenAI 兼容）"},
		{Key: "tagger.llm_model", Value: "deepseek-chat", Desc: "LLM 模型名"},
		{Key: "tagger.interval_seconds", Value: "120", Desc: "轮询间隔（秒）"},
		{Key: "tagger.batch_size", Value: "20", Desc: "单次 LLM 请求条数"},
		{Key: "tagger.max_per_tick", Value: "200", Desc: "单次轮询最多处理条数"},
	}
	for _, s := range desired {
		var existing model.SystemSetting
		if err := db.Where("`key` = ?", s.Key).First(&existing).Error; err == nil {
			continue // 已存在，保留用户配置
		}
		if err := db.Create(&s).Error; err != nil {
			log.Printf("[seed-settings] %s: %v", s.Key, err)
		}
	}
}

// seedDefaultAdmin 仅在没有任何启用的 admin 账户时创建一个。
// 密码优先取 ADMIN_INIT_PASSWORD，否则随机 16 位明文写到启动日志。
func seedDefaultAdmin(db *gorm.DB) {
	var cnt int64
	if err := db.Model(&model.User{}).Where("role = ? AND status = ?", "admin", 1).Count(&cnt).Error; err != nil {
		log.Printf("[admin-seed] count failed: %v", err)
		return
	}
	if cnt > 0 {
		return
	}
	pwd := strings.TrimSpace(os.Getenv("ADMIN_INIT_PASSWORD"))
	generated := false
	if pwd == "" {
		pwd = randomPassword(16)
		generated = true
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[admin-seed] bcrypt: %v", err)
		return
	}
	user := model.User{
		Username: "admin",
		Password: string(hash),
		Email:    "admin@local",
		Nickname: "Administrator",
		Role:     "admin",
		Status:   1,
	}
	// 用户名/邮箱可能已存在（之前 viewer 占位），先尝试创建
	if err := db.Where("username = ?", user.Username).FirstOrCreate(&user, user).Error; err != nil {
		log.Printf("[admin-seed] create: %v", err)
		return
	}
	// 已存在但角色不是 admin —— 提升它
	if user.Role != "admin" || user.Status != 1 {
		db.Model(&user).Updates(map[string]interface{}{"role": "admin", "status": 1, "password": string(hash)})
	}
	if generated {
		log.Printf("[admin-seed] initial admin password: %s — change immediately", pwd)
	} else {
		log.Printf("[admin-seed] admin user created from ADMIN_INIT_PASSWORD env")
	}
}

func randomPassword(n int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	b := make([]byte, n)
	max := big.NewInt(int64(len(alphabet)))
	for i := range b {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			b[i] = alphabet[0]
			continue
		}
		b[i] = alphabet[idx.Int64()]
	}
	return string(b)
}

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
		&model.SystemSetting{},
		&model.AuditLog{},
	); err != nil {
		log.Fatalf("failed to migrate: %v", err)
	}

	seedCrawlerSpiderConfigs(db)
	seedSystemSettings(db)
	seedDefaultAdmin(db)

	runCrawlerSQL(db)

	if n, err := handler.RecoverStaleCrawlerRuns(db); err != nil {
		log.Fatalf("recover stale crawler runs: %v", err)
	} else if n > 0 {
		log.Printf("[crawler task] startup_recovered stale_run_count=%d (marked failed, see crawler_run_logs.message)", n)
	}

	// 迁移旧 DB key（deepseek_* → llm_*），幂等
	tagger.MigrateOldKeys(db)

	// 启动 AI 自动打标后台任务：先用 system_settings 中的覆盖值合并 yaml 配置，再实例化服务
	effectiveTaggerCfg := tagger.LoadConfig(db, config.Cfg.Tagger)
	config.Cfg.Tagger = effectiveTaggerCfg
	taggerSvc := tagger.New(db, effectiveTaggerCfg)
	ctx := context.Background()
	go taggerSvc.Start(ctx)

	r := api.NewRouter(db, logger, taggerSvc)

	addr := fmt.Sprintf(":%s", config.Cfg.Server.Port)
	logger.Info("server starting", zap.String("addr", addr))
	if err := r.Run(addr); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
