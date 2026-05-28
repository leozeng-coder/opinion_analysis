package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"opinion-analysis/src/model"
)

// XhsSyncer 小红书同步器
type XhsSyncer struct {
	*BaseSyncer
}

func (s *XhsSyncer) GetPlatformName() string { return "小红书" }
func (s *XhsSyncer) GetPlatformCode() string { return "xhs" }
func (s *XhsSyncer) GetSourceTable() string  { return "xhs_note" }

func (s *XhsSyncer) Sync(ctx context.Context, config SyncConfig, progress *SyncProgress) error {
	var notes []model.XhsNote

	query := s.db.WithContext(ctx).Table("xhs_note")
	if config.MinSourceID > 0 {
		query = query.Where("id > ?", config.MinSourceID)
	} else if config.SyncMode == "incremental" && !config.LastSyncTime.IsZero() {
		query = query.Where("time > ?", config.LastSyncTime.Unix())
	}

	if err := query.Order("id ASC").Find(&notes).Error; err != nil {
		return fmt.Errorf("query xhs_note failed: %w", err)
	}

	progress.TotalCount = len(notes)
	fmt.Printf("[XhsSyncer] 查询到 %d 条小红书数据\n", len(notes))

	for i, note := range notes {
		exists, err := s.checkDuplicate(note.NoteURL)
		if err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		if exists {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount+1, progress.ErrorCount)
			continue
		}

		// 处理时间戳：如果是毫秒级（13位），转换为秒级
		timestamp := note.Time
		if timestamp > 9999999999 { // 大于10位数字，说明是毫秒级
			timestamp = timestamp / 1000
		}

		article := model.Article{
			SourceID:    config.SourceID,
			Title:       note.Title,
			Content:     note.Desc,
			Author:      note.Nickname,
			OriginURL:   note.NoteURL,
			Platform:    "xhs",
			PublishedAt: time.Unix(timestamp, 0),
			Keywords:    extractKeywords(note.TagList),
			Sentiment:   "neutral",
			SentScore:   0.5,
		}

		if config.EnableSentiment {
			article.Sentiment, article.SentScore = s.analyzeSentiment(config.SentimentEndpoint, article.Content)
		}

		if err := s.saveArticle(&article); err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		progress.Update(i+1, progress.NewCount+1, progress.SkippedCount, progress.ErrorCount)
	}

	return nil
}

// DouyinSyncer 抖音同步器
type DouyinSyncer struct {
	*BaseSyncer
}

func (s *DouyinSyncer) GetPlatformName() string { return "抖音" }
func (s *DouyinSyncer) GetPlatformCode() string { return "dy" }
func (s *DouyinSyncer) GetSourceTable() string  { return "douyin_aweme" }

func (s *DouyinSyncer) Sync(ctx context.Context, config SyncConfig, progress *SyncProgress) error {
	var awemes []model.DouyinAweme

	query := s.db.WithContext(ctx).Table("douyin_aweme")
	if config.MinSourceID > 0 {
		query = query.Where("id > ?", config.MinSourceID)
	} else if config.SyncMode == "incremental" && !config.LastSyncTime.IsZero() {
		query = query.Where("create_time > ?", config.LastSyncTime.Unix())
	}

	if err := query.Order("id ASC").Find(&awemes).Error; err != nil {
		return fmt.Errorf("query douyin_aweme failed: %w", err)
	}

	progress.TotalCount = len(awemes)
	fmt.Printf("[DouyinSyncer] 查询到 %d 条抖音数据\n", len(awemes))

	for i, aweme := range awemes {
		exists, err := s.checkDuplicate(aweme.AwemeURL)
		if err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		if exists {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount+1, progress.ErrorCount)
			continue
		}

		article := model.Article{
			SourceID:    config.SourceID,
			Title:       aweme.Title,
			Content:     aweme.Desc,
			Author:      aweme.Nickname,
			OriginURL:   aweme.AwemeURL,
			Platform:    "douyin",
			PublishedAt: time.Unix(aweme.CreateTime, 0),
			Keywords:    "[]",
			Sentiment:   "neutral",
			SentScore:   0.5,
		}

		if config.EnableSentiment {
			article.Sentiment, article.SentScore = s.analyzeSentiment(config.SentimentEndpoint, article.Content)
		}

		if err := s.saveArticle(&article); err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		progress.Update(i+1, progress.NewCount+1, progress.SkippedCount, progress.ErrorCount)
	}

	return nil
}

