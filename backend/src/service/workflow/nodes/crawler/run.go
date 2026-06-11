package crawler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
	"opinion-analysis/src/repository"
	platformSync "opinion-analysis/src/service"
	crawlerSvc "opinion-analysis/src/service/crawler"
	"opinion-analysis/src/service/workflow/nodes"
)

// RunNode 执行爬虫任务，仅负责触发 MediaCrawler 并记录源表 baseline，
// 不做数据同步（由下游 platform_sync 节点处理）。
type RunNode struct {
	*nodes.BaseNode
	db          *gorm.DB
	crawlerRepo *repository.CrawlerRepository
	crawlerSvc  *crawlerSvc.Service

	// 所有活跃的 runID（支持并发多节点同时执行）
	activeRunIDs sync.Map // key: uint64(runID), value: string(platform)
}

func NewRunNode(db *gorm.DB, crawlerRepo *repository.CrawlerRepository, systemRepo *repository.SystemRepository) *RunNode {
	return &RunNode{
		BaseNode:    nodes.NewBaseNode("crawler_run"),
		db:          db,
		crawlerRepo: crawlerRepo,
		crawlerSvc:  crawlerSvc.NewService(crawlerRepo, systemRepo),
	}
}

func (n *RunNode) Validate(config map[string]interface{}) error {
	topics := n.GetStringSlice(config, "topics")
	if len(topics) == 0 {
		return fmt.Errorf("话题（topics）为必填项")
	}
	return nil
}

func (n *RunNode) Execute(ctx context.Context, config map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	// 平台：优先 platform（新节点单选），兼容旧字段 platforms（多选，取第一个）
	platform := n.GetString(config, "platform", "")
	var platforms []string
	if platform != "" {
		platforms = []string{platform}
	} else {
		platforms = n.GetStringSlice(config, "platforms")
		if len(platforms) == 0 {
			platforms = []string{"zhihu"}
		}
		platform = platforms[0]
	}

	crawlerType       := n.GetString(config, "crawlerType", "search")
	keywords          := n.GetStringSlice(config, "keywords")
	specifiedIds      := n.GetString(config, "specifiedIds", "")
	creatorIds        := n.GetString(config, "creatorIds", "")
	loginType         := n.GetString(config, "loginType", "cookie")
	saveOption        := n.GetString(config, "saveOption", "db")
	startPage         := n.GetInt(config, "startPage", 1)
	enableComments    := n.GetBool(config, "enableComments", true)
	enableSubComments := n.GetBool(config, "enableSubComments", false)
	headless          := n.GetBool(config, "headless", true)
	topics            := n.GetStringSlice(config, "topics")
	waitForCompletion := n.GetBool(config, "waitForCompletion", true)
	timeoutMinutes    := n.GetInt(config, "timeoutMinutes", 60)
	maxNotesCount        := n.GetInt(config, "maxNotesCount", 0)        // 0 = 使用后台管理配置的默认值
	maxCommentsCount     := n.GetInt(config, "maxCommentsCount", 0)     // 0 = 使用后台管理配置的默认值
	maxSubCommentsCount  := n.GetInt(config, "maxSubCommentsCount", 0)  // 0 = 使用后台管理配置的默认值

	syncCodes := platformSync.ResolveSyncCodes(platforms)

	// 触发爬虫前，记录每个目标源表的 max(id)，
	// 下游 platform_sync 节点以此作为「本次新增」的主键 baseline。
	sourceMaxIDs := n.captureSourceBaselines(ctx, syncCodes)

	log.Printf("[CrawlerRunNode] Starting crawler: platform=%s crawlerType=%s keywords=%v sourceBaselines=%v",
		platform, crawlerType, keywords, sourceMaxIDs)

	result, err := n.crawlerSvc.Trigger(ctx, crawlerSvc.TriggerParams{
		Spiders:           platforms, // 兼容旧字段
		Platform:          platform,
		CrawlerType:       crawlerType,
		Keywords:          keywords,
		SpecifiedIds:      specifiedIds,
		CreatorIds:        creatorIds,
		LoginType:         loginType,
		SaveOption:        saveOption,
		StartPage:         startPage,
		EnableComments:    enableComments,
		EnableSubComments: enableSubComments,
		Headless:          headless,
		Topics:            topics,
		TimeoutMinutes:      timeoutMinutes,
		MaxNotesCount:       maxNotesCount,       // 工作流可覆盖爬取数量
		MaxCommentsCount:    maxCommentsCount,    // 工作流可覆盖一级评论数量
		MaxSubCommentsCount: maxSubCommentsCount, // 工作流可覆盖二级评论数量
	})
	if err != nil {
		return nil, n.WrapError("failed to trigger crawler", err)
	}

	log.Printf("[CrawlerRunNode] Crawler triggered: runId=%d", result.RunID)
	n.activeRunIDs.Store(uint64(result.RunID), platform)
	defer n.activeRunIDs.Delete(uint64(result.RunID))

	commentMaxIDs := n.captureCommentBaselines(ctx, syncCodes)

	var waitErr error
	if waitForCompletion {
		timeout := time.Duration(timeoutMinutes) * time.Minute
		if err := n.crawlerSvc.WaitForCompletion(ctx, result.RunID, timeout); err != nil {
			waitErr = err
			log.Printf("[CrawlerRunNode] Crawler wait failed (data may still exist): runId=%d err=%v", result.RunID, err)
		} else {
			log.Printf("[CrawlerRunNode] Crawler finished: runId=%d", result.RunID)

			// 统计本次爬取新增的文章数和评论数
			newArticles := n.countNewRows(ctx, syncCodes, sourceMaxIDs, platformSync.SyncCodeToSourceTable)
			newComments := n.countNewRows(ctx, syncCodes, commentMaxIDs, platformSync.SyncCodeToCommentTable)
			log.Printf("[CrawlerRunNode] 本次爬取完成：新增文章 %d 篇，新增评论 %d 条（runId=%d）",
				newArticles, newComments, result.RunID)
		}
	}

	status := "triggered"
	if waitForCompletion && waitErr == nil {
		status = "completed"
	} else if waitForCompletion && waitErr != nil {
		status = "timeout"
	}

	produced := map[string]interface{}{
		"crawlerRunId":       result.RunID,
		"crawlerStartedAt":   result.StartedAt.Format(time.RFC3339),
		"platform":           platform,
		"syncPlatformCodes":  syncCodes,
		"sourceMaxIdsBefore": sourceMaxIDs,
		"keywords":           keywords,
		"topics":             topics,
		"status":             status,
		"waitedCompletion":   waitForCompletion,
	}

	if waitErr != nil {
		produced["timeoutError"] = waitErr.Error()
		// 返回 partial_success：数据可能已写入源表，元信息保留给下游使用
		return nodes.CarryForward(input, produced), n.WrapError("crawler timeout (data may exist in source tables)", waitErr)
	}

	return nodes.CarryForward(input, produced), nil
}

