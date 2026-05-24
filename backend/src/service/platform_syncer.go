package service

import (
	"context"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"opinion-analysis/src/model"
	"opinion-analysis/src/service/sentiment"
)

// removeEmoji 移除字符串中的 emoji 和 4 字节 UTF-8 字符
func removeEmoji(text string) string {
	// 移除所有 4 字节 UTF-8 字符（包括 emoji、特殊符号、全角标点等）
	// 只保留 1-3 字节的 UTF-8 字符（基本中文、英文、数字、常用标点）
	result := make([]byte, 0, len(text))
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r != utf8.RuneError && size <= 3 {
			result = append(result, text[i:i+size]...)
		}
		i += size
	}
	return string(result)
}

// PlatformSyncer 平台同步器接口
type PlatformSyncer interface {
	Sync(ctx context.Context, config SyncConfig, progress *SyncProgress) error
	GetPlatformName() string
	GetPlatformCode() string
	GetSourceTable() string
}

// BaseSyncer 基础同步器（提供通用方法）
type BaseSyncer struct {
	db                *gorm.DB
	sentimentAnalyzer *sentiment.Analyzer
}

// checkDuplicate 检查是否已存在（根据 origin_url 去重）
func (b *BaseSyncer) checkDuplicate(url string) (bool, error) {
	var count int64
	if err := b.db.Model(&model.Article{}).Where("origin_url = ?", url).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// saveArticle 保存文章到 articles 表
func (b *BaseSyncer) saveArticle(article *model.Article) error {
	// 清理 emoji 和特殊字符
	article.Title = removeEmoji(article.Title)
	article.Content = removeEmoji(article.Content)
	article.Author = removeEmoji(article.Author)

	return b.db.Create(article).Error
}

// analyzeSentiment 情感分析（基于 LLM）
func (b *BaseSyncer) analyzeSentiment(endpoint, content string) (string, float64) {
	// endpoint 参数保留用于向后兼容，但现在使用统一的 LLM 配置
	if b.sentimentAnalyzer == nil {
		return "neutral", 0.500
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := b.sentimentAnalyzer.Analyze(ctx, content)
	if err != nil {
		// 分析失败时返回中性
		return "neutral", 0.500
	}

	return result.Sentiment, result.Score
}

// SyncerFactory 同步器工厂
type SyncerFactory struct {
	db      *gorm.DB
	syncers map[string]PlatformSyncer
	mu      sync.RWMutex
}

// NewSyncerFactory 创建同步器工厂
func NewSyncerFactory(db *gorm.DB) *SyncerFactory {
	factory := &SyncerFactory{
		db:      db,
		syncers: make(map[string]PlatformSyncer),
	}

	// 注册所有平台的同步器
	factory.registerSyncers()

	return factory
}

// registerSyncers 注册所有平台同步器
func (f *SyncerFactory) registerSyncers() {
	base := &BaseSyncer{
		db:                f.db,
		sentimentAnalyzer: sentiment.New(f.db),
	}

	f.syncers["xhs"] = &XhsSyncer{BaseSyncer: base}
	f.syncers["dy"] = &DouyinSyncer{BaseSyncer: base}
	f.syncers["bili"] = &BilibiliSyncer{BaseSyncer: base}
	f.syncers["wb"] = &WeiboSyncer{BaseSyncer: base}
	f.syncers["ks"] = &KuaishouSyncer{BaseSyncer: base}
	f.syncers["tieba"] = &TiebaSyncer{BaseSyncer: base}
	f.syncers["zhihu"] = &ZhihuSyncer{BaseSyncer: base}
}

// GetSyncer 获取指定平台的同步器
func (f *SyncerFactory) GetSyncer(platform string) (PlatformSyncer, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	syncer, ok := f.syncers[platform]
	if !ok {
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
	return syncer, nil
}

// GetAllSyncers 获取所有平台同步器
func (f *SyncerFactory) GetAllSyncers() []PlatformSyncer {
	f.mu.RLock()
	defer f.mu.RUnlock()

	syncers := make([]PlatformSyncer, 0, len(f.syncers))
	for _, syncer := range f.syncers {
		syncers = append(syncers, syncer)
	}
	return syncers
}

// GetAllPlatforms 获取所有支持的平台代码
func (f *SyncerFactory) GetAllPlatforms() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	platforms := make([]string, 0, len(f.syncers))
	for platform := range f.syncers {
		platforms = append(platforms, platform)
	}
	return platforms
}

// SyncProgress 同步进度
type SyncProgress struct {
	Platform     string    `json:"platform"`
	Status       string    `json:"status"` // pending/running/completed/failed
	TotalCount   int       `json:"totalCount"`
	ProcessedCount int     `json:"processedCount"`
	NewCount     int       `json:"newCount"`
	SkippedCount int       `json:"skippedCount"`
	ErrorCount   int       `json:"errorCount"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	Duration     string    `json:"duration"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
	mu           sync.RWMutex
}

// Update 更新进度
func (p *SyncProgress) Update(processed, new, skipped, errors int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ProcessedCount = processed
	p.NewCount = new
	p.SkippedCount = skipped
	p.ErrorCount = errors
}

// SetStatus 设置状态
func (p *SyncProgress) SetStatus(status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Status = status
	if status == "completed" || status == "failed" {
		p.EndTime = time.Now()
		p.Duration = p.EndTime.Sub(p.StartTime).String()
	}
}

// SetError 设置错误信息
func (p *SyncProgress) SetError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Status = "failed"
	p.ErrorMessage = err.Error()
	p.EndTime = time.Now()
	p.Duration = p.EndTime.Sub(p.StartTime).String()
}

// GetSnapshot 获取进度快照（线程安全）
func (p *SyncProgress) GetSnapshot() SyncProgress {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return SyncProgress{
		Platform:       p.Platform,
		Status:         p.Status,
		TotalCount:     p.TotalCount,
		ProcessedCount: p.ProcessedCount,
		NewCount:       p.NewCount,
		SkippedCount:   p.SkippedCount,
		ErrorCount:     p.ErrorCount,
		StartTime:      p.StartTime,
		EndTime:        p.EndTime,
		Duration:       p.Duration,
		ErrorMessage:   p.ErrorMessage,
	}
}

// ProgressTracker 进度跟踪器
type ProgressTracker struct {
	progresses map[string]*SyncProgress
	mu         sync.RWMutex
}

// NewProgressTracker 创建进度跟踪器
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{
		progresses: make(map[string]*SyncProgress),
	}
}

// StartProgress 开始跟踪进度
func (t *ProgressTracker) StartProgress(platform string, totalCount int) *SyncProgress {
	t.mu.Lock()
	defer t.mu.Unlock()

	progress := &SyncProgress{
		Platform:   platform,
		Status:     "running",
		TotalCount: totalCount,
		StartTime:  time.Now(),
	}
	t.progresses[platform] = progress
	return progress
}

// GetProgress 获取指定平台的进度
func (t *ProgressTracker) GetProgress(platform string) *SyncProgress {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.progresses[platform]
}

// GetAllProgress 获取所有平台的进度快照
func (t *ProgressTracker) GetAllProgress() []SyncProgress {
	t.mu.RLock()
	defer t.mu.RUnlock()

	snapshots := make([]SyncProgress, 0, len(t.progresses))
	for _, progress := range t.progresses {
		snapshots = append(snapshots, progress.GetSnapshot())
	}
	return snapshots
}

// ClearProgress 清除指定平台的进度
func (t *ProgressTracker) ClearProgress(platform string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.progresses, platform)
}

// ClearAllProgress 清除所有进度
func (t *ProgressTracker) ClearAllProgress() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.progresses = make(map[string]*SyncProgress)
}