// BilibiliSyncer B站同步器
type BilibiliSyncer struct {
	*BaseSyncer
}

func (s *BilibiliSyncer) GetPlatformName() string { return "B站" }
func (s *BilibiliSyncer) GetPlatformCode() string { return "bili" }
func (s *BilibiliSyncer) GetSourceTable() string  { return "bilibili_video" }

func (s *BilibiliSyncer) Sync(ctx context.Context, config SyncConfig, progress *SyncProgress) error {
	var videos []model.BilibiliVideo

	query := s.db.WithContext(ctx).Table("bilibili_video")
	if config.MinSourceID > 0 {
		query = query.Where("id > ?", config.MinSourceID)
	} else if config.SyncMode == "incremental" && !config.LastSyncTime.IsZero() {
		query = query.Where("create_time > ?", config.LastSyncTime.Unix())
	}

	if err := query.Order("id ASC").Find(&videos).Error; err != nil {
		return fmt.Errorf("query bilibili_video failed: %w", err)
	}

	progress.TotalCount = len(videos)
	fmt.Printf("[BilibiliSyncer] 查询到 %d 条B站数据\n", len(videos))

	for i, video := range videos {
		exists, err := s.checkDuplicate(video.VideoURL)
		if err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		if exists {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount+1, progress.ErrorCount)
			continue
		}

		article := model.Article{
			SourceID:    config.SourceID,
			Title:       video.Title,
			Content:     video.Desc,
			Author:      video.Nickname,
			OriginURL:   video.VideoURL,
			Platform:    "bilibili",
			PublishedAt: time.Unix(video.CreateTime, 0),
			Keywords:    "[]",
			Sentiment:   "neutral",
			SentScore:   0.5,
		}

		if config.EnableSentiment {
			article.Sentiment, article.SentScore = s.analyzeSentiment(config.SentimentEndpoint, article.Content)
		}

		if err := s.saveArticle(&article); err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		progress.Update(i+1, progress.NewCount+1, progress.SkippedCount, progress.ErrorCount)
	}

	return nil
}

// WeiboSyncer 微博同步器
type WeiboSyncer struct {
	*BaseSyncer
}

func (s *WeiboSyncer) GetPlatformName() string { return "微博" }
func (s *WeiboSyncer) GetPlatformCode() string { return "wb" }
func (s *WeiboSyncer) GetSourceTable() string  { return "weibo_note" }

func (s *WeiboSyncer) Sync(ctx context.Context, config SyncConfig, progress *SyncProgress) error {
	var notes []model.WeiboNote

	query := s.db.WithContext(ctx).Table("weibo_note")
	if config.MinSourceID > 0 {
		query = query.Where("id > ?", config.MinSourceID)
	} else if config.SyncMode == "incremental" && !config.LastSyncTime.IsZero() {
		query = query.Where("create_time > ?", config.LastSyncTime.Unix())
	}

	if err := query.Order("id ASC").Find(&notes).Error; err != nil {
		return fmt.Errorf("query weibo_note failed: %w", err)
	}

	progress.TotalCount = len(notes)
	fmt.Printf("[WeiboSyncer] 查询到 %d 条微博数据\n", len(notes))

	for i, note := range notes {
		exists, err := s.checkDuplicate(note.NoteURL)
		if err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		if exists {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount+1, progress.ErrorCount)
			continue
		}

		title := note.Content
		if len(title) > 50 {
			title = title[:50] + "..."
		}

		article := model.Article{
			SourceID:    config.SourceID,
			Title:       title,
			Content:     note.Content,
			Author:      note.Nickname,
			OriginURL:   note.NoteURL,
			Platform:    "weibo",
			PublishedAt: time.Unix(note.CreateTime, 0),
			Keywords:    "[]",
			Sentiment:   "neutral",
			SentScore:   0.5,
		}

		if config.EnableSentiment {
			article.Sentiment, article.SentScore = s.analyzeSentiment(config.SentimentEndpoint, article.Content)
		}

		if err := s.saveArticle(&article); err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		progress.Update(i+1, progress.NewCount+1, progress.SkippedCount, progress.ErrorCount)
	}

	return nil
}