// OnCancel 覆盖：向所有活跃的 MediaCrawler 发 stop 信号。
func (n *RunNode) OnCancel(_ context.Context) {
	n.activeRunIDs.Range(func(key, value any) bool {
		runID := uint(key.(uint64))
		platform, _ := value.(string)
		log.Printf("[CrawlerRunNode] OnCancel: sending stop to MediaCrawler for runId=%d platform=%s", runID, platform)
		go n.crawlerSvc.StopCrawler(runID, platform)
		return true
	})
}

// captureSourceBaselines 查询每个平台对应源表当前的 max(id)
func (n *RunNode) captureSourceBaselines(ctx context.Context, syncCodes []string) map[string]uint {
	baselines := make(map[string]uint, len(syncCodes))
	for _, code := range syncCodes {
		table := platformSync.SyncCodeToSourceTable(code)
		if table == "" {
			log.Printf("[CrawlerRunNode] Unknown source table for syncCode=%s, baseline=0", code)
			baselines[code] = 0
			continue
		}
		var maxID uint
		if err := n.db.WithContext(ctx).Table(table).
			Select("COALESCE(MAX(id), 0)").Scan(&maxID).Error; err != nil {
			log.Printf("[CrawlerRunNode] Query max id of %s failed: %v (treat as 0)", table, err)
			baselines[code] = 0
			continue
		}
		baselines[code] = maxID
	}
	return baselines
}

// captureCommentBaselines 查询每个平台评论表当前的 max(id)
func (n *RunNode) captureCommentBaselines(ctx context.Context, syncCodes []string) map[string]uint {
	baselines := make(map[string]uint, len(syncCodes))
	for _, code := range syncCodes {
		table := platformSync.SyncCodeToCommentTable(code)
		if table == "" {
			baselines[code] = 0
			continue
		}
		var maxID uint
		if err := n.db.WithContext(ctx).Table(table).
			Select("COALESCE(MAX(id), 0)").Scan(&maxID).Error; err != nil {
			log.Printf("[CrawlerRunNode] Query max id of %s failed: %v (treat as 0)", table, err)
			baselines[code] = 0
			continue
		}
		baselines[code] = maxID
	}
	return baselines
}

// countNewRows 统计各平台表中 id > baseline 的新增行数之和
func (n *RunNode) countNewRows(ctx context.Context, syncCodes []string, baselines map[string]uint, tableFunc func(string) string) int {
	total := 0
	for _, code := range syncCodes {
		table := tableFunc(code)
		if table == "" {
			continue
		}
		baseline := baselines[code]
		var count int64
		if err := n.db.WithContext(ctx).Table(table).
			Where("id > ?", baseline).Count(&count).Error; err != nil {
			log.Printf("[CrawlerRunNode] Count new rows of %s failed: %v", table, err)
			continue
		}
		total += int(count)
	}
	return total
}
