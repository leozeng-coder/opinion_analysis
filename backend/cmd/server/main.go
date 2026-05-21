package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"opinion-analysis/config"
	"opinion-analysis/pkg/redisclient"
	"opinion-analysis/src/api"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
	"opinion-analysis/src/service/alertengine"
	"opinion-analysis/src/service/ragprocess"
	"opinion-analysis/src/service/tagger"
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
		{Key: "dashboard.hot_topic_threshold", Value: "2", Desc: "热点话题最低出现次数阈值（AI 标签在文章中出现 ≥ 该值视为热点）"},
		// 大模型配置默认值（实际值请通过管理后台 → 系统状态 → 大模型配置维护）
		{Key: "tagger.enabled", Value: "true", Desc: "AI 自动打标后台任务是否启用"},
		{Key: "tagger.llm_api_key", Value: "", Desc: "LLM API Key（敏感）"},
		{Key: "tagger.llm_base_url", Value: "https://api.deepseek.com", Desc: "LLM API Base URL（OpenAI 兼容）"},
		{Key: "tagger.llm_model", Value: "deepseek-chat", Desc: "LLM 模型名"},
		{Key: "tagger.interval_seconds", Value: "120", Desc: "轮询间隔（秒）"},
		{Key: "tagger.batch_size", Value: "20", Desc: "单次 LLM 请求条数"},
		{Key: "tagger.max_per_tick", Value: "200", Desc: "单次轮询最多处理条数"},
		{Key: "rag.sync_enabled", Value: "true", Desc: "RAG 向量同步定时任务是否启用"},
		{Key: "rag.embed_provider", Value: "local", Desc: "RAG 句向量来源：local=本地模型，api=OpenAI 兼容 API"},
		{Key: "rag.embed_model", Value: "paraphrase-multilingual-MiniLM-L12-v2", Desc: "RAG 句向量模型名（本地 HuggingFace id 或 API model）"},
		{Key: "rag.embed_api_base", Value: "", Desc: "RAG Embedding API Base URL（OpenAI 兼容）"},
		{Key: "rag.embed_api_key", Value: "", Desc: "RAG Embedding API Key（敏感）"},
		{Key: "rag.chunk_max_chars", Value: "420", Desc: "RAG 切块最大字符数"},
		{Key: "rag.chunk_overlap", Value: "72", Desc: "RAG 切块重叠字符数"},
		{Key: "rag.sync_interval_sec", Value: "120", Desc: "RAG 定时增量同步间隔（秒）"},
		{Key: "rag.sync_batch", Value: "100", Desc: "RAG 单次同步最多处理文章数"},
		{Key: "alert.on_crawl", Value: "true", Desc: "爬虫任务成功完成后是否自动触发告警评估"},
		{Key: "smtp.host", Value: "", Desc: "SMTP 服务器地址"},
		{Key: "smtp.port", Value: "465", Desc: "SMTP 端口（465=SSL, 587=STARTTLS）"},
		{Key: "smtp.username", Value: "", Desc: "SMTP 用户名"},
		{Key: "smtp.password", Value: "", Desc: "SMTP 密码或应用专用密码"},
		{Key: "smtp.from", Value: "", Desc: "发件人地址（可选，默认使用 username）"},
		{Key: "smtp.use_tls", Value: "false", Desc: "587 端口是否启用 STARTTLS（465 端口请保持 false）"},
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

// seedDefaultAdmin 仅在没有任何启用的 admin 账户时创建一个默认 admin / admin。
// 可通过环境变量 ADMIN_INIT_PASSWORD 覆盖初始密码（生产环境推荐）。
func seedDefaultAdmin(db *gorm.DB) {
	var cnt int64
	if err := db.Model(&model.User{}).Where("role = ? AND status = ?", "admin", 1).Count(&cnt).Error; err != nil {
		log.Printf("[admin-seed] count failed: %v", err)
		return
	}
	if cnt > 0 {
		return
	}
	initPwdEnv := strings.TrimSpace(os.Getenv("ADMIN_INIT_PASSWORD"))
	pwd := initPwdEnv
	if pwd == "" {
		pwd = "admin"
	}
	fromEnv := initPwdEnv != ""
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
	if fromEnv {
		log.Printf("[admin-seed] created admin (username admin), password from ADMIN_INIT_PASSWORD env")
	} else {
		log.Printf("[admin-seed] created default admin: username=admin password=admin — change immediately in production")
	}
}

func seedCrawlerSpiderConfigs(db *gorm.DB) {
	// 迁移旧 key（rss/zhihu/tieba/search）到新 key（broad-topic/deep-sentiment）
	oldKeys := []string{"rss", "zhihu", "tieba", "search"}
	for _, old := range oldKeys {
		_ = repository.NewStore(db, nil).Crawler.DeleteSpiderConfigByKey(old)
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
		crawlerRepo := repository.NewStore(db, nil).Crawler
		if _, err := crawlerRepo.FindSpiderByKey(d.Key); err == nil {
			continue
		}
		seed := model.CrawlerSpiderConfig{
			SpiderKey:       d.Key,
			DisplayName:     d.DisplayName,
			IntervalMinutes: d.Interval,
			Enabled:         d.Enabled,
		}
		if err := crawlerRepo.CreateSpiderConfig(&seed); err != nil {
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
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		log.Fatalf("failed to migrate: %v", err)
	}

	seedCrawlerSpiderConfigs(db)
	seedSystemSettings(db)
	seedDefaultAdmin(db)

	runCrawlerSQL(db)

	rdb := redisclient.New(config.Cfg.Redis)
	if rdb != nil {
		defer rdb.Close()
	}

	if n, err := repository.NewStore(db, repository.NewDigestRepository(rdb)).Crawler.RecoverStaleRuns(); err != nil {
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

	ragProc := ragprocess.NewManager()
	alertEngine := alertengine.New(repository.NewStore(db, nil), taggerSvc)
	if config.Cfg.RAG.Managed && config.Cfg.RAG.AutoStart {
		go func() {
			startCtx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()
			if err := ragProc.EnsureStarted(startCtx); err != nil {
				log.Printf("[rag-process] auto_start: %v", err)
			}
		}()
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := ragProc.Stop(stopCtx); err != nil {
			log.Printf("[rag-process] stop: %v", err)
		}
	}()

	r := api.NewRouter(db, rdb, logger, taggerSvc, ragProc, alertEngine)

	addr := fmt.Sprintf(":%s", config.Cfg.Server.Port)
	logger.Info("server starting", zap.String("addr", addr))
	if err := r.Run(addr); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