// KuaishouSyncer 快手同步器
type KuaishouSyncer struct {
	*BaseSyncer
}

func (s *KuaishouSyncer) GetPlatformName() string { return "快手" }
func (s *KuaishouSyncer) GetPlatformCode() string { return "ks" }
func (s *KuaishouSyncer) GetSourceTable() string  { return "kuaishou_video" }

func (s *KuaishouSyncer) Sync(ctx context.Context, config SyncConfig, progress *SyncProgress) error {
	var videos []model.KuaishouVideo

	query := s.db.WithContext(ctx).Table("kuaishou_video")
	if config.MinSourceID > 0 {
		query = query.Where("id > ?", config.MinSourceID)
	} else if config.SyncMode == "incremental" && !config.LastSyncTime.IsZero() {
		query = query.Where("create_time > ?", config.LastSyncTime.Unix())
	}

	if err := query.Order("id ASC").Find(&videos).Error; err != nil {
		return fmt.Errorf("query kuaishou_video failed: %w", err)
	}

	progress.TotalCount = len(videos)
	fmt.Printf("[KuaishouSyncer] 查询到 %d 条快手数据\n", len(videos))

	for i, video := range videos {
		exists, err := s.checkDuplicate(video.VideoURL)
		if err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		if exists {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount+1, progress.ErrorCount)
			continue
		}

		article := model.Article{
			SourceID:    config.SourceID,
			Title:       video.Title,
			Content:     video.Desc,
			Author:      video.Nickname,
			OriginURL:   video.VideoURL,
			Platform:    "kuaishou",
			PublishedAt: time.Unix(video.CreateTime, 0),
			Keywords:    "[]",
			Sentiment:   "neutral",
			SentScore:   0.5,
		}

		if config.EnableSentiment {
			article.Sentiment, article.SentScore = s.analyzeSentiment(config.SentimentEndpoint, article.Content)
		}

		if err := s.saveArticle(&article); err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		progress.Update(i+1, progress.NewCount+1, progress.SkippedCount, progress.ErrorCount)
	}

	return nil
}

// TiebaSyncer 贴吧同步器
type TiebaSyncer struct {
	*BaseSyncer
}

func (s *TiebaSyncer) GetPlatformName() string { return "贴吧" }
func (s *TiebaSyncer) GetPlatformCode() string { return "tieba" }
func (s *TiebaSyncer) GetSourceTable() string  { return "tieba_note" }

func (s *TiebaSyncer) Sync(ctx context.Context, config SyncConfig, progress *SyncProgress) error {
	var notes []model.TiebaNote

	query := s.db.WithContext(ctx).Table("tieba_note")
	if config.MinSourceID > 0 {
		query = query.Where("id > ?", config.MinSourceID)
	} else if config.SyncMode == "incremental" && !config.LastSyncTime.IsZero() {
		query = query.Where("publish_time > ?", config.LastSyncTime.Format("2006-01-02 15:04:05"))
	}

	if err := query.Order("id ASC").Find(&notes).Error; err != nil {
		return fmt.Errorf("query tieba_note failed: %w", err)
	}

	progress.TotalCount = len(notes)
	fmt.Printf("[TiebaSyncer] 查询到 %d 条贴吧数据\n", len(notes))

	for i, note := range notes {
		exists, err := s.checkDuplicate(note.NoteURL)
		if err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		if exists {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount+1, progress.ErrorCount)
			continue
		}

		publishTime, err := time.Parse("2006-01-02 15:04:05", note.PublishTime)
		if err != nil {
			// 记录详细的时间解析错误
			fmt.Printf("[TiebaSyncer] 时间解析失败 - NoteID: %s, PublishTime: '%s', Error: %v\n",
				note.NoteID, note.PublishTime, err)
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		article := model.Article{
			SourceID:    config.SourceID,
			Title:       note.Title,
			Content:     note.Desc,
			Author:      note.UserNickname,
			OriginURL:   note.NoteURL,
			Platform:    "tieba",
			PublishedAt: publishTime,
			Keywords:    "[]",
			Sentiment:   "neutral",
			SentScore:   0.5,
		}

		if config.EnableSentiment {
			article.Sentiment, article.SentScore = s.analyzeSentiment(config.SentimentEndpoint, article.Content)
		}

		if err := s.saveArticle(&article); err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		progress.Update(i+1, progress.NewCount+1, progress.SkippedCount, progress.ErrorCount)
	}

	return nil
}

