package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
	"opinion-analysis/src/service/digest"
)

// PlatformSyncService 平台数据同步服务（重构版）
type PlatformSyncService struct {
	db              *gorm.DB
	factory         *SyncerFactory
	progressTracker *ProgressTracker
	digestGen       *digest.Generator
}

func NewPlatformSyncService(db *gorm.DB) *PlatformSyncService {
	return &PlatformSyncService{
		db:              db,
		factory:         NewSyncerFactory(db),
		progressTracker: NewProgressTracker(),
		digestGen:       nil, // 延迟初始化
	}
}

// SetDigestGenerator 设置摘要生成器（可选）
func (s *PlatformSyncService) SetDigestGenerator(gen *digest.Generator) {
	s.digestGen = gen
}

// SyncConfig 同步配置
type SyncConfig struct {
	Platform          string    `json:"platform"`
	Enabled           bool      `json:"enabled"`
	SyncMode          string    `json:"syncMode"` // incremental/full
	IntervalMinutes   int       `json:"intervalMinutes"`
	LastSyncTime      time.Time `json:"lastSyncTime"`
	SourceID          uint      `json:"sourceId"`
	EnableSentiment   bool      `json:"enableSentiment"`
	SentimentEndpoint string    `json:"sentimentEndpoint"`

	// MinSourceID > 0 时表示按源表主键 PK 做增量同步：
	// 只处理源表中 id > MinSourceID 的行，忽略 LastSyncTime。
	// 工作流场景下由 crawler_run 节点提前记录每个源表的 max(id) 作为 baseline。
	MinSourceID uint `json:"minSourceId"`

	// IncludeSourceIDs 非空时，仅同步源表中 id 在该集合内的行（用于「数据过滤」节点筛选后只持久化保留项）。
	IncludeSourceIDs []uint `json:"includeSourceIds,omitempty"`

	// Topic 用于向量数据库分区，从工作流的爬虫节点 config.topics 中提取
	Topic string `json:"topic,omitempty"`
}

