package crawler

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
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

	// 当前活跃的 runID（atomic，用于 OnCancel 安全读取）
	activeRunID atomic.Uint64
}

func NewRunNode(db *gorm.DB, crawlerRepo *repository.CrawlerRepository) *RunNode {
	return &RunNode{
		BaseNode:    nodes.NewBaseNode("crawler_run"),
		db:          db,
		crawlerRepo: crawlerRepo,
		crawlerSvc:  crawlerSvc.NewService(crawlerRepo),
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
	timeoutMinutes    := n.GetInt(config, "timeoutMinutes", 10)

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
		TimeoutMinutes:    timeoutMinutes,
	})
	if err != nil {
		return nil, n.WrapError("failed to trigger crawler", err)
	}

	log.Printf("[CrawlerRunNode] Crawler triggered: runId=%d", result.RunID)
	n.activeRunID.Store(uint64(result.RunID))
	defer n.activeRunID.Store(0)

	if waitForCompletion {
		timeout := time.Duration(timeoutMinutes) * time.Minute
		if err := n.crawlerSvc.WaitForCompletion(ctx, result.RunID, timeout); err != nil {
			return nil, n.WrapError("crawler execution failed", err)
		}
		log.Printf("[CrawlerRunNode] Crawler finished: runId=%d", result.RunID)
	}

	status := "triggered"
	if waitForCompletion {
		status = "completed"
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

	return nodes.CarryForward(input, produced), nil
}

// OnCancel 覆盖：立即发 stop 信号给 MediaCrawler，不等待爬虫完成。
// engine 已经 cancel 了 ctx，WaitForCompletion 会通过 ctx.Done() 返回；
// 这里额外发 /api/crawler/stop 确保 MediaCrawler 进程真正停止。
func (n *RunNode) OnCancel(_ context.Context) {
	runID := uint(n.activeRunID.Load())
	if runID == 0 {
		log.Printf("[CrawlerRunNode] OnCancel: no active run, skip stop signal")
		return
	}
	log.Printf("[CrawlerRunNode] OnCancel: sending stop to MediaCrawler for runId=%d", runID)
	// 使用后台 context，因为传入的 ctx 可能已经 Done
	go n.crawlerSvc.StopCrawler(runID)
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