// ZhihuSyncer 知乎同步器
type ZhihuSyncer struct {
	*BaseSyncer
}

func (s *ZhihuSyncer) GetPlatformName() string { return "知乎" }
func (s *ZhihuSyncer) GetPlatformCode() string { return "zhihu" }
func (s *ZhihuSyncer) GetSourceTable() string  { return "zhihu_content" }

func (s *ZhihuSyncer) Sync(ctx context.Context, config SyncConfig, progress *SyncProgress) error {
	var contents []model.ZhihuContent

	query := s.db.WithContext(ctx).Table("zhihu_content")
	if config.MinSourceID > 0 {
		query = query.Where("id > ?", config.MinSourceID)
	} else if config.SyncMode == "incremental" && !config.LastSyncTime.IsZero() {
		query = query.Where("created_time > ?", config.LastSyncTime.Format("2006-01-02 15:04:05"))
	}

	if err := query.Order("id ASC").Find(&contents).Error; err != nil {
		return fmt.Errorf("query zhihu_content failed: %w", err)
	}

	progress.TotalCount = len(contents)
	fmt.Printf("[ZhihuSyncer] 查询到 %d 条知乎数据\n", len(contents))

	for i, content := range contents {
		exists, err := s.checkDuplicate(content.ContentURL)
		if err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		if exists {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount+1, progress.ErrorCount)
			continue
		}

		// 知乎的 CreatedTime 实际存储的是 Unix 时间戳（字符串格式）
		var createdTime time.Time
		// 先尝试解析为 Unix 时间戳
		if timestamp, err := strconv.ParseInt(content.CreatedTime, 10, 64); err == nil {
			createdTime = time.Unix(timestamp, 0)
		} else {
			// 如果不是时间戳，尝试解析为日期时间字符串
			if t, err := time.Parse("2006-01-02 15:04:05", content.CreatedTime); err == nil {
				createdTime = t
			} else {
				// 两种格式都解析失败
				fmt.Printf("[ZhihuSyncer] 时间解析失败 - ContentID: %s, CreatedTime: '%s', Error: %v\n",
					content.ContentID, content.CreatedTime, err)
				progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
				continue
			}
		}

		article := model.Article{
			SourceID:    config.SourceID,
			Title:       content.Title,
			Content:     content.ContentText,
			Author:      content.UserNickname,
			OriginURL:   content.ContentURL,
			Platform:    "zhihu",
			PublishedAt: createdTime,
			Keywords:    "[]",
			Sentiment:   "neutral",
			SentScore:   0.5,
		}

		if config.EnableSentiment {
			article.Sentiment, article.SentScore = s.analyzeSentiment(config.SentimentEndpoint, article.Content)
		}

		if err := s.saveArticle(&article); err != nil {
			progress.Update(i+1, progress.NewCount, progress.SkippedCount, progress.ErrorCount+1)
			continue
		}

		progress.Update(i+1, progress.NewCount+1, progress.SkippedCount, progress.ErrorCount)
	}

	return nil
}

// extractKeywords 从标签列表提取关键词
func extractKeywords(tagList string) string {
	if tagList == "" {
		return "[]"
	}
	trimmed := strings.TrimSpace(tagList)
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		return tagList
	}
	return "[]"
}