// SourceFilterRow 源表行用于「数据过滤」的最小字段集（标题 + 正文）。
type SourceFilterRow struct {
	SourceID uint   `json:"sourceId"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

// SyncResult 同步结果
type SyncResult struct {
	Platform     string    `json:"platform"`
	TotalCount   int       `json:"totalCount"`
	NewCount     int       `json:"newCount"`
	SkippedCount int       `json:"skippedCount"`
	ErrorCount   int       `json:"errorCount"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	Duration     string    `json:"duration"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
}

// SyncPlatforms 同步多个平台（支持并发）
func (s *PlatformSyncService) SyncPlatforms(ctx context.Context, platforms []string) (map[string]*SyncResult, error) {
	if len(platforms) == 0 {
		return nil, fmt.Errorf("no platforms specified")
	}

	results := make(map[string]*SyncResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, platform := range platforms {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			result, err := s.SyncSinglePlatform(ctx, p)

			mu.Lock()
			if err != nil {
				results[p] = &SyncResult{
					Platform:     p,
					Status:       "failed",
					ErrorMessage: err.Error(),
					StartTime:    time.Now(),
					EndTime:      time.Now(),
				}
			} else {
				results[p] = result
			}
			mu.Unlock()
		}(platform)
	}

	wg.Wait()

	// 同步完成后，触发摘要生成
	s.triggerDigestGeneration(ctx, results)

	return results, nil
}

// SyncPlatformSince 从指定时间点起增量同步单个平台（按源帖子发帖时间过滤，旧逻辑保留）
func (s *PlatformSyncService) SyncPlatformSince(ctx context.Context, platform string, since time.Time, enableSentiment bool) (*SyncResult, error) {
	return s.syncPlatformSince(ctx, platform, since, "", enableSentiment)
}

// SyncPlatformSinceWithTopic 从指定时间点起增量同步，支持设置 topic 字段。
func (s *PlatformSyncService) SyncPlatformSinceWithTopic(ctx context.Context, platform string, since time.Time, topic string, enableSentiment bool) (*SyncResult, error) {
	return s.syncPlatformSince(ctx, platform, since, topic, enableSentiment)
}

func (s *PlatformSyncService) syncPlatformSince(ctx context.Context, platform string, since time.Time, topic string, enableSentiment bool) (*SyncResult, error) {
	syncer, err := s.factory.GetSyncer(platform)
	if err != nil {
		return nil, err
	}

	sourceID := s.getOrCreateDefaultSource()
	config := SyncConfig{
		Platform:        platform,
		SyncMode:        "incremental",
		LastSyncTime:    since,
		SourceID:        sourceID,
		EnableSentiment: enableSentiment,
		Topic:           topic,
	}

	progress := s.progressTracker.StartProgress(platform, 0)
	if err := syncer.Sync(ctx, config, progress); err != nil {
		progress.SetError(err)
		return s.progressToResult(progress), err
	}

	progress.SetStatus("completed")
	return s.progressToResult(progress), nil
}

// SyncPlatformFromSourceID 按源表主键增量同步（推荐用于爬虫后续节点）
// 只处理源表中 id > sinceSourceID 的新行，与源帖子发帖时间无关。
func (s *PlatformSyncService) SyncPlatformFromSourceID(ctx context.Context, platform string, sinceSourceID uint, enableSentiment bool) (*SyncResult, error) {
	syncer, err := s.factory.GetSyncer(platform)
	if err != nil {
		return nil, err
	}

	sourceID := s.getOrCreateDefaultSource()
	config := SyncConfig{
		Platform:        platform,
		SyncMode:        "incremental",
		SourceID:        sourceID,
		EnableSentiment: enableSentiment,
		MinSourceID:     sinceSourceID,
	}

	progress := s.progressTracker.StartProgress(platform, 0)
	if err := syncer.Sync(ctx, config, progress); err != nil {
		progress.SetError(err)
		return s.progressToResult(progress), err
	}

	progress.SetStatus("completed")
	return s.progressToResult(progress), nil
}

// MaxSourceTableID 查询指定源表当前的 max(id)，作为爬虫前的 baseline
func (s *PlatformSyncService) MaxSourceTableID(ctx context.Context, sourceTable string) (uint, error) {
	if sourceTable == "" {
		return 0, fmt.Errorf("empty source table")
	}
	var maxID uint
	err := s.db.WithContext(ctx).Table(sourceTable).
		Select("COALESCE(MAX(id), 0)").Scan(&maxID).Error
	return maxID, err
}

// SyncSinglePlatform 同步单个平台
func (s *PlatformSyncService) SyncSinglePlatform(ctx context.Context, platform string) (*SyncResult, error) {
	// 获取同步器
	syncer, err := s.factory.GetSyncer(platform)
	if err != nil {
		return nil, err
	}

	// 获取最后同步时间
	lastSyncTime, _ := s.GetLastSyncTime(platform)

	// 获取或创建默认数据源
	sourceID := s.getOrCreateDefaultSource()

	// 创建同步配置
	config := SyncConfig{
		Platform:        platform,
		SyncMode:        "incremental",
		LastSyncTime:    lastSyncTime,
		SourceID:        sourceID,
		EnableSentiment: true, // 启用情感分析
	}

	// 创建进度跟踪
	progress := s.progressTracker.StartProgress(platform, 0)

	// 执行同步
	err = syncer.Sync(ctx, config, progress)

	if err != nil {
		progress.SetError(err)
		return s.progressToResult(progress), err
	}

	progress.SetStatus("completed")
	return s.progressToResult(progress), nil
}

// triggerDigestGeneration 触发摘要生成（异步）
func (s *PlatformSyncService) triggerDigestGeneration(ctx context.Context, results map[string]*SyncResult) {
	if s.digestGen == nil {
		return
	}

	// 检查是否有成功的同步
	hasSuccess := false
	totalNew := 0
	for _, result := range results {
		if result.Status == "completed" && result.NewCount > 0 {
			hasSuccess = true
			totalNew += result.NewCount
		}
	}

	if !hasSuccess || totalNew == 0 {
		log.Printf("[digest] 本次同步无新数据，跳过摘要生成")
		return
	}

	log.Printf("[digest] 检测到新数据（%d条），触发摘要生成...", totalNew)

	// 异步生成摘要
	go func() {
		genCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		if err := s.digestGen.GenerateRecentDigest(genCtx); err != nil {
			log.Printf("[digest] 生成摘要失败: %v", err)
		}
	}()
}

// getOrCreateDefaultSource 获取或创建默认数据源
func (s *PlatformSyncService) getOrCreateDefaultSource() uint {
	var source model.DataSource

	// 尝试查找默认数据源
	err := s.db.Where("name = ?", "MediaCrawler").First(&source).Error
	if err == nil {
		return source.ID
	}

	// 不存在则创建
	source = model.DataSource{
		Name:   "MediaCrawler",
		Type:   "crawler",
		Config: "{}",  // JSON 字段不能为空字符串
		Status: 1,
	}

	if err := s.db.Create(&source).Error; err != nil {
		// 创建失败，返回 0（会导致外键错误，但至少不会 panic）
		return 0
	}

	return source.ID
}

// SyncAllPlatforms 同步所有平台
func (s *PlatformSyncService) SyncAllPlatforms(ctx context.Context) (map[string]*SyncResult, error) {
	platforms := s.factory.GetAllPlatforms()
	return s.SyncPlatforms(ctx, platforms)
}

// GetSyncProgress 获取同步进度
func (s *PlatformSyncService) GetSyncProgress(platform string) *SyncProgress {
	return s.progressTracker.GetProgress(platform)
}

// GetAllSyncProgress 获取所有平台的同步进度
func (s *PlatformSyncService) GetAllSyncProgress() []SyncProgress {
	return s.progressTracker.GetAllProgress()
}

// GetLastSyncTime 获取平台的最后同步时间
func (s *PlatformSyncService) GetLastSyncTime(platform string) (time.Time, error) {
	var article model.Article
	err := s.db.Where("platform = ?", platform).Order("published_at DESC").First(&article).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return article.PublishedAt, nil
}

// GetPlatformList 获取所有支持的平台列表
func (s *PlatformSyncService) GetPlatformList() []PlatformInfo {
	platforms := []PlatformInfo{
		{Code: "xhs", Name: "小红书", Table: "xhs_note"},
		{Code: "dy", Name: "抖音", Table: "douyin_aweme"},
		{Code: "bili", Name: "B站", Table: "bilibili_video"},
		{Code: "wb", Name: "微博", Table: "weibo_note"},
		{Code: "ks", Name: "快手", Table: "kuaishou_video"},
		{Code: "tieba", Name: "贴吧", Table: "tieba_note"},
		{Code: "zhihu", Name: "知乎", Table: "zhihu_content"},
	}

	// 为每个平台添加最后同步时间
	for i := range platforms {
		lastSyncTime, _ := s.GetLastSyncTime(platforms[i].Code)
		platforms[i].LastSyncTime = lastSyncTime
	}

	return platforms
}

// PlatformInfo 平台信息
type PlatformInfo struct {
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	Table        string    `json:"table"`
	LastSyncTime time.Time `json:"lastSyncTime"`
}

// progressToResult 将进度转换为结果
func (s *PlatformSyncService) progressToResult(progress *SyncProgress) *SyncResult {
	snapshot := progress.GetSnapshot()
	return &SyncResult{
		Platform:     snapshot.Platform,
		TotalCount:   snapshot.TotalCount,
		NewCount:     snapshot.NewCount,
		SkippedCount: snapshot.SkippedCount,
		ErrorCount:   snapshot.ErrorCount,
		StartTime:    snapshot.StartTime,
		EndTime:      snapshot.EndTime,
		Duration:     snapshot.Duration,
		Status:       snapshot.Status,
		ErrorMessage: snapshot.ErrorMessage,
	}
}

// IncrementalSync 增量同步平台数据到 articles 表（保留兼容性）
func (s *PlatformSyncService) IncrementalSync(ctx context.Context, config SyncConfig) (*SyncResult, error) {
	return s.SyncSinglePlatform(ctx, config.Platform)
}
